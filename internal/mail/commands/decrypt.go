package commands

import (
	"errors"
	"fmt"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/crypto"
)

type Account struct {
	Address    string
	PrivateKey []byte
}

// ready reports whether the account can decrypt anything.
func (a Account) ready() bool {
	return a.Address != "" && len(a.PrivateKey) > 0
}

// decryptBody replaces an email's encrypted envelope with the cleartext body.
//
// Encrypted emails carry their envelope in the text body, behind a marker,
// while the subject travels in the clear so the backend can index it. Doing the
// substitution here means everything downstream — the MIME builder, the IMAP
// connector — only ever sees a normal email.
//
// A body that cannot be decrypted is left as it is rather than failing the
// call: a message encrypted for another address, or one that arrives before the
// keys do, should still show up in the client. The error is returned alongside
// so the caller can log it.
func decryptBody(email api.EmailResponseDto, account Account) (api.EmailResponseDto, error) {
	text := deref(email.TextBody)
	if !crypto.IsEncryptedBody(text) {
		return email, nil
	}
	if !account.ready() {
		return email, errors.New("decrypt body: no account keys available")
	}

	envelope, err := crypto.ParseEnvelope(text)
	if err != nil {
		return email, fmt.Errorf("decrypt body of %s: %w", email.Id, err)
	}

	decrypted, err := crypto.DecryptEnvelope(envelope, account.PrivateKey, account.Address)
	if err != nil {
		return email, fmt.Errorf("decrypt body of %s: %w", email.Id, err)
	}

	// The envelope holds one body. It is served as HTML because that is what
	// the web client composes, and the HTML slot is what mail clients prefer.
	email.TextBody = nil
	email.HtmlBody = &decrypted.Text

	if decrypted.Preview != "" {
		email.Preview = decrypted.Preview
		email.TextBody = &decrypted.Preview
	}

	return email, nil
}

// decryptPreview replaces a listed email's encrypted preview with its text.
//
// It mirrors decryptBody's contract: a preview that cannot be opened leaves the
// summary as it arrived and reports why, so a message encrypted for another
// address still shows up in the listing.
//
// This is separate from decryptBody because a listing carries no envelope. The
// preview travels in its own block, wrapped with the same keys, which is what
// lets a mailbox be listed without downloading every message in it.
func decryptPreview(summary api.EmailSummaryResponseDto, account Account) (api.EmailSummaryResponseDto, error) {
	if summary.Encryption == nil || summary.Encryption.EncryptedPreview == "" {
		return summary, nil
	}
	if !account.ready() {
		return summary, errors.New("decrypt preview: no account keys available")
	}

	preview, err := crypto.DecryptPreview(
		wrappedKeys(summary.Encryption.WrappedKeys),
		summary.Encryption.EncryptedPreview,
		account.PrivateKey,
		account.Address,
	)
	if err != nil {
		return summary, fmt.Errorf("decrypt preview of %s: %w", summary.Id, err)
	}

	summary.Preview = preview
	return summary, nil
}

func wrappedKeys(keys []api.EncryptedWrappedKeyDto) []crypto.WrappedKey {
	converted := make([]crypto.WrappedKey, 0, len(keys))
	for _, key := range keys {
		converted = append(converted, crypto.WrappedKey{
			EncryptedForEmail: key.EncryptedForEmail,
			EncryptedKey:      key.EncryptedKey,
			HybridCiphertext:  key.HybridCiphertext,
		})
	}
	return converted
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
