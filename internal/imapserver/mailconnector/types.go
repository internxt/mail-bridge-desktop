package mailconnector

import (
	"context"
	"sync"
	"time"

	"github.com/ProtonMail/gluon/imap"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/logger"
)

// MailService turns an account session into Mail API calls. The bridge's
// internal/mail package implements it against the real backend; tests supply
// a fake.
type MailService interface {
	ListMailboxes(ctx context.Context) ([]api.MailboxResponseDto, error)
	ListAllEmails(ctx context.Context, opts api.ListEmailsOptions) ([]api.EmailSummaryResponseDto, error)
	GetMessageLiteral(ctx context.Context, emailID string) ([]byte, error)
	ForgetThreads()
	MarkRead(ctx context.Context, emailIDs []string, read bool) error
	MarkFlagged(ctx context.Context, emailIDs []string, flagged bool) error
	Move(ctx context.Context, emailIDs []string, mailbox api.Mailbox) error
	Delete(ctx context.Context, emailIDs []string) error
	SaveDraft(ctx context.Context, raw []byte) (string, error)
	SentCopyID(raw []byte) (string, bool)
	DiscardDrafts(ctx context.Context, draftIDs []string) error
}

// MailConnector is the Gluon connector.Connector implementation that serves
// an account's real mail.
type MailConnector struct {
	service MailService
	log     *logger.Logger
	sync    SyncEvents

	updates chan imap.Update
	// Closes the Gluon instance when the connector is closed.
	closeOnce         sync.Once
	mailboxTypesMutex sync.RWMutex
	mailboxTypes      map[imap.MailboxID]api.Mailbox
	messagesMutex     sync.RWMutex
	messages          map[string]messageState
}

// SyncEvents is what the connector reports while it downloads new mail. A nil
// handler is not called, so a caller that wants none supplies none.
type SyncEvents struct {
	OnStarted  func(total int)
	OnProgress func(done, total, percent int)
	OnFinished func(done, total int, code string)
}

// mailboxWork is what one folder's listing found that still needs downloading.
// Listing every folder before downloading any body is what lets a sync say how
// many messages it is about to fetch in total, rather than only how many this
// folder holds.
type mailboxWork struct {
	mailbox api.MailboxResponseDto
	created []api.EmailSummaryResponseDto
}

// messageState is everything about a message that can change without the
// message itself changing: which folders hold it, and the flags a client shows.
type messageState struct {
	mailboxIDs []string
	isRead     bool
	isFlagged  bool
	isDraft    bool
}

// Synchronizer is a connector that can bring itself up to date. The mail
// connector is one; the development fixture is not.
type Synchronizer interface {
	Sync(context.Context) error
}

// Poller runs a Synchronizer on a timer until stopped.
type Poller struct {
	sync     Synchronizer
	interval time.Duration
	log      *logger.Logger

	reset chan struct{}
	stop  context.CancelFunc
	done  chan struct{}
}
