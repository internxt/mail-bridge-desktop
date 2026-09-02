// Package mail sits between the transport and the storage layers: it holds
// the account's session and calls the Mail API with it.
//
// Keeping this here is what lets internal/api stay pure transport and
// internal/crypto stay pure cryptography: neither has to know where the
// account's token or keys come from.
//
// The operations themselves live in the commands subpackage, one per file.
package mail

import (
	"context"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/logger"
	"mail-bridge-desktop/internal/mail/commands"
)

// Account is the session the bridge acts for. It arrives from the parent over
// the control channel and is held in memory only: nothing here is persisted,
// so signing out is a matter of dropping the service.
type Account struct {
	// Token authenticates every Mail API call.
	Token string

	// Address is the mailbox being served.
	Address string

	// PrivateKey is the account's mail key, already decrypted: the 32-byte
	// root seed. Empty until bodies are decrypted, which is a later step.
	PrivateKey []byte
}

// MailService turns an account session into Mail API calls.
type MailService struct {
	api     commands.Client
	account Account
	log     *logger.Logger
}

func New(client commands.Client, account Account, log *logger.Logger) *MailService {
	return &MailService{api: client, account: account, log: log}
}

// ListMailboxes returns the account's folders.
func (s *MailService) ListMailboxes(ctx context.Context) ([]api.MailboxResponseDto, error) {
	return commands.ListMailboxes(ctx, s.api, s.account.Token)
}

// ListEmails returns one page of email summaries from a folder.
func (s *MailService) ListEmails(ctx context.Context, opts api.ListEmailsOptions) (api.EmailListResponseDto, error) {
	return commands.ListEmails(ctx, s.api, s.account.Token, opts)
}

// ListAllEmails returns every email in a folder, paging through the API.
func (s *MailService) ListAllEmails(ctx context.Context, opts api.ListEmailsOptions) ([]api.EmailSummaryResponseDto, error) {
	return commands.ListAllEmails(ctx, s.api, s.account.Token, opts)
}
