package store

import (
	"errors"

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
type memoryKeychain struct {
	values map[string][]byte
}

func newMemoryKeychain() *memoryKeychain {
	return &memoryKeychain{values: map[string][]byte{}}
}

func (m *memoryKeychain) Get(key string) ([]byte, error) {
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
	m.values[key] = value
	return nil
}

func (m *memoryKeychain) Remove(key string) error {
	delete(m.values, key)
	return nil
}
