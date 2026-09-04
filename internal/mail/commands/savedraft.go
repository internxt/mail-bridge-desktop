package commands

import (
	"context"
	"errors"
	"fmt"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/crypto"
)

func SaveDraft(ctx context.Context, client Client, token string, msg OutgoingMessage, account Account) (api.EmailResponseDto, error) {
	if account.Address == "" || len(account.PublicKey) == 0 {
		return api.EmailResponseDto{}, errors.New("save draft: no account key to seal the draft with")
	}

	envelope, err := crypto.BuildEnvelope(msg.body(), []crypto.Recipient{
		{Address: account.Address, PublicKey: account.PublicKey},
	})
	if err != nil {
		return api.EmailResponseDto{}, fmt.Errorf("save draft: seal message: %w", err)
	}

	block := toEncryptionBlock(envelope)
	draft, err := client.SaveDraft(ctx, token, api.DraftEmailRequestDto{
		Subject:    optionalString(msg.Subject),
		Encryption: &block,
		To:         optionalAddresses(msg.To),
		Cc:         optionalAddresses(msg.Cc),
		Bcc:        optionalAddresses(msg.Bcc),
	})
	if err != nil {
		return api.EmailResponseDto{}, fmt.Errorf("save draft: %w", err)
	}
	return draft, nil
}

func DiscardDrafts(ctx context.Context, client Client, token string, draftIDs []string) error {
	for _, draftID := range draftIDs {
		if err := client.DiscardDraft(ctx, token, draftID); err != nil {
			return fmt.Errorf("discard draft %s: %w", draftID, err)
		}
	}
	return nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
