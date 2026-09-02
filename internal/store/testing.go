package store

// TestKeychain stands in for the system keychain in tests from other packages,
// which must never touch the real one: it holds the developer's own
// credentials, and it does not exist at all on CI machines.
//
// Reuse one across two stores to model a restart, where the process is new but
// the keychain still holds what the previous run wrote.
type TestKeychain struct {
	backend
}

// NewTestKeychain returns an empty in-memory keychain.
func NewTestKeychain() *TestKeychain {
	return &TestKeychain{backend: newMemoryKeychain()}
}

// NewForTesting builds a store backed by the given keychain. Pass a
// t.TempDir() as stateDir: values too large for a keychain still land there.
func NewForTesting(stateDir string, keychain *TestKeychain) (*Store, error) {
	if keychain == nil {
		keychain = NewTestKeychain()
	}
	return newWithKeychain(stateDir, keychain)
}
