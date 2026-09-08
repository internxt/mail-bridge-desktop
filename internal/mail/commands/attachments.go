package commands

import (
	"context"
	"errors"
	"fmt"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/crypto"
)

func DownloadAttachments(ctx context.Context, client Client, token string, email api.EmailResponseDto, account Account, onError func(error)) (map[string][]byte, error) {
	if len(email.Attachments) == 0 {
		return nil, nil
	}

	sessionKey, err := attachmentsSessionKey(email, account)
	if err != nil {
		return nil, err
	}

	blobs := make(map[string][]byte, len(email.Attachments))
	for _, attachment := range email.Attachments {
		content, err := client.DownloadAttachment(ctx, token, email.Id, attachment.BlobId)
		if err != nil {
			onError(fmt.Errorf("download attachment %s of %s: %w", attachment.Name, email.Id, err))
			continue
		}

		if len(sessionKey) > 0 {
			content, err = crypto.DecryptSymmetrically(sessionKey, content, nil)
			if err != nil {
				onError(fmt.Errorf("decrypt attachment %s of %s: %w", attachment.Name, email.Id, err))
				continue
			}
		}

		blobs[attachment.BlobId] = content
	}

	return blobs, nil
}

func attachmentsSessionKey(email api.EmailResponseDto, account Account) ([]byte, error) {
	text := deref(email.TextBody)
	if !crypto.IsEncryptedBody(text) {
		return nil, nil
	}
	if !account.ready() {
		return nil, errors.New("decrypt attachments: no account keys available")
	}

	envelope, err := crypto.ParseEnvelope(text)
	if err != nil {
		return nil, fmt.Errorf("decrypt attachments of %s: %w", email.Id, err)
	}

	opened, err := crypto.DecryptEnvelope(envelope, account.PrivateKey, account.Address)
	if err != nil {
		return nil, fmt.Errorf("decrypt attachments of %s: %w", email.Id, err)
	}
	return opened.AttachmentsSessionKey, nil
}
