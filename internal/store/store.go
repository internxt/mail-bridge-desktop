package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
)

const (
	dirPerm  os.FileMode = 0o700
	filePerm os.FileMode = 0o600
	indexKey             = "__index"
)

type Store struct {
	mu      sync.RWMutex
	backend backend
}

func New(stateDir string) (*Store, error) {
	if stateDir == "" {
		return nil, errors.New("store: state directory is required")
	}
	return newWithKeychain(stateDir, keychain{})
}

func newWithKeychain(stateDir string, kc backend) (*Store, error) {
	// If path is already a directory, MkdirAll does nothing and returns nil.
	if err := os.MkdirAll(stateDir, dirPerm); err != nil {
		return nil, fmt.Errorf("store: create state directory: %w", err)
	}
	return &Store{
		backend: router{
			keychain: kc,
			disk:     disk{dir: stateDir, keys: kc},
		},
	}, nil
}

func (s *Store) Set(key string, value []byte) error {
	if key == "" {
		return errors.New("store: key is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.backend.Set(key, value); err != nil {
		return err
	}
	return s.addToIndex(key)
}

func (s *Store) Get(key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.backend.Get(key)
}

func (s *Store) Remove(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.backend.Remove(key); err != nil {
		return err
	}
	return s.dropFromIndex(key)
}

func (s *Store) Keys() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.index()
}

func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys, err := s.index()
	if err != nil {
		return err
	}

	var errs []error
	for _, key := range keys {
		if err := s.backend.Remove(key); err != nil {
			errs = append(errs, err)
		}
	}
	if err := s.backend.Remove(indexKey); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *Store) index() ([]string, error) {
	raw, err := s.backend.Get(indexKey)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: read index: %w", err)
	}
	var keys []string
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, fmt.Errorf("store: decode index: %w", err)
	}
	return keys, nil
}

func (s *Store) addToIndex(key string) error {
	keys, err := s.index()
	if err != nil {
		return err
	}
	if slices.Contains(keys, key) {
		return nil
	}
	return s.saveIndex(append(keys, key))
}

func (s *Store) dropFromIndex(key string) error {
	keys, err := s.index()
	if err != nil {
		return err
	}
	kept := slices.DeleteFunc(keys, func(k string) bool { return k == key })
	return s.saveIndex(kept)
}

func (s *Store) saveIndex(keys []string) error {
	raw, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("store: encode index: %w", err)
	}
	if err := s.backend.Set(indexKey, raw); err != nil {
		return fmt.Errorf("store: save index: %w", err)
	}
	return nil
}

func SetJSON(s *Store, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("store: encode %s: %w", key, err)
	}
	return s.Set(key, raw)
}

func GetJSON(s *Store, key string, v any) error {
	raw, err := s.Get(key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("store: decode %s: %w", key, err)
	}
	return nil
}
