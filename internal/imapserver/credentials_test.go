package imapserver

import (
	"bytes"
	"testing"

	"mail-bridge-desktop/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewForTesting(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return s
}

// TestEnsureStoragePassphrasePersists matters because Gluon's cache is
// encrypted with this value: a new one on every start leaves the cache
// unreadable and forces a full resynchronisation.
func TestEnsureStoragePassphrasePersists(t *testing.T) {
	s := newTestStore(t)

	first, err := EnsureStoragePassphrase(s)
	if err != nil {
		t.Fatalf("EnsureStoragePassphrase: %v", err)
	}
	second, err := EnsureStoragePassphrase(s)
	if err != nil {
		t.Fatalf("EnsureStoragePassphrase again: %v", err)
	}

	if !bytes.Equal(first, second) {
		t.Error("the passphrase changed between calls; the message cache would be unreadable")
	}
	if len(first) != storagePassphraseBytes {
		t.Errorf("passphrase is %d bytes, want %d", len(first), storagePassphraseBytes)
	}
}

func TestEnsureStoragePassphraseRequiresStore(t *testing.T) {
	if _, err := EnsureStoragePassphrase(nil); err == nil {
		t.Fatal("expected an error, got nil")
	}
}
