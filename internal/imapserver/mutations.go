package imapserver

import (
	"context"
	"time"

	"github.com/ProtonMail/gluon/connector"
	"github.com/ProtonMail/gluon/imap"
)

// TODO(write): wire these to the API.

func (c *mailConnector) CreateMailbox(ctx context.Context, name []string) (imap.Mailbox, error) {
	return imap.Mailbox{}, connector.ErrOperationNotAllowed
}

func (c *mailConnector) UpdateMailboxName(ctx context.Context, mboxID imap.MailboxID, newName []string) error {
	return connector.ErrOperationNotAllowed
}

func (c *mailConnector) DeleteMailbox(ctx context.Context, mboxID imap.MailboxID) error {
	return connector.ErrOperationNotAllowed
}

func (c *mailConnector) CreateMessage(ctx context.Context, mboxID imap.MailboxID, literal []byte, flags imap.FlagSet, date time.Time) (imap.Message, []byte, error) {
	return imap.Message{}, nil, connector.ErrOperationNotAllowed
}

func (c *mailConnector) AddMessagesToMailbox(ctx context.Context, messageIDs []imap.MessageID, mboxID imap.MailboxID) error {
	return connector.ErrOperationNotAllowed
}

func (c *mailConnector) RemoveMessagesFromMailbox(ctx context.Context, messageIDs []imap.MessageID, mboxID imap.MailboxID) error {
	return connector.ErrOperationNotAllowed
}

func (c *mailConnector) MoveMessages(ctx context.Context, messageIDs []imap.MessageID, mboxFromID, mboxToID imap.MailboxID) (bool, error) {
	return false, connector.ErrOperationNotAllowed
}

func (c *mailConnector) MarkMessagesSeen(ctx context.Context, messageIDs []imap.MessageID, seen bool) error {
	return connector.ErrOperationNotAllowed
}

func (c *mailConnector) MarkMessagesFlagged(ctx context.Context, messageIDs []imap.MessageID, flagged bool) error {
	return connector.ErrOperationNotAllowed
}
