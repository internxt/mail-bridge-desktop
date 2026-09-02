package store

import (
	"encoding/base64"
	"fmt"
)

const privateKeyLen = 32

// EncryptionPrivateKey returns the account's private key as raw bytes, ready
// for internal/crypto.
//
// It is stored base64-encoded
func EncryptionPrivateKey(s *Store) ([]byte, error) {
	encoded, err := s.Get(KeyEncryptionPrivateKey)
	if err != nil {
		return nil, err
	}

	key, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return nil, fmt.Errorf("store: decode %s: %w", KeyEncryptionPrivateKey, err)
	}
	if len(key) != privateKeyLen {
		return nil, fmt.Errorf("store: %s is %d bytes, want %d", KeyEncryptionPrivateKey, len(key), privateKeyLen)
	}

	return key, nil
}

// PublicKey returns the account's hybrid public key as raw bytes.
func PublicKey(s *Store) ([]byte, error) {
	encoded, err := s.Get(KeyPublicKey)
	if err != nil {
		return nil, err
	}

	key, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return nil, fmt.Errorf("store: decode %s: %w", KeyPublicKey, err)
	}

	return key, nil
}
