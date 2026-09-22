// Package mail sits between the transport and the storage layers: it holds
// the account's session and calls the Mail API with it.
//
// Keeping this here is what lets internal/api stay pure transport and
// internal/crypto stay pure cryptography: neither has to know where the
// account's token or keys come from.
//
// The operations themselves live in the commands subpackage, one per file.
package mail

import (
	"context"
	"fmt"
	"sync"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/logger"
	"mail-bridge-desktop/internal/mail/commands"
)

// Account is the session the bridge acts for. It arrives from the parent over
// the control channel and is held in memory only: nothing here is persisted,
// so signing out is a matter of dropping the service.
type Account struct {
	Token      string
	Address    string
	PrivateKey []byte
	PublicKey  []byte
}

// MailService turns an account session into Mail API calls.
type MailService struct {
	api          commands.Client
	accountMutex sync.RWMutex
	account      Account

	log             *logger.Logger
	threadsMutex    sync.Mutex
	threads         map[string]api.EmailResponseDto
	serverPublicKey []byte

	sentMutex  sync.Mutex
	sentCopies map[string]string
	sentOrder  []string
}

// sentCopiesKept bounds what SendEmail remembers. A client appends its copy of a
// message seconds after sending it, so only the last few are ever asked for.
const sentCopiesKept = 32

func New(client commands.Client, account Account, serverPublicKey []byte, log *logger.Logger) *MailService {
	return &MailService{
		api:             client,
		account:         account,
		log:             log,
		threads:         make(map[string]api.EmailResponseDto),
		serverPublicKey: serverPublicKey,
		sentCopies:      make(map[string]string, sentCopiesKept),
	}
}

func (s *MailService) SetToken(token string) {
	s.accountMutex.Lock()
	defer s.accountMutex.Unlock()

	s.account.Token = token
}

func (s *MailService) token() string {
	s.accountMutex.RLock()
	defer s.accountMutex.RUnlock()
	return s.account.Token
}

// ForgetThreads drops the messages remembered during a sync.
func (s *MailService) ForgetThreads() {
	s.threadsMutex.Lock()
	defer s.threadsMutex.Unlock()
	clear(s.threads)
}

// rememberThread indexes every message a thread returned, so the other folders
// it appears in do not have to ask for it again.
func (s *MailService) rememberThread(thread []api.EmailResponseDto) {
	s.threadsMutex.Lock()
	defer s.threadsMutex.Unlock()
	for _, email := range thread {
		s.threads[email.Id] = email
	}
}

// rememberedEmail returns a message from an already fetched thread.
func (s *MailService) rememberedEmail(emailID string) (api.EmailResponseDto, bool) {
	s.threadsMutex.Lock()
	defer s.threadsMutex.Unlock()
	email, found := s.threads[emailID]
	return email, found
}

// ListMailboxes returns the account's folders.
func (s *MailService) ListMailboxes(ctx context.Context) ([]api.MailboxResponseDto, error) {
	return commands.ListMailboxes(ctx, s.api, s.token())
}

// ListEmails returns one page of email summaries from a folder.
func (s *MailService) ListEmails(ctx context.Context, opts api.ListEmailsOptions) (api.EmailListResponseDto, error) {
	return commands.ListEmails(ctx, s.api, s.token(), opts)
}

// ListAllEmails returns every email in a folder, paging through the API.
func (s *MailService) ListAllEmails(ctx context.Context, opts api.ListEmailsOptions) ([]api.EmailSummaryResponseDto, error) {
	return commands.ListAllEmails(ctx, s.api, s.token(), opts, s.decryptionAccount(), func(err error) {
		s.log.Warn("listing without a preview: %v", err)
	})
}

// GetMessageLiteral returns one email as the RFC 5322 message a mail client
// expects, with its body decrypted when the account holds the keys.
//
// Attachments are declared but left empty: this is what the sync stores, so a
// client is told the message's real size and structure without a byte of any
// attachment having been downloaded. ResolveAttachments fills them in when a
// client actually opens the message.
func (s *MailService) GetMessageLiteral(ctx context.Context, emailID string) ([]byte, error) {
	email, decryptErr := s.email(ctx, emailID)
	if email.Id == "" {
		return nil, decryptErr
	}

	literal, err := BuildLiteralWithAttachments(email, nil)
	if err != nil {
		return nil, err
	}
	return literal, decryptErr
}

// ResolveAttachments rebuilds an email's message with its attachments in
// place, downloading and decrypting them.
func (s *MailService) ResolveAttachments(ctx context.Context, emailID string, literal []byte) ([]byte, error) {
	email, err := s.rawEmail(ctx, emailID)
	if err != nil {
		return nil, err
	}
	if len(email.Attachments) == 0 {
		return literal, nil
	}

	blobs, err := commands.DownloadAttachments(ctx, s.api, s.token(), email, s.decryptionAccount(), func(err error) {
		s.log.Warn("serving message %s without one of its attachments: %v", emailID, err)
	})
	if err != nil {
		return nil, err
	}

	opened, err := commands.PickFromThread([]api.EmailResponseDto{email}, emailID, s.decryptionAccount())
	if err != nil {
		return nil, err
	}
	return BuildLiteralWithAttachments(opened, blobs)
}

func (s *MailService) email(ctx context.Context, emailID string) (api.EmailResponseDto, error) {
	if email, found := s.rememberedEmail(emailID); found {
		return email, nil
	}

	thread, err := s.api.GetThread(ctx, s.token(), emailID)
	if err != nil {
		return api.EmailResponseDto{}, err
	}

	decrypted := make([]api.EmailResponseDto, 0, len(thread))
	for _, email := range thread {
		opened, err := commands.PickFromThread(thread, email.Id, s.decryptionAccount())
		if err != nil {
			s.log.Warn("remembering message %s without its body: %v", email.Id, err)
			opened = email
		}
		decrypted = append(decrypted, opened)
	}
	s.rememberThread(decrypted)

	return commands.PickFromThread(decrypted, emailID, s.decryptionAccount())
}

// rawEmail returns the email as the API sent it, envelope and all.
func (s *MailService) rawEmail(ctx context.Context, emailID string) (api.EmailResponseDto, error) {
	thread, err := s.api.GetThread(ctx, s.token(), emailID)
	if err != nil {
		return api.EmailResponseDto{}, err
	}

	for _, email := range thread {
		if email.Id == emailID {
			return email, nil
		}
	}
	if len(thread) == 1 {
		return thread[0], nil
	}
	return api.EmailResponseDto{}, fmt.Errorf("get email %s: %w", emailID, commands.ErrEmailNotFound)
}

// MarkRead marks emails as read or unread.
func (s *MailService) MarkRead(ctx context.Context, emailIDs []string, read bool) error {
	return commands.MarkRead(ctx, s.api, s.token(), emailIDs, read)
}

// MarkFlagged flags or unflags emails.
func (s *MailService) MarkFlagged(ctx context.Context, emailIDs []string, flagged bool) error {
	return commands.MarkFlagged(ctx, s.api, s.token(), emailIDs, flagged)
}

// Move puts emails in another mailbox.
func (s *MailService) Move(ctx context.Context, emailIDs []string, mailbox api.Mailbox) error {
	return commands.Move(ctx, s.api, s.token(), emailIDs, mailbox)
}

// Delete removes emails for good.
func (s *MailService) Delete(ctx context.Context, emailIDs []string) error {
	return commands.Delete(ctx, s.api, s.token(), emailIDs)
}

// SendEmail parses a raw RFC 5322 message an SMTP client handed over, seals
// it for every recipient, and submits it.
func (s *MailService) SendEmail(ctx context.Context, raw []byte, envelopeRecipients []string) error {
	msg, err := ParseOutgoingMessage(raw, envelopeRecipients)
	if err != nil {
		return err
	}

	sentID, err := commands.SendEmail(ctx, s.api, s.token(), msg, s.decryptionAccount(), s.serverPublicKey)
	if err != nil {
		return err
	}

	s.rememberSentCopy(raw, sentID)
	return nil
}

// SentCopyID is the ID the backend gave the copy it filed when this very message was
// sent through the bridge. A client appends its own copy to Sent right afterwards, and
// answering that append with this ID is what keeps the two from becoming two messages.
func (s *MailService) SentCopyID(raw []byte) (string, bool) {
	messageID := messageIDHeaderOf(raw)
	if messageID == "" {
		return "", false
	}

	s.sentMutex.Lock()
	defer s.sentMutex.Unlock()

	id, found := s.sentCopies[messageID]
	return id, found
}

// rememberSentCopy files a message under the Message-ID its client stamped on it,
// which is the only thing the append that follows will have in common with it.
func (s *MailService) rememberSentCopy(raw []byte, sentID string) {
	messageID := messageIDHeaderOf(raw)
	if messageID == "" || sentID == "" {
		return
	}

	s.sentMutex.Lock()
	defer s.sentMutex.Unlock()

	if _, known := s.sentCopies[messageID]; !known {
		s.sentOrder = append(s.sentOrder, messageID)
	}
	s.sentCopies[messageID] = sentID

	for len(s.sentOrder) > sentCopiesKept {
		delete(s.sentCopies, s.sentOrder[0])
		s.sentOrder = s.sentOrder[1:]
	}
}

// SaveDraft stores a message a client is still writing, sealed for the
// account alone, and returns the ID the backend gave it.
func (s *MailService) SaveDraft(ctx context.Context, raw []byte) (string, error) {
	msg, err := ParseOutgoingMessage(raw, nil)
	if err != nil {
		return "", err
	}

	draft, err := commands.SaveDraft(ctx, s.api, s.token(), msg, s.decryptionAccount())
	if err != nil {
		return "", err
	}
	return draft.Id, nil
}

// DiscardDrafts destroys drafts for good, rather than moving them to the
// trash the way deleting an ordinary email does.
func (s *MailService) DiscardDrafts(ctx context.Context, draftIDs []string) error {
	return commands.DiscardDrafts(ctx, s.api, s.token(), draftIDs)
}

func (s *MailService) decryptionAccount() commands.Account {
	s.accountMutex.RLock()
	defer s.accountMutex.RUnlock()

	return commands.Account{
		Address:    s.account.Address,
		PrivateKey: s.account.PrivateKey,
		PublicKey:  s.account.PublicKey,
	}
}
