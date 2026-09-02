package commands

import (
	"context"
	"fmt"

	"mail-bridge-desktop/internal/api"
)

// ListMailboxes returns the folders of the account.
func ListMailboxes(ctx context.Context, client Client, token string) ([]api.MailboxResponseDto, error) {
	mailboxes, err := client.GetMailboxes(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("list mailboxes: %w", err)
	}
	return mailboxes, nil
}
