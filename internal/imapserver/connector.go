package imapserver

import (
	"context"
	"fmt"
	"sync"

	"github.com/ProtonMail/gluon/connector"
	"github.com/ProtonMail/gluon/imap"
	"golang.org/x/sync/errgroup"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/logger"
)

type MailService interface {
	ListMailboxes(ctx context.Context) ([]api.MailboxResponseDto, error)
	ListAllEmails(ctx context.Context, opts api.ListEmailsOptions) ([]api.EmailSummaryResponseDto, error)
	GetMessageLiteral(ctx context.Context, emailID string) ([]byte, error)
	ForgetThreads()
	MarkRead(ctx context.Context, emailIDs []string, read bool) error
	MarkFlagged(ctx context.Context, emailIDs []string, flagged bool) error
	Move(ctx context.Context, emailIDs []string, mailbox api.Mailbox) error
	Delete(ctx context.Context, emailIDs []string) error
}

type mailConnector struct {
	service MailService
	log     *logger.Logger

	updates chan imap.Update
	// Closes the Gluon instance when the connector is closed.
	closeOnce         sync.Once
	mailboxTypesMutex sync.RWMutex
	mailboxTypes      map[imap.MailboxID]api.Mailbox
	messagesMutex     sync.RWMutex
	messages          map[string]messageState
}

// NewMailConnectorFactory serves the account's own mail, rather than a fixture.
func NewMailConnectorFactory(service MailService, log *logger.Logger) ConnectorFactory {
	return func(ctx context.Context, session UnlockedSession, credentials Credentials) (connector.Connector, error) {
		return &mailConnector{
			service:      service,
			log:          log,
			updates:      make(chan imap.Update, updateBufferSize),
			mailboxTypes: make(map[imap.MailboxID]api.Mailbox),
			messages:     make(map[string]messageState),
		}, nil
	}
}

const updateBufferSize = 32

const (
	listEmailsLimit      = 100
	fetchBodyConcurrency = 8
)

// Authorize is replaced by authConnector, which the server wraps every
// connector in. It is here only to satisfy the interface.
func (c *mailConnector) Authorize(context.Context, string, []byte) bool { return false }

// GetMailboxVisibility shows every folder the account has.
func (c *mailConnector) GetMailboxVisibility(context.Context, imap.MailboxID) imap.MailboxVisibility {
	return imap.Visible
}

// GetUpdates returns the channel Gluon reads mailbox changes from.
func (c *mailConnector) GetUpdates() <-chan imap.Update { return c.updates }

// GetMessageLiteral is called when Gluon needs a message body it does not have
// cached.
func (c *mailConnector) GetMessageLiteral(ctx context.Context, id imap.MessageID) ([]byte, error) {
	literal, err := c.service.GetMessageLiteral(ctx, string(id))
	if literal == nil {
		return nil, fmt.Errorf("fetch message %s: %w", id, err)
	}
	if err != nil {
		c.log.Warn("serving message %s undecrypted: %v", id, err)
	}
	return literal, nil
}

func (c *mailConnector) Close(ctx context.Context) error {
	c.closeOnce.Do(func() { close(c.updates) })
	return nil
}

// Sync brings Gluon in line with the account: it announces what is new, what
// changed elsewhere, and what is gone.
func (c *mailConnector) Sync(ctx context.Context) error {
	// What the service remembers is only worth holding for the length of a
	// sync: it is what keeps a conversation in several folders from being
	// downloaded once per folder, and nothing in it expires on its own.
	defer c.service.ForgetThreads()

	mailboxes, err := c.service.ListMailboxes(ctx)
	if err != nil {
		return fmt.Errorf("list mailboxes: %w", err)
	}

	seen := make(map[string]messageState)
	complete := true

	for _, mailbox := range mailboxes {
		if err := c.syncMailbox(ctx, mailbox, seen); err != nil {
			c.log.Warn("skipping mailbox %s: %v", mailbox.Name, err)
			complete = false
			continue
		}
	}

	c.rememberMessages(seen)

	// Deletions are only safe when every folder answered. After a partial run
	// the messages of the folder that failed are missing from seen, and taking
	// that at face value would delete mail that is still there.
	if complete {
		c.forgetDeleted(seen)
	}

	return nil
}

// forgetDeleted tells Gluon about messages that are no longer in the account.
func (c *mailConnector) forgetDeleted(seen map[string]messageState) {
	missing := c.missingMessages(seen)
	for _, id := range missing {
		c.updates <- imap.NewMessagesDeleted(imap.MessageID(id))
	}
	c.forgetMessages(missing)

	if len(missing) > 0 {
		c.log.Info("removed %d messages that are gone", len(missing))
	}
}

// announceNewMessages announces messages Gluon has never seen, fetching their bodies.
func (c *mailConnector) announceNewMessages(ctx context.Context, mailbox api.MailboxResponseDto, summaries []api.EmailSummaryResponseDto) error {
	if len(summaries) == 0 {
		return nil
	}

	literals := make([][]byte, len(summaries))

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(fetchBodyConcurrency)

	for i, summary := range summaries {
		group.Go(func() error {
			literal, err := c.service.GetMessageLiteral(groupCtx, summary.Id)

			if err != nil {
				c.log.Warn("serving message %s without its body: %v", summary.Id, err)
			}

			literals[i] = literal
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return fmt.Errorf("fetch message bodies: %w", err)
	}

	messages := make([]*imap.MessageCreated, 0, len(summaries))

	for i, summary := range summaries {
		message, err := toIMAPMessage(mailbox, summary, literals[i])

		if err != nil {
			c.log.Warn("skipping message %s: %v", summary.Id, err)
			continue
		}

		messages = append(messages, &message)
	}

	if len(messages) > 0 {
		c.updates <- imap.NewMessagesCreated(true, messages...)
	}

	return nil
}

// syncMailbox announces one folder and reconciles the messages it holds.
func (c *mailConnector) syncMailbox(ctx context.Context, mailbox api.MailboxResponseDto, seen map[string]messageState) error {
	// A folder is announced once. Gluon ignores a repeat, but a sync on a timer
	// would otherwise send one per folder per cycle for nothing.
	if !c.knownMailbox(imap.MailboxID(mailbox.Id)) {
		c.updates <- imap.NewMailboxCreated(toIMAPMailbox(mailbox))
	}
	c.rememberMailboxType(mailbox)

	summaries, err := c.service.ListAllEmails(ctx, api.ListEmailsOptions{
		Mailbox: mailboxType(mailbox),
		Limit:   listEmailsLimit,
	})
	if err != nil {
		return fmt.Errorf("list emails: %w", err)
	}

	var created []api.EmailSummaryResponseDto

	for _, summary := range summaries {
		state := stateOf(summary)
		seen[summary.Id] = state

		known, found := c.knownMessage(summary.Id)

		switch {
		case !found:
			created = append(created, summary)
		case !known.sameAs(state):
			c.updates <- imap.NewMessageMailboxesUpdated(
				imap.MessageID(summary.Id),
				state.imapMailboxIDs(),
				state.flags(),
			)
		}
	}

	if err := c.announceNewMessages(ctx, mailbox, created); err != nil {
		return err
	}

	c.log.Info("synced %s: %d messages, %d new", mailbox.Name, len(summaries), len(created))
	return nil
}
