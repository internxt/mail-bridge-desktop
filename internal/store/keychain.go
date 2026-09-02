package store

import (
	"errors"
	"sync"

	"github.com/zalando/go-keyring"
)

// serviceName groups every entry the bridge owns in the system keychain.
const serviceName = "Internxt Mail Bridge"

type keychain struct{}

func (keychain) Get(key string) ([]byte, error) {
	v, err := keyring.Get(serviceName, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return []byte(v), nil
}

func (keychain) Set(key string, value []byte) error {
	return keyring.Set(serviceName, key, string(value))
}

func (keychain) Remove(key string) error {
	err := keyring.Delete(serviceName, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// memoryKeychain is an in-memory stand-in used by tests, so they never touch
// the real keychain (which does not exist on CI machines).
//
// The real keychain is safe to use from several goroutines, so this one locks
// too: without it a test exercising concurrent access would fail under -race
// for a reason that has nothing to do with what it is testing.
type memoryKeychain struct {
	mu     sync.Mutex
	values map[string][]byte
}

func newMemoryKeychain() *memoryKeychain {
	return &memoryKeychain{values: map[string][]byte{}}
}

func (m *memoryKeychain) Get(key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.values[key]
	if !ok {
		return nil, ErrNotFound
	}
	return v, nil
}

func (m *memoryKeychain) Set(key string, value []byte) error {
	if len(value) > keychainMaxBytes {
		return errors.New("store: value too big for the keychain")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.values[key] = value
	return nil
}

func (m *memoryKeychain) Remove(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.values, key)
	return nil
}
