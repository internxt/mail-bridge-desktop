package crypto

import (
	"crypto/ecdh"
	"crypto/mlkem"
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
