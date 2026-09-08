package mailconnector

import (
	"context"
	"fmt"
	"time"

	"github.com/ProtonMail/gluon/connector"
	"github.com/ProtonMail/gluon/imap"

	"mail-bridge-desktop/internal/api"
)

// rememberMailboxType records what kind of folder an ID refers to, so a later
// move can name its destination.
func (c *MailConnector) rememberMailboxType(mailbox api.MailboxResponseDto) {
	kind := mailboxType(mailbox)
	if kind == "" {
		return
	}
	c.mailboxTypesMutex.Lock()
	defer c.mailboxTypesMutex.Unlock()
	if c.mailboxTypes == nil {
		c.mailboxTypes = make(map[imap.MailboxID]api.Mailbox)
	}
	c.mailboxTypes[imap.MailboxID(mailbox.Id)] = kind
}

// mailboxTypeOf returns the kind of folder an ID refers to, as learnt during
// the sync.
func (c *MailConnector) mailboxTypeOf(id imap.MailboxID) (api.Mailbox, error) {
	c.mailboxTypesMutex.RLock()
	defer c.mailboxTypesMutex.RUnlock()

	kind, found := c.mailboxTypes[id]
	if !found {
		return "", fmt.Errorf("unknown mailbox %s", id)
	}
	return kind, nil
}

func emailIDs(ids []imap.MessageID) []string {
	converted := make([]string, 0, len(ids))
	for _, id := range ids {
		converted = append(converted, string(id))
	}
	return converted
}

// MarkMessagesSeen records that mail was read, or marked unread again.
func (c *MailConnector) MarkMessagesSeen(ctx context.Context, ids []imap.MessageID, seen bool) error {
	return c.service.MarkRead(ctx, emailIDs(ids), seen)
}

// MarkMessagesFlagged records a client starring mail, or removing the star.
func (c *MailConnector) MarkMessagesFlagged(ctx context.Context, ids []imap.MessageID, flagged bool) error {
	return c.service.MarkFlagged(ctx, emailIDs(ids), flagged)
}

// MoveMessages moves mail between folders.
func (c *MailConnector) MoveMessages(ctx context.Context, ids []imap.MessageID, mboxFromID, mboxToID imap.MailboxID) (bool, error) {
	destination, err := c.mailboxTypeOf(mboxToID)
	if err != nil {
		return false, err
	}
	if err := c.service.Move(ctx, emailIDs(ids), destination); err != nil {
		return false, err
	}
	return true, nil
}

// AddMessagesToMailbox is how a client copies mail into a folder.
//
// The API gives an email one folder, so adding it to another is the same
// request as moving it. A client that copies will see the original disappear,
// which is the closest honest answer available.
func (c *MailConnector) AddMessagesToMailbox(ctx context.Context, ids []imap.MessageID, mboxID imap.MailboxID) error {
	destination, err := c.mailboxTypeOf(mboxID)
	if err != nil {
		return err
	}
	return c.service.Move(ctx, emailIDs(ids), destination)
}

// RemoveMessagesFromMailbox takes mail out of a folder, which is what a client
// expunging deleted messages asks for.

func (c *MailConnector) RemoveMessagesFromMailbox(ctx context.Context, ids []imap.MessageID, mboxID imap.MailboxID) error {
	from, err := c.mailboxTypeOf(mboxID)
	if err != nil {
		return err
	}
	switch from {
	case api.MailboxDrafts:
		return c.service.DiscardDrafts(ctx, emailIDs(ids))
	case api.MailboxTrash:
		return c.service.Delete(ctx, emailIDs(ids))
	default:
		return c.service.Move(ctx, emailIDs(ids), api.MailboxTrash)
	}
}

func (c *MailConnector) CreateMailbox(ctx context.Context, name []string) (imap.Mailbox, error) {
	return imap.Mailbox{}, connector.ErrOperationNotAllowed
}

func (c *MailConnector) UpdateMailboxName(ctx context.Context, mboxID imap.MailboxID, newName []string) error {
	return connector.ErrOperationNotAllowed
}

func (c *MailConnector) DeleteMailbox(ctx context.Context, mboxID imap.MailboxID) error {
	return connector.ErrOperationNotAllowed
}

// CreateMessage is what a client appending a message asks for.
//
// Drafts are stored. Every append to drafts stores a new one: Gluon requires
// it — it treats a remote ID it already knows as an error here — and it
// matches what a client does anyway, appending the new revision and then
// expunging the old, which arrives separately as RemoveMessagesFromMailbox.
func (c *MailConnector) CreateMessage(ctx context.Context, mboxID imap.MailboxID, literal []byte, flags imap.FlagSet, date time.Time) (imap.Message, []byte, error) {
	destination, err := c.mailboxTypeOf(mboxID)
	if err != nil {
		return imap.Message{}, nil, err
	}

	switch destination {
	case api.MailboxDrafts:
		id, err := c.service.SaveDraft(ctx, literal)
		if err != nil {
			return imap.Message{}, nil, fmt.Errorf("save draft: %w", err)
		}
		return imap.Message{ID: imap.MessageID(id), Flags: flags, Date: date}, literal, nil

	case api.MailboxSent:
		id := discardedMessageID()
		c.updates <- imap.NewMessagesDeleted(id)
		c.log.Info("dropped a sent copy the backend already stores")
		return imap.Message{ID: id, Flags: flags, Date: date}, literal, nil

	default:
		return imap.Message{}, nil, connector.ErrOperationNotAllowed
	}
}

// discardedMessageID names a message that was accepted but not stored. It has
// to be unique, since Gluon rejects an append that comes back with an ID it
// already knows, and it must never collide with an ID the API could give out.
func discardedMessageID() imap.MessageID {
	return imap.MessageID(fmt.Sprintf("bridge-discarded-%d", time.Now().UnixNano()))
}
