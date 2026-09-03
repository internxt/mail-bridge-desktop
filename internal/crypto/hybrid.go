package crypto

import (
	"crypto/ecdh"
	"crypto/mlkem"
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/sha3"
)

const (
	seedLen             = 32
	mlkemSeedLen        = 64
	x25519KeyLen        = 32
	mlkemCiphertextLen  = 1088
	hybridCiphertextLen = mlkemCiphertextLen + x25519KeyLen
)

const combinerLabel = `\.//^\`

func DecapsulateHybrid(ciphertext, seed []byte) ([]byte, error) {
	if len(seed) != seedLen {
		return nil, fmt.Errorf("crypto: hybrid seed is %d bytes, want %d", len(seed), seedLen)
	}
	if len(ciphertext) != hybridCiphertextLen {
		return nil, fmt.Errorf("crypto: hybrid ciphertext is %d bytes, want %d", len(ciphertext), hybridCiphertextLen)
	}

	mlkemSeed, x25519Secret := expandHybridSeed(seed)
	mlkemCiphertext := ciphertext[:mlkemCiphertextLen]
	x25519Ciphertext := ciphertext[mlkemCiphertextLen:]

	decapsulationKey, err := mlkem.NewDecapsulationKey768(mlkemSeed)
	if err != nil {
		return nil, fmt.Errorf("crypto: load ML-KEM key: %w", err)
	}
	mlkemShared, err := decapsulationKey.Decapsulate(mlkemCiphertext)
	if err != nil {
		return nil, fmt.Errorf("crypto: ML-KEM decapsulate: %w", err)
	}

	// X25519 half. The "ciphertext" is the sender's ephemeral public key, and
	// the shared secret is the plain Diffie-Hellman output.
	privateKey, err := ecdh.X25519().NewPrivateKey(x25519Secret)
	if err != nil {
		return nil, fmt.Errorf("crypto: load X25519 key: %w", err)
	}
	ephemeralKey, err := ecdh.X25519().NewPublicKey(x25519Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("crypto: load X25519 ciphertext: %w", err)
	}
	x25519Shared, err := privateKey.ECDH(ephemeralKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: X25519 exchange: %w", err)
	}

	// The combiner binds both secrets to the X25519 transcript. The public key
	// is recomputed from the private one rather than carried alongside,
	// matching what noble does during decapsulation.
	var input []byte
	input = append(input, mlkemShared...)
	input = append(input, x25519Shared...)
	input = append(input, x25519Ciphertext...)
	input = append(input, privateKey.PublicKey().Bytes()...)
	input = append(input, combinerLabel...)

	shared := sha3.Sum256(input)
	return shared[:], nil
}

// expandHybridSeed derives the two component keys from the root seed with
// SHAKE256, the way noble's expandSeedXof does: one XOF output, split by
// component in registration order (ML-KEM first, then X25519).
func expandHybridSeed(seed []byte) (mlkemSeed, x25519Secret []byte) {
	expanded := make([]byte, mlkemSeedLen+x25519KeyLen)
	sha3.ShakeSum256(expanded, seed)
	return expanded[:mlkemSeedLen], expanded[mlkemSeedLen:]
}

// HybridEncryptedKey is a session key wrapped for one recipient.
type HybridEncryptedKey struct {
	HybridCiphertext []byte
	EncryptedKey     []byte
}

// DecryptKeysHybrid unwraps an email's symmetric key:
// decapsulate to a shared secret, then use it as the AES-KW wrapping key.
func DecryptKeysHybrid(encryptedKey HybridEncryptedKey, recipientPrivateKey []byte) ([]byte, error) {
	sharedSecret, err := DecapsulateHybrid(encryptedKey.HybridCiphertext, recipientPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: hybrid decryption: %w", err)
	}

	key, err := UnwrapKey(encryptedKey.EncryptedKey, sharedSecret)
	if err != nil {
		return nil, fmt.Errorf("crypto: hybrid decryption: %w", err)
	}

	return key, nil
}

const (
	mlkemPublicKeyLen  = 1184
	x25519PublicKeyLen = 32
	hybridPublicKeyLen = mlkemPublicKeyLen + x25519PublicKeyLen
)

// EncapsulateHybrid agrees on a secret with a recipient, knowing only their
// public key: out comes a shared secret, plus a ciphertext only their private
// key can open.
//
// publicKey is the recipient's 1216-byte hybrid public key (1184 bytes
// ML-KEM-768 followed by 32 bytes X25519), as published by the Mail API.
func EncapsulateHybrid(publicKey []byte) (ciphertext, sharedSecret []byte, err error) {
	if len(publicKey) != hybridPublicKeyLen {
		return nil, nil, fmt.Errorf("crypto: hybrid public key is %d bytes, want %d", len(publicKey), hybridPublicKeyLen)
	}

	mlkemPublicKey := publicKey[:mlkemPublicKeyLen]
	x25519PublicKey := publicKey[mlkemPublicKeyLen:]

	encapsulationKey, err := mlkem.NewEncapsulationKey768(mlkemPublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: load ML-KEM public key: %w", err)
	}
	mlkemShared, mlkemCiphertext := encapsulationKey.Encapsulate()

	recipientKey, err := ecdh.X25519().NewPublicKey(x25519PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: load X25519 public key: %w", err)
	}

	// The "ciphertext" on this half is the sender's own ephemeral public key:
	// the recipient recovers the same shared secret by running ECDH against
	// it with their private key.
	ephemeralKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: generate ephemeral X25519 key: %w", err)
	}
	x25519Shared, err := ephemeralKey.ECDH(recipientKey)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: X25519 exchange: %w", err)
	}
	x25519Ciphertext := ephemeralKey.PublicKey().Bytes()

	// Mirrors the combiner in DecapsulateHybrid: same inputs, same order. There
	// the fourth input is the receiver's own static public key, recomputed
	// from their private key; here it is that same static key, but as given —
	// the recipient's, not the sender's ephemeral one.
	var input []byte
	input = append(input, mlkemShared...)
	input = append(input, x25519Shared...)
	input = append(input, x25519Ciphertext...)
	input = append(input, x25519PublicKey...)
	input = append(input, combinerLabel...)

	shared := sha3.Sum256(input)

	ciphertext = append(mlkemCiphertext, x25519Ciphertext...)
	return ciphertext, shared[:], nil
}

// EncryptKeysHybrid wraps an email's symmetric key for one recipient: the
// mirror of DecryptKeysHybrid.
func EncryptKeysHybrid(sessionKey, recipientPublicKey []byte) (HybridEncryptedKey, error) {
	ciphertext, sharedSecret, err := EncapsulateHybrid(recipientPublicKey)
	if err != nil {
		return HybridEncryptedKey{}, fmt.Errorf("crypto: hybrid encryption: %w", err)
	}

	encryptedKey, err := WrapKey(sessionKey, sharedSecret)
	if err != nil {
		return HybridEncryptedKey{}, fmt.Errorf("crypto: hybrid encryption: %w", err)
	}

	return HybridEncryptedKey{HybridCiphertext: ciphertext, EncryptedKey: encryptedKey}, nil
}
