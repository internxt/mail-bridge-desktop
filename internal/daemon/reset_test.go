package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mail-bridge-desktop/internal/store"
)

// TestResetClearsTheStoreAndRemovesTheStateDirectory is what a logout relies
// on: nothing the bridge stored for the account survives.
func TestResetClearsTheStoreAndRemovesTheStateDirectory(t *testing.T) {
	dir := t.TempDir()
	credentials, err := store.NewForTesting(dir, nil)
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	if err := credentials.Set("token", []byte("secret")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := reset(dir, credentials, nil); err != nil {
		t.Fatalf("reset: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("state directory still exists after reset")
	}
}

// TestResetOnAnAlreadyEmptyStoreSucceeds covers the account that was never
// really used: there is nothing to clear, and that is still success.
func TestResetOnAnAlreadyEmptyStoreSucceeds(t *testing.T) {
	dir := t.TempDir()
	credentials, err := store.NewForTesting(dir, nil)
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}

	if err := reset(dir, credentials, nil); err != nil {
		t.Fatalf("reset: %v", err)
	}
}

// TestResetOnAMissingStateDirectorySucceeds is the other half of "missing is
// success": removing a state directory that is already gone is not a failure.
func TestResetOnAMissingStateDirectorySucceeds(t *testing.T) {
	dir := t.TempDir()
	credentials, err := store.NewForTesting(dir, nil)
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	if err := reset(dir, credentials, nil); err != nil {
		t.Fatalf("reset: %v", err)
	}
}

// TestResetStillRemovesTheDirectoryWhenTheStoreFailedToOpen is what keeps a
// broken store from leaving stray state behind: the directory removal still
// runs, and the open failure is reported rather than swallowed.
func TestResetStillRemovesTheDirectoryWhenTheStoreFailedToOpen(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "state")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := reset(dir, nil, errors.New("store unavailable"))
	if err == nil {
		t.Fatal("reset: want an error reporting the store could not be opened")
	}

	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("state directory still exists after a failed reset")
	}
}
