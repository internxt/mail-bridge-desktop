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
	"encoding/base64"
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
}

// MailService turns an account session into Mail API calls.
type MailService struct {
	api             commands.Client
	account         Account
	log             *logger.Logger
	threadsMutex    sync.Mutex
	threads         map[string]api.EmailResponseDto
	serverPublicKey []byte
	ownPublicKey    []byte
}

func New(client commands.Client, account Account, serverPublicKey []byte, log *logger.Logger) *MailService {
	return &MailService{
		api:             client,
		account:         account,
		log:             log,
		threads:         make(map[string]api.EmailResponseDto),
		serverPublicKey: serverPublicKey,
	}
}

// Init fetches the account's own public key, so the sender can read their
// own Sent copy of anything they send.
func (s *MailService) Init(ctx context.Context) error {
	keys, err := s.api.GetMailAccountKeys(ctx, s.account.Token)
	if err != nil {
		return fmt.Errorf("get account keys: %w", err)
	}

	publicKey, err := base64.StdEncoding.DecodeString(keys.PublicKey)
	if err != nil {
		return fmt.Errorf("decode account public key: %w", err)
	}

	s.ownPublicKey = publicKey
	return nil
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
	return commands.ListMailboxes(ctx, s.api, s.account.Token)
}

// ListEmails returns one page of email summaries from a folder.
func (s *MailService) ListEmails(ctx context.Context, opts api.ListEmailsOptions) (api.EmailListResponseDto, error) {
	return commands.ListEmails(ctx, s.api, s.account.Token, opts)
}

// ListAllEmails returns every email in a folder, paging through the API.
func (s *MailService) ListAllEmails(ctx context.Context, opts api.ListEmailsOptions) ([]api.EmailSummaryResponseDto, error) {
	return commands.ListAllEmails(ctx, s.api, s.account.Token, opts, s.decryptionAccount(), func(err error) {
		s.log.Warn("listing without a preview: %v", err)
	})
}

// GetMessageLiteral returns one email as the RFC 5322 message a mail client
// expects, with its body decrypted when the account holds the keys.
func (s *MailService) GetMessageLiteral(ctx context.Context, emailID string) ([]byte, error) {
	email, decryptErr := s.email(ctx, emailID)
	if email.Id == "" {
		return nil, decryptErr
	}

	literal, err := BuildLiteral(email)
	if err != nil {
		return nil, err
	}
	return literal, decryptErr
}

func (s *MailService) email(ctx context.Context, emailID string) (api.EmailResponseDto, error) {
	if email, found := s.rememberedEmail(emailID); found {
		return email, nil
	}

	thread, err := s.api.GetThread(ctx, s.account.Token, emailID)
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

// MarkRead marks emails as read or unread.
func (s *MailService) MarkRead(ctx context.Context, emailIDs []string, read bool) error {
	return commands.MarkRead(ctx, s.api, s.account.Token, emailIDs, read)
}

// MarkFlagged flags or unflags emails.
func (s *MailService) MarkFlagged(ctx context.Context, emailIDs []string, flagged bool) error {
	return commands.MarkFlagged(ctx, s.api, s.account.Token, emailIDs, flagged)
}

// Move puts emails in another mailbox.
func (s *MailService) Move(ctx context.Context, emailIDs []string, mailbox api.Mailbox) error {
	return commands.Move(ctx, s.api, s.account.Token, emailIDs, mailbox)
}

// Delete removes emails for good.
func (s *MailService) Delete(ctx context.Context, emailIDs []string) error {
	return commands.Delete(ctx, s.api, s.account.Token, emailIDs)
}

// SendEmail parses a raw RFC 5322 message an SMTP client handed over, seals
// it for every recipient, and submits it.
func (s *MailService) SendEmail(ctx context.Context, raw []byte, envelopeRecipients []string) error {
	msg, err := ParseOutgoingMessage(raw, envelopeRecipients)
	if err != nil {
		return err
	}
	return commands.SendEmail(ctx, s.api, s.account.Token, msg, s.decryptionAccount(), s.serverPublicKey)
}

// SaveDraft stores a message a client is still writing, sealed for the
// account alone, and returns the ID the backend gave it.
func (s *MailService) SaveDraft(ctx context.Context, raw []byte) (string, error) {
	msg, err := ParseOutgoingMessage(raw, nil)
	if err != nil {
		return "", err
	}

	draft, err := commands.SaveDraft(ctx, s.api, s.account.Token, msg, s.decryptionAccount())
	if err != nil {
		return "", err
	}
	return draft.Id, nil
}

// DiscardDrafts destroys drafts for good, rather than moving them to the
// trash the way deleting an ordinary email does.
func (s *MailService) DiscardDrafts(ctx context.Context, draftIDs []string) error {
	return commands.DiscardDrafts(ctx, s.api, s.account.Token, draftIDs)
}

func (s *MailService) decryptionAccount() commands.Account {
	return commands.Account{
		Address:    s.account.Address,
		PrivateKey: s.account.PrivateKey,
		PublicKey:  s.ownPublicKey,
	}
}
