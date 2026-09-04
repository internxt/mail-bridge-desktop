package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"errors"
	"fmt"
)

var aeskwIV = [8]byte{0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6}

var ErrKeyUnwrap = errors.New("crypto: key unwrap integrity check failed")

// aesKeyWrap is AES Key Wrap, RFC 3394: a cipher specialised in wrapping keys
// rather than messages — 32 bytes in, 40 bytes out, no IV, no separate tag.
// Its own integrity check is reproducing a fixed constant on unwrap: with the
// right key it always does, with the wrong one the odds are 1 in 2⁶⁴.
type aesKeyWrap struct {
	block cipher.Block
}

func newAESKeyWrap(wrappingKey []byte) (aesKeyWrap, error) {
	block, err := aes.NewCipher(wrappingKey)
	if err != nil {
		return aesKeyWrap{}, fmt.Errorf("crypto: build wrap cipher: %w", err)
	}
	return aesKeyWrap{block: block}, nil
}

// Wrap seals key under this wrapper's key, running the algorithm forwards:
// j ascending, encrypting one 8-byte block at a time.
func (w aesKeyWrap) Wrap(key []byte) ([]byte, error) {
	if len(key) < 16 || len(key)%8 != 0 {
		return nil, fmt.Errorf("crypto: key to wrap is %d bytes, want a multiple of 8 and at least 16", len(key))
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
			w.block.Encrypt(buf[:], buf[:])

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

// Unwrap reverses Wrap: the same algorithm run backwards, j descending and
// decrypting, ending in a check that the integrity constant came back intact.
func (w aesKeyWrap) Unwrap(wrapped []byte) ([]byte, error) {
	if len(wrapped) < 24 || len(wrapped)%8 != 0 {
		return nil, fmt.Errorf("crypto: wrapped key is %d bytes, want a multiple of 8 and at least 24", len(wrapped))
	}

	n := len(wrapped)/8 - 1

	var a [8]byte
	copy(a[:], wrapped[:8])

	r := make([]byte, len(wrapped)-8)
	copy(r, wrapped[8:])

	var buf [16]byte
	for j := 5; j >= 0; j-- {
		for i := n; i >= 1; i-- {
			t := uint64(n*j + i)

			for k := 0; k < 8; k++ {
				a[7-k] ^= byte(t >> (8 * k))
			}

			copy(buf[:8], a[:])
			copy(buf[8:], r[(i-1)*8:i*8])
			w.block.Decrypt(buf[:], buf[:])

			copy(a[:], buf[:8])
			copy(r[(i-1)*8:i*8], buf[8:])
		}
	}

	if subtle.ConstantTimeCompare(a[:], aeskwIV[:]) != 1 {
		return nil, ErrKeyUnwrap
	}

	return r, nil
}

// WrapKey is the mirror of UnwrapKey: RFC 3394 AES key wrap.
func WrapKey(key, wrappingKey []byte) ([]byte, error) {
	wrapper, err := newAESKeyWrap(wrappingKey)
	if err != nil {
		return nil, err
	}
	return wrapper.Wrap(key)
}

func UnwrapKey(encryptedKey, wrappingKey []byte) ([]byte, error) {
	wrapper, err := newAESKeyWrap(wrappingKey)
	if err != nil {
		return nil, err
	}
	return wrapper.Unwrap(encryptedKey)
}
