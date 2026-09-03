// Package commands holds one file per mail operation.
//
// Each command is a plain function taking everything it needs, so it can be
// tested without a store or a running daemon. Resolving the account token is
// the job of internal/mail, which composes these.
package commands

import (
	"context"

	"mail-bridge-desktop/internal/api"
)

// Client is the part of the API client the commands use. It is an interface so
// tests can stand in for it.
type Client interface {
	GetUserFolder(ctx context.Context, token string, opts api.ListEmailsOptions) (api.EmailListResponseDto, error)
	GetMailboxes(ctx context.Context, token string) ([]api.MailboxResponseDto, error)
	GetThread(ctx context.Context, token, threadID string) ([]api.EmailResponseDto, error)
	UpdateEmail(ctx context.Context, token, emailID string, update api.UpdateEmailRequestDto) error
	DeleteEmail(ctx context.Context, token, emailID string) error
}
