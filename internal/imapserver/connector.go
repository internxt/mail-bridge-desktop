package imapserver

import (
	"context"
	"fmt"
	"sync"

	"github.com/ProtonMail/gluon/connector"
	"github.com/ProtonMail/gluon/imap"

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
}

// NewMailConnectorFactory serves the account's own mail, rather than a fixture.
func NewMailConnectorFactory(service MailService, log *logger.Logger) ConnectorFactory {
	return func(ctx context.Context, session UnlockedSession, credentials Credentials) (connector.Connector, error) {
		return &mailConnector{
			service:      service,
			log:          log,
			updates:      make(chan imap.Update, updateBufferSize),
			mailboxTypes: make(map[imap.MailboxID]api.Mailbox),
		}, nil
	}
}

const updateBufferSize = 32

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

// Sync loads the account's folders and their messages.
func (c *mailConnector) Sync(ctx context.Context) error {
	// What the service remembers is only worth holding for the length of a
	// sync: it is what keeps a conversation in several folders from being
	// downloaded once per folder, and nothing in it expires on its own.
	defer c.service.ForgetThreads()

	mailboxes, err := c.service.ListMailboxes(ctx)
	if err != nil {
		return fmt.Errorf("list mailboxes: %w", err)
	}

	for _, mailbox := range mailboxes {
		if err := c.syncMailbox(ctx, mailbox); err != nil {
			c.log.Warn("skipping mailbox %s: %v", mailbox.Name, err)
			continue
		}
	}

	return nil
}

// syncMailbox announces one folder and the messages it holds.
func (c *mailConnector) syncMailbox(ctx context.Context, mailbox api.MailboxResponseDto) error {
	c.rememberMailboxType(mailbox)
	c.updates <- imap.NewMailboxCreated(toIMAPMailbox(mailbox))

	summaries, err := c.service.ListAllEmails(ctx, api.ListEmailsOptions{
		Mailbox: mailboxType(mailbox),
	})
	if err != nil {
		return fmt.Errorf("list emails: %w", err)
	}

	messages := make([]*imap.MessageCreated, 0, len(summaries))
	for _, summary := range summaries {
		literal, err := c.service.GetMessageLiteral(ctx, summary.Id)
		if err != nil {
			c.log.Warn("serving message %s without its body: %v", summary.Id, err)
		}

		message, err := toIMAPMessage(mailbox, summary, literal)
		if err != nil {
			c.log.Warn("skipping message %s: %v", summary.Id, err)
			continue
		}
		messages = append(messages, &message)
	}

	if len(messages) > 0 {
		c.updates <- imap.NewMessagesCreated(true, messages...)
	}

	c.log.Info("synced %s: %d messages", mailbox.Name, len(messages))
	return nil
}
