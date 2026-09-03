package commands

import (
	"context"
	"fmt"

	"mail-bridge-desktop/internal/api"
)

// MarkRead marks emails as read or unread.
func MarkRead(ctx context.Context, client Client, token string, emailIDs []string, read bool) error {
	return updateEach(ctx, client, token, emailIDs, api.UpdateEmailRequestDto{IsRead: &read})
}

// MarkFlagged flags or unflags emails.
func MarkFlagged(ctx context.Context, client Client, token string, emailIDs []string, flagged bool) error {
	return updateEach(ctx, client, token, emailIDs, api.UpdateEmailRequestDto{IsFlagged: &flagged})
}

// Move puts emails in another mailbox.
func Move(ctx context.Context, client Client, token string, emailIDs []string, mailbox api.Mailbox) error {
	destination, err := updateMailbox(mailbox)
	if err != nil {
		return err
	}
	return updateEach(ctx, client, token, emailIDs, api.UpdateEmailRequestDto{Mailbox: &destination})
}

// Delete removes emails for good.
func Delete(ctx context.Context, client Client, token string, emailIDs []string) error {
	for _, emailID := range emailIDs {
		if err := client.DeleteEmail(ctx, token, emailID); err != nil {
			return fmt.Errorf("delete email %s: %w", emailID, err)
		}
	}
	return nil
}

func updateEach(ctx context.Context, client Client, token string, emailIDs []string, update api.UpdateEmailRequestDto) error {
	for _, emailID := range emailIDs {
		if err := client.UpdateEmail(ctx, token, emailID, update); err != nil {
			return fmt.Errorf("update email %s: %w", emailID, err)
		}
	}
	return nil
}

// updateMailbox translates a mailbox type into the one the update endpoint
// accepts. They carry the same values under different generated types.
func updateMailbox(mailbox api.Mailbox) (api.UpdateEmailRequestDtoMailbox, error) {
	switch mailbox {
	case api.MailboxInbox, api.MailboxDrafts, api.MailboxSent,
		api.MailboxTrash, api.MailboxSpam, api.MailboxArchive:
		return api.UpdateEmailRequestDtoMailbox(mailbox), nil
	default:
		return "", fmt.Errorf("cannot move mail to %q", mailbox)
	}
}
