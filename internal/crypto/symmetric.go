package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"errors"
	"fmt"
)

const ivLen = 12

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
