// Package mail sits between the transport and the storage layers: it reads
// the account's credentials from the store, calls the Mail API with them and,
// in time, decrypts what comes back.
//
// Keeping this here is what lets internal/api stay pure transport and
// internal/crypto stay pure cryptography: neither has to know where the user's
// token or mnemonic live.
package mail

import (
	"context"
	"encoding/json"
	"fmt"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/store"
)

// Service turns stored credentials into Mail API calls.
type Service struct {
	api   *api.Client
	store *store.Store
}

func New(client *api.Client, credentials *store.Store) *Service {
	return &Service{api: client, store: credentials}
}

// ListEmails returns one page of emails from a folder.
//
// TODO(crypto): decrypt the encrypted summaries before returning them.
func (s *Service) ListEmails(ctx context.Context, opts api.ListEmailsOptions) (api.EmailListResponseDto, error) {
	token, err := s.token()
	if err != nil {
		return api.EmailListResponseDto{}, err
	}
	userFolder, err := s.api.GetUserFolder(ctx, token, opts)

	pretty, _ := json.MarshalIndent(userFolder.Emails, "", "  ")
	fmt.Printf("%s\n", pretty)

	return userFolder, err
}

// token reads the account token from the store.
func (s *Service) token() (string, error) {
	value, err := s.store.Get(store.KeyToken)
	if err != nil {
		return "", fmt.Errorf("mail: read token: %w", err)
	}
	return string(value), nil
}
