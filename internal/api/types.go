package api

type Mailbox = MailboxResponseDtoType

const (
	MailboxInbox   = MailboxResponseDtoTypeInbox
	MailboxDrafts  = MailboxResponseDtoTypeDrafts
	MailboxSent    = MailboxResponseDtoTypeSent
	MailboxTrash   = MailboxResponseDtoTypeTrash
	MailboxSpam    = MailboxResponseDtoTypeSpam
	MailboxArchive = MailboxResponseDtoTypeArchive
)

type ListEmailsOptions struct {
	Mailbox  Mailbox
	Limit    int
	Position int
	AnchorID string
	Unread   *bool
}
