package commands

import (
	"errors"
	"fmt"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/crypto"
)

// Account is the identity a mailbox is read as: the address emails were
// encrypted for, and the private key that opens them.
//
// A zero Account disables decryption, which is what lets the bridge run against
// a plaintext account, or before the desktop client has sent the keys over IPC.
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

	return email, nil
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
