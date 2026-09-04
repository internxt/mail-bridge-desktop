package mailconnector

import (
	"fmt"
	"strings"
	"time"

	"github.com/ProtonMail/gluon/imap"

	"mail-bridge-desktop/internal/api"
)

const inboxName = "INBOX"

// toIMAPMailbox describes one folder to Gluon.
func toIMAPMailbox(mailbox api.MailboxResponseDto) imap.Mailbox {
	return imap.Mailbox{
		ID:             imap.MailboxID(mailbox.Id),
		Name:           []string{mailboxName(mailbox)},
		Flags:          imap.NewFlagSet(imap.AttrNoInferiors),
		PermanentFlags: defaultPermanentFlags(),
		Attributes:     mailboxAttributes(mailbox),
	}
}

func mailboxName(mailbox api.MailboxResponseDto) string {
	if mailboxType(mailbox) == api.MailboxInbox {
		return inboxName
	}
	return mailbox.Name
}

func mailboxAttributes(mailbox api.MailboxResponseDto) imap.FlagSet {
	switch mailboxType(mailbox) {
	case api.MailboxSent:
		return imap.NewFlagSet(imap.AttrSent)
	case api.MailboxDrafts:
		return imap.NewFlagSet(imap.AttrDrafts)
	case api.MailboxTrash:
		return imap.NewFlagSet(imap.AttrTrash)
	case api.MailboxSpam:
		return imap.NewFlagSet(imap.AttrJunk)
	case api.MailboxArchive:
		return imap.NewFlagSet(imap.AttrArchive)
	default:
		return imap.NewFlagSet()
	}
}

func mailboxType(mailbox api.MailboxResponseDto) api.Mailbox {
	if mailbox.Type == nil {
		return ""
	}
	return *mailbox.Type
}

func defaultPermanentFlags() imap.FlagSet {
	return imap.NewFlagSet(imap.FlagSeen, imap.FlagFlagged, imap.FlagDeleted, imap.FlagAnswered, imap.FlagDraft)
}

// toIMAPMessage describes one message to Gluon.
func toIMAPMessage(mailbox api.MailboxResponseDto, summary api.EmailSummaryResponseDto, literal []byte) (imap.MessageCreated, error) {
	// Gluon stores the literal a message is created with and serves it from
	// then on, so this is the one chance to give a client the real message.
	// The summary stands in only when the body could not be fetched.
	if len(literal) == 0 {
		literal = summaryLiteral(summary)
	}

	parsed, err := imap.NewParsedMessage(literal)
	if err != nil {
		return imap.MessageCreated{}, fmt.Errorf("parse message: %w", err)
	}

	return imap.MessageCreated{
		Message: imap.Message{
			ID:    imap.MessageID(summary.Id),
			Flags: messageFlags(summary),
			Date:  receivedAt(summary),
		},
		Literal:       literal,
		MailboxIDs:    []imap.MailboxID{imap.MailboxID(mailbox.Id)},
		ParsedMessage: parsed,
	}, nil
}

func messageFlags(summary api.EmailSummaryResponseDto) imap.FlagSet {
	flags := imap.NewFlagSet()
	if summary.IsRead {
		flags.AddToSelf(imap.FlagSeen)
	}
	if summary.IsFlagged {
		flags.AddToSelf(imap.FlagFlagged)
	}
	if summary.IsDraft {
		flags.AddToSelf(imap.FlagDraft)
	}
	return flags
}

func receivedAt(summary api.EmailSummaryResponseDto) time.Time {
	received, err := time.Parse(time.RFC3339, summary.ReceivedAt)
	if err != nil {
		return time.Now()
	}
	return received
}

// summaryLiteral builds the RFC 5322 message a client sees before it opens
// anything: headers, and the preview in place of the body.
//
// The Message-ID is derived from the email's own ID so it stays the same
// between syncs. A generated one would make clients treat the same email as
// new every time.
func summaryLiteral(summary api.EmailSummaryResponseDto) []byte {
	var message strings.Builder

	message.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&message, "Message-ID: <%s@mail-bridge.internxt.local>\r\n", summary.Id)
	fmt.Fprintf(&message, "Date: %s\r\n", receivedAt(summary).Format(time.RFC1123Z))
	fmt.Fprintf(&message, "Subject: %s\r\n", headerValue(summary.Subject))
	fmt.Fprintf(&message, "From: %s\r\n", addressList(summary.From))
	message.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	message.WriteString("\r\n")
	message.WriteString(headerValue(summary.Preview))
	message.WriteString("\r\n")

	return []byte(message.String())
}

func addressList(addresses []api.EmailAddressDto) string {
	if len(addresses) == 0 {
		return "unknown@invalid"
	}

	rendered := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address.Name != nil && *address.Name != "" {
			rendered = append(rendered, fmt.Sprintf("%q <%s>", *address.Name, address.Email))
			continue
		}
		rendered = append(rendered, address.Email)
	}
	return strings.Join(rendered, ", ")
}

func headerValue(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}
