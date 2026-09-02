package development

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"mail-bridge-desktop/internal/control"
)

const (
	passwordBytes = 32
	// TODO: Create the password here, not in the parent.
	passwordFile = "dev-mail-password"
)

// SessionFromEnv builds the session the real parent would send, from the .env.
//
// The password is generated here and kept in stateDir, because the parent owns
// the mail client's credentials in this protocol. Reusing it is what stops a
// configured mail client from breaking on every restart.
func SessionFromEnv(stateDir string) (control.Session, error) {
	address := os.Getenv("BRIDGE_DEV_EMAIL")
	if address == "" {
		return control.Session{}, errors.New("BRIDGE_DEV_EMAIL is not set")
	}

	backend, err := json.Marshal(control.BackendSession{
		Token:                os.Getenv("BRIDGE_DEV_TOKEN"),
		EncryptionPrivateKey: os.Getenv("BRIDGE_DEV_ENCRYPTION_PRIVATE_KEY"),
		PublicKey:            os.Getenv("BRIDGE_DEV_PUBLIC_KEY"),
	})
	if err != nil {
		return control.Session{}, fmt.Errorf("encode backend session: %w", err)
	}

	password, err := ensurePassword(stateDir)
	if err != nil {
		return control.Session{}, err
	}

	return control.Session{
		AccountID:      address,
		Addresses:      []string{address},
		BackendSession: backend,
		MailClient: control.MailClient{
			Username: address,
			Password: password,
		},
	}, nil
}

// ensurePassword reads the stored mail password, generating it the first time.
func ensurePassword(stateDir string) (string, error) {
	path := filepath.Join(stateDir, passwordFile)

	stored, err := os.ReadFile(path)
	switch {
	case err == nil && len(stored) > 0:
		return string(stored), nil
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return "", fmt.Errorf("read development password: %w", err)
	}

	generated, err := randomPassword()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", fmt.Errorf("create state directory: %w", err)
	}
	// 0o600: it is a credential, even a development one.
	if err := os.WriteFile(path, []byte(generated), 0o600); err != nil {
		return "", fmt.Errorf("store development password: %w", err)
	}

	return generated, nil
}

func randomPassword() (string, error) {
	value := make([]byte, passwordBytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate development password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
