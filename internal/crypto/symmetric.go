package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

const ivLen = 12

// aesGCM is AES in Galois/Counter Mode: authenticated encryption with one
// key, in the ciphertext‖tag‖iv layout the JS client reads. The IV is
// generated fresh on every Seal and travels at the end of the payload, not
// the front — that is what the JS library does, so that is what this reads.
type aesGCM struct {
	aead cipher.AEAD
}

func newAESGCM(key []byte) (aesGCM, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return aesGCM{}, fmt.Errorf("crypto: build cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return aesGCM{}, fmt.Errorf("crypto: build GCM: %w", err)
	}

	return aesGCM{aead: aead}, nil
}

// Seal encrypts plaintext, authenticating aux alongside it without including
// it in the output. The random IV it generates travels at the end of the
// returned payload.
func (c aesGCM) Seal(plaintext, aux []byte) ([]byte, error) {
	iv := make([]byte, ivLen)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("crypto: generate IV: %w", err)
	}

	ciphertext := c.aead.Seal(nil, iv, plaintext, aux)
	return append(ciphertext, iv...), nil
}

// Open reverses Seal: it reads the IV from the end of encrypted, then
// authenticates and decrypts the rest against aux.
func (c aesGCM) Open(encrypted, aux []byte) ([]byte, error) {
	if len(encrypted) < ivLen {
		return nil, fmt.Errorf("crypto: encrypted payload is %d bytes, shorter than the %d-byte IV", len(encrypted), ivLen)
	}

	split := len(encrypted) - ivLen
	ciphertext, iv := encrypted[:split], encrypted[split:]

	plaintext, err := c.aead.Open(nil, iv, ciphertext, aux)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt symmetrically: %w", err)
	}

	return plaintext, nil
}

// EncryptSymmetrically is the mirror of DecryptSymmetrically: AES-GCM with a
// random IV, appended after the ciphertext to match the layout the JS client
// reads.
func EncryptSymmetrically(key, plaintext, aux []byte) ([]byte, error) {
	cipher, err := newAESGCM(key)
	if err != nil {
		return nil, err
	}
	return cipher.Seal(plaintext, aux)
}

func DecryptSymmetrically(key, encrypted, aux []byte) ([]byte, error) {
	cipher, err := newAESGCM(key)
	if err != nil {
		return nil, err
	}
	return cipher.Open(encrypted, aux)
}
