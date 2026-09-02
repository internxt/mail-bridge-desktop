package imapserver

import (
	"errors"
	"fmt"

	"mail-bridge-desktop/internal/store"
)

const (
	// storagePassphraseBytes is the key size Gluon expects for its cache.
	storagePassphraseBytes = 32
)

// EnsureStoragePassphrase returns the passphrase encrypting Gluon's message
// cache, generating and storing it the first time.
//
// This one must persist: the cache on disk is encrypted with it,
// so a new passphrase leaves the previous cache unreadable and forces
// a full resynchronisation of every mailbox.
func EnsureStoragePassphrase(credentials *store.Store) ([]byte, error) {
	if credentials == nil {
		return nil, errors.New("imapserver: credential store is required")
	}

	passphrase, err := credentials.Get(store.KeyStoragePassphrase)
	switch {
	case err == nil:
		return passphrase, nil
	case !errors.Is(err, store.ErrNotFound):
		return nil, fmt.Errorf("imapserver: read storage passphrase: %w", err)
	}

	generated, err := randomBytes(storagePassphraseBytes)
	if err != nil {
		return nil, fmt.Errorf("imapserver: generate storage passphrase: %w", err)
	}
	if err := credentials.Set(store.KeyStoragePassphrase, generated); err != nil {
		return nil, fmt.Errorf("imapserver: store storage passphrase: %w", err)
	}

	return generated, nil
}
