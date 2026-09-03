package imapserver

import (
	"slices"

	"github.com/ProtonMail/gluon/imap"

	"mail-bridge-desktop/internal/api"
)

// messageState is everything about a message that can change without the
// message itself changing: which folders hold it, and the flags a client shows.
type messageState struct {
	mailboxIDs []string
	isRead     bool
	isFlagged  bool
	isDraft    bool
}

func stateOf(summary api.EmailSummaryResponseDto) messageState {
	return messageState{
		mailboxIDs: slices.Clone(summary.MailboxIds),
		isRead:     summary.IsRead,
		isFlagged:  summary.IsFlagged,
		isDraft:    summary.IsDraft,
	}
}

// sameAs reports whether nothing a client can see has changed.
func (s messageState) sameAs(other messageState) bool {
	if s.isRead != other.isRead || s.isFlagged != other.isFlagged || s.isDraft != other.isDraft {
		return false
	}
	if len(s.mailboxIDs) != len(other.mailboxIDs) {
		return false
	}
	for _, id := range s.mailboxIDs {
		if !slices.Contains(other.mailboxIDs, id) {
			return false
		}
	}
	return true
}

func (s messageState) imapMailboxIDs() []imap.MailboxID {
	ids := make([]imap.MailboxID, 0, len(s.mailboxIDs))
	for _, id := range s.mailboxIDs {
		ids = append(ids, imap.MailboxID(id))
	}
	return ids
}

// flags is the flag set Gluon stores for a message. It has to be the complete
// set rather than what changed: Gluon replaces the flags with whatever arrives.
func (s messageState) flags() imap.FlagSet {
	flags := imap.NewFlagSet()
	if s.isRead {
		flags.AddToSelf(imap.FlagSeen)
	}
	if s.isFlagged {
		flags.AddToSelf(imap.FlagFlagged)
	}
	if s.isDraft {
		flags.AddToSelf(imap.FlagDraft)
	}
	return flags
}

// rememberMessages records what a sync saw, replacing what it knew about those
// messages.
func (c *mailConnector) rememberMessages(seen map[string]messageState) {
	c.messagesMutex.Lock()
	defer c.messagesMutex.Unlock()
	if c.messages == nil {
		c.messages = make(map[string]messageState, len(seen))
	}
	for id, state := range seen {
		c.messages[id] = state
	}
}

// forgetMessages drops messages that are gone, so a later sync treats them as
// new if they come back.
func (c *mailConnector) forgetMessages(ids []string) {
	c.messagesMutex.Lock()
	defer c.messagesMutex.Unlock()
	for _, id := range ids {
		delete(c.messages, id)
	}
}

// knownMessage returns what the last sync saw of a message.
func (c *mailConnector) knownMessage(id string) (messageState, bool) {
	c.messagesMutex.RLock()
	defer c.messagesMutex.RUnlock()
	state, found := c.messages[id]
	return state, found
}

// missingMessages returns the messages known before that this sync did not see
// anywhere.
func (c *mailConnector) missingMessages(seen map[string]messageState) []string {
	c.messagesMutex.RLock()
	defer c.messagesMutex.RUnlock()

	var missing []string
	for id := range c.messages {
		if _, found := seen[id]; !found {
			missing = append(missing, id)
		}
	}
	return missing
}

// knownMailbox reports whether a folder has been announced to Gluon already.
func (c *mailConnector) knownMailbox(id imap.MailboxID) bool {
	c.mailboxTypesMutex.RLock()
	defer c.mailboxTypesMutex.RUnlock()
	_, found := c.mailboxTypes[id]
	return found
}
