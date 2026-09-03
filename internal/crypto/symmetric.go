package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
)

const ivLen = 12

// EncryptSymmetrically is the mirror of DecryptSymmetrically: AES-GCM with a
// random IV, appended after the ciphertext to match the layout the JS client
// reads.
func EncryptSymmetrically(key, plaintext, aux []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: build cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: build GCM: %w", err)
	}

	iv := make([]byte, ivLen)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("crypto: generate IV: %w", err)
	}

	ciphertext := aead.Seal(nil, iv, plaintext, aux)
	return append(ciphertext, iv...), nil
}

func DecryptSymmetrically(key, encrypted, aux []byte) ([]byte, error) {
	if len(encrypted) < ivLen {
		return nil, fmt.Errorf("crypto: encrypted payload is %d bytes, shorter than the %d-byte IV", len(encrypted), ivLen)
	}

	split := len(encrypted) - ivLen
	ciphertext, iv := encrypted[:split], encrypted[split:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: build cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: build GCM: %w", err)
	}

	plaintext, err := aead.Open(nil, iv, ciphertext, aux)
	if err != nil {

		return nil, fmt.Errorf("crypto: decrypt symmetrically: %w", err)
	}

	return plaintext, nil
}

var aeskwIV = [8]byte{0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6}

var ErrKeyUnwrap = errors.New("crypto: key unwrap integrity check failed")

// WrapKey is the mirror of UnwrapKey: RFC 3394 AES key wrap. It runs the same
// algorithm in the opposite direction — j ascending, encrypting instead of
// decrypting — and prepends the fixed integrity constant UnwrapKey checks for.
func WrapKey(key, wrappingKey []byte) ([]byte, error) {
	if len(key) < 16 || len(key)%8 != 0 {
		return nil, fmt.Errorf("crypto: key to wrap is %d bytes, want a multiple of 8 and at least 16", len(key))
	}

	block, err := aes.NewCipher(wrappingKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: build wrap cipher: %w", err)
	}

	n := len(key) / 8

	a := aeskwIV

	r := make([]byte, len(key))
	copy(r, key)

	var buf [16]byte
	for j := 0; j <= 5; j++ {
		for i := 1; i <= n; i++ {
			copy(buf[:8], a[:])
			copy(buf[8:], r[(i-1)*8:i*8])
			block.Encrypt(buf[:], buf[:])

			copy(a[:], buf[:8])
			t := uint64(n*j + i)
			for k := 0; k < 8; k++ {
				a[7-k] ^= byte(t >> (8 * k))
			}

			copy(r[(i-1)*8:i*8], buf[8:])
		}
	}

	return append(a[:], r...), nil
}

func UnwrapKey(encryptedKey, wrappingKey []byte) ([]byte, error) {
	if len(encryptedKey) < 24 || len(encryptedKey)%8 != 0 {
		return nil, fmt.Errorf("crypto: wrapped key is %d bytes, want a multiple of 8 and at least 24", len(encryptedKey))
	}

	block, err := aes.NewCipher(wrappingKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: build unwrap cipher: %w", err)
	}

	n := len(encryptedKey)/8 - 1

	var a [8]byte
	copy(a[:], encryptedKey[:8])

	r := make([]byte, len(encryptedKey)-8)
	copy(r, encryptedKey[8:])

	var buf [16]byte
	for j := 5; j >= 0; j-- {
		for i := n; i >= 1; i-- {
			t := uint64(n*j + i)

			for k := 0; k < 8; k++ {
				a[7-k] ^= byte(t >> (8 * k))
			}

			copy(buf[:8], a[:])
			copy(buf[8:], r[(i-1)*8:i*8])
			block.Decrypt(buf[:], buf[:])

			copy(a[:], buf[:8])
			copy(r[(i-1)*8:i*8], buf[8:])
		}
	}

	if subtle.ConstantTimeCompare(a[:], aeskwIV[:]) != 1 {
		return nil, ErrKeyUnwrap
	}

	return r, nil
}
