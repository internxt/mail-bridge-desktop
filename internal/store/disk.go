package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	dekSuffix   = ".dek"
	dekLenBytes = 32 // AES-256
)

// disk keeps values as AES-GCM encrypted files on disk. The key of
// each value lives in the keychain, so the files alone are useless.
type disk struct {
	dir  string
	keys backend
}

func (e disk) Get(key string) ([]byte, error) {
	sealed, err := os.ReadFile(e.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: read value: %w", err)
	}

	encoded, err := e.keys.Get(key + dekSuffix)
	if err != nil {
		return nil, fmt.Errorf("store: read encryption key: %w", err)
	}
	dek, err := hex.DecodeString(string(encoded))
	if err != nil {
		return nil, fmt.Errorf("store: decode encryption key: %w", err)
	}

	return open(dek, sealed)
}

func (e disk) Set(key string, value []byte) error {
	dek := make([]byte, dekLenBytes)
	if _, err := rand.Read(dek); err != nil {
		return fmt.Errorf("store: generate encryption key: %w", err)
	}

	sealed, err := seal(dek, value)
	if err != nil {
		return err
	}

	// The key goes in first: a file we cannot decrypt is useless, while a key
	// without a file is harmless.
	if err := e.keys.Set(key+dekSuffix, []byte(hex.EncodeToString(dek))); err != nil {
		return fmt.Errorf("store: save encryption key: %w", err)
	}
	return writeFileAtomic(e.path(key), sealed)
}

func (e disk) Remove(key string) error {
	if err := os.Remove(e.path(key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("store: remove value: %w", err)
	}
	return e.keys.Remove(key + dekSuffix)
}

// path is where the encrypted value of a key lives. The key is hashed so that
// key names never show up in the filesystem.
func (e disk) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(e.dir, hex.EncodeToString(sum[:])+".enc")
}

// seal encrypts plaintext with AES-GCM.
func seal(dek, plaintext []byte) ([]byte, error) {
	aead, err := newAEAD(dek)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("store: generate nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

// open reverses seal.
func open(dek, sealed []byte) ([]byte, error) {
	aead, err := newAEAD(dek)
	if err != nil {
		return nil, err
	}
	if len(sealed) < aead.NonceSize() {
		return nil, errors.New("store: stored value is truncated")
	}
	nonce, ciphertext := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("store: decrypt value: %w", err)
	}
	return plaintext, nil
}

func newAEAD(dek []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("store: create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("store: create gcm: %w", err)
	}
	return aead, nil
}

// writeFileAtomic writes through a temporary file and renames it into place,
// so an interrupted write never leaves a half-written value behind.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("store: create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(filePerm); err != nil {
		tmp.Close()
		return fmt.Errorf("store: set permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("store: write value: %w", err)
	}
	// Flush to disk before the rename, or a crash could leave an empty file.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("store: sync value: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("store: close value: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("store: replace value: %w", err)
	}
	return nil
}
