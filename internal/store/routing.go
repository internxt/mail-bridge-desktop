package store

import (
	"errors"
	"fmt"
)

const keychainMaxBytes = 3000

type router struct {
	keychain backend
	disk     backend
}

func (r router) backendFor(size int) backend {
	if size <= keychainMaxBytes {
		return r.keychain
	}
	return r.disk
}

func (r router) Get(key string) ([]byte, error) {
	value, err := r.keychain.Get(key)
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("store: read value: %w", err)
	}
	return r.disk.Get(key)
}

func (r router) Set(key string, value []byte) error {
	if err := r.Remove(key); err != nil {
		return err
	}
	return r.backendFor(len(value)).Set(key, value)
}

func (r router) Remove(key string) error {
	return errors.Join(
		r.keychain.Remove(key),
		r.disk.Remove(key),
	)
}
