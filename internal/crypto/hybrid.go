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

	mlkemPublicKeyLen  = 1184
	x25519PublicKeyLen = 32
	hybridPublicKeyLen = mlkemPublicKeyLen + x25519PublicKeyLen
)

const combinerLabel = `\.//^\`

// xwingKEM is the X-Wing hybrid KEM: ML-KEM-768 combined with X25519 so that
// breaking the shared secret requires breaking both. X25519 covers the risk
// that ML-KEM turns out to be flawed; ML-KEM covers the risk of a quantum
// computer breaking X25519. Encapsulate agrees on a secret with a recipient
// knowing only their public key; Decapsulate recovers it with the private key
// that matches.
type xwingKEM struct{}

// Encapsulate agrees on a secret with a recipient, knowing only their public
// key: out comes a shared secret, plus a ciphertext only their private key
// can open.
//
// publicKey is the recipient's 1216-byte hybrid public key (1184 bytes
// ML-KEM-768 followed by 32 bytes X25519), as published by the Mail API.
func (xwingKEM) Encapsulate(publicKey []byte) (ciphertext, sharedSecret []byte, err error) {
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

	ephemeralKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: generate ephemeral X25519 key: %w", err)
	}
	x25519Shared, err := ephemeralKey.ECDH(recipientKey)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: X25519 exchange: %w", err)
	}
	x25519Ciphertext := ephemeralKey.PublicKey().Bytes()

	shared := combineHybridSecrets(mlkemShared, x25519Shared, x25519Ciphertext, x25519PublicKey)

	ciphertext = append(mlkemCiphertext, x25519Ciphertext...)
	return ciphertext, shared, nil
}

// Decapsulate recovers the shared secret Encapsulate agreed on, using the
// private key matching the public key it was encapsulated against.
func (xwingKEM) Decapsulate(ciphertext, privateKey []byte) ([]byte, error) {
	if len(privateKey) != seedLen {
		return nil, fmt.Errorf("crypto: hybrid seed is %d bytes, want %d", len(privateKey), seedLen)
	}
	if len(ciphertext) != hybridCiphertextLen {
		return nil, fmt.Errorf("crypto: hybrid ciphertext is %d bytes, want %d", len(ciphertext), hybridCiphertextLen)
	}

	mlkemSeed, x25519Secret := expandHybridSeed(privateKey)
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
	ownKey, err := ecdh.X25519().NewPrivateKey(x25519Secret)
	if err != nil {
		return nil, fmt.Errorf("crypto: load X25519 key: %w", err)
	}
	ephemeralKey, err := ecdh.X25519().NewPublicKey(x25519Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("crypto: load X25519 ciphertext: %w", err)
	}
	x25519Shared, err := ownKey.ECDH(ephemeralKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: X25519 exchange: %w", err)
	}

	// The combiner binds both secrets to the X25519 transcript. The public key
	// is recomputed from the private one rather than carried alongside,
	// matching what noble does during decapsulation.
	shared := combineHybridSecrets(mlkemShared, x25519Shared, x25519Ciphertext, ownKey.PublicKey().Bytes())
	return shared, nil
}

// WrapSessionKey encrypts a session key for one recipient: encapsulate a
// shared secret against their public key, then wrap the session key under it
// with AES Key Wrap. The shared secret itself never leaves this method.
func (k xwingKEM) WrapSessionKey(sessionKey, recipientPublicKey []byte) (HybridEncryptedKey, error) {
	ciphertext, sharedSecret, err := k.Encapsulate(recipientPublicKey)
	if err != nil {
		return HybridEncryptedKey{}, fmt.Errorf("crypto: hybrid encryption: %w", err)
	}

	wrapper, err := newAESKeyWrap(sharedSecret)
	if err != nil {
		return HybridEncryptedKey{}, fmt.Errorf("crypto: hybrid encryption: %w", err)
	}
	encryptedKey, err := wrapper.Wrap(sessionKey)
	if err != nil {
		return HybridEncryptedKey{}, fmt.Errorf("crypto: hybrid encryption: %w", err)
	}

	return HybridEncryptedKey{HybridCiphertext: ciphertext, EncryptedKey: encryptedKey}, nil
}

// UnwrapSessionKey recovers a session key wrapped by WrapSessionKey, using
// the recipient's private key.
func (k xwingKEM) UnwrapSessionKey(encryptedKey HybridEncryptedKey, recipientPrivateKey []byte) ([]byte, error) {
	sharedSecret, err := k.Decapsulate(encryptedKey.HybridCiphertext, recipientPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: hybrid decryption: %w", err)
	}

	wrapper, err := newAESKeyWrap(sharedSecret)
	if err != nil {
		return nil, fmt.Errorf("crypto: hybrid decryption: %w", err)
	}
	key, err := wrapper.Unwrap(encryptedKey.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: hybrid decryption: %w", err)
	}

	return key, nil
}

// combineHybridSecrets is the X-Wing combiner:
//
//	SHA3-256( ss_mlkem ‖ ss_x25519 ‖ ct_x25519 ‖ pk_x25519 ‖ "\.//^\" )
//
// Feeding in the X25519 ciphertext and the recipient's public key, not just
// the two shared secrets, binds the result to this specific exchange. The
// trailing label is domain separation, keeping this hash from colliding with
// the same inputs hashed for another purpose.
func combineHybridSecrets(mlkemShared, x25519Shared, x25519Ciphertext, x25519PublicKey []byte) []byte {
	var input []byte
	input = append(input, mlkemShared...)
	input = append(input, x25519Shared...)
	input = append(input, x25519Ciphertext...)
	input = append(input, x25519PublicKey...)
	input = append(input, combinerLabel...)

	shared := sha3.Sum256(input)
	return shared[:]
}

// expandHybridSeed derives the two component keys from the root seed with
// SHAKE256, the way noble's expandSeedXof does: one XOF output, split by
// component in registration order (ML-KEM first, then X25519).
func expandHybridSeed(seed []byte) (mlkemSeed, x25519Secret []byte) {
	expanded := make([]byte, mlkemSeedLen+x25519KeyLen)
	sha3.ShakeSum256(expanded, seed)
	return expanded[:mlkemSeedLen], expanded[mlkemSeedLen:]
}

type HybridEncryptedKey struct {
	HybridCiphertext []byte
	EncryptedKey     []byte
}

func DecapsulateHybrid(ciphertext, seed []byte) ([]byte, error) {
	return xwingKEM{}.Decapsulate(ciphertext, seed)
}

func EncapsulateHybrid(publicKey []byte) (ciphertext, sharedSecret []byte, err error) {
	return xwingKEM{}.Encapsulate(publicKey)
}

func DecryptKeysHybrid(encryptedKey HybridEncryptedKey, recipientPrivateKey []byte) ([]byte, error) {
	return xwingKEM{}.UnwrapSessionKey(encryptedKey, recipientPrivateKey)
}

func EncryptKeysHybrid(sessionKey, recipientPublicKey []byte) (HybridEncryptedKey, error) {
	return xwingKEM{}.WrapSessionKey(sessionKey, recipientPublicKey)
}
