package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const EncryptedEmailPrefix = "INTERNXT-ENCRYPTED-EMAIL-v1"

// envelopeVersion is the only wire version this package writes. Reading is
// not restricted to it: ParseEnvelope accepts whatever version is on the
// wire, since the web client is the one that decides when to bump it.
const envelopeVersion = "v3"

// sessionKeyLen is the symmetric key length in bytes, i.e. AES-256.
const sessionKeyLen = 32

// Recipient is one address to seal an envelope for, with their hybrid public
// key as published by the Mail API.
type Recipient struct {
	Address   string
	PublicKey []byte
}

var ErrNoWrappedKey = errors.New("crypto: no wrapped key for this address")

type WrappedKey struct {
	EncryptedForEmail string `json:"encryptedForEmail"`
	EncryptedKey      string `json:"encryptedKey"`
	HybridCiphertext  string `json:"hybridCiphertext"`
}

type Envelope struct {
	Version                        string       `json:"version"`
	EncryptedText                  string       `json:"encryptedText"`
	EncryptedPreview               string       `json:"encryptedPreview"`
	EncryptedAttachmentsSessionKey string       `json:"encryptedAttachmentsSessionKey"`
	WrappedKeys                    []WrappedKey `json:"wrappedKeys"`
}

func IsEncryptedBody(textBody string) bool {
	return strings.HasPrefix(textBody, EncryptedEmailPrefix+"\n")
}

func ParseEnvelope(textBody string) (Envelope, error) {
	if !IsEncryptedBody(textBody) {
		return Envelope{}, errors.New("crypto: body does not carry an encrypted envelope")
	}

	payload := textBody[len(EncryptedEmailPrefix)+1:]
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
	if err != nil {
		return Envelope{}, fmt.Errorf("crypto: decode envelope: %w", err)
	}

	var envelope Envelope
	if err := json.Unmarshal(decoded, &envelope); err != nil {
		return Envelope{}, fmt.Errorf("crypto: parse envelope: %w", err)
	}

	return envelope, nil
}

func findKeyFor(wrappedKeys []WrappedKey, address string) (WrappedKey, error) {
	for _, wrapped := range wrappedKeys {
		if strings.EqualFold(wrapped.EncryptedForEmail, address) {
			return wrapped, nil
		}
	}
	return WrappedKey{}, fmt.Errorf("%w: %s", ErrNoWrappedKey, address)
}

// decodeWrappedKey turns a wrapped key's base64 fields into bytes. Necessary since
// we get the key from the client via IPC in base64.
func decodeWrappedKey(wrapped WrappedKey) (HybridEncryptedKey, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(wrapped.HybridCiphertext)
	if err != nil {
		return HybridEncryptedKey{}, fmt.Errorf("crypto: decode hybrid ciphertext: %w", err)
	}

	encryptedKey, err := base64.StdEncoding.DecodeString(wrapped.EncryptedKey)
	if err != nil {
		return HybridEncryptedKey{}, fmt.Errorf("crypto: decode wrapped key: %w", err)
	}

	return HybridEncryptedKey{HybridCiphertext: ciphertext, EncryptedKey: encryptedKey}, nil
}

func sessionKeyFor(wrappedKeys []WrappedKey, privateKey []byte, address string) ([]byte, error) {
	wrapped, err := findKeyFor(wrappedKeys, address)
	if err != nil {
		return nil, err
	}

	encryptedKey, err := decodeWrappedKey(wrapped)
	if err != nil {
		return nil, err
	}

	return DecryptKeysHybrid(encryptedKey, privateKey)
}

// DecryptEnvelope opens an envelope with the account's private key, returning
// the body, the preview and the attachments session key.
//
// privateKey is the 32-byte root seed, i.e. the decrypted encryptionPrivateKey.
// address selects which wrapped key to use, so it must be the account address
// the email was encrypted for.
func DecryptEnvelope(envelope Envelope, privateKey []byte, address string) (Email, error) {
	sessionKey, err := sessionKeyFor(envelope.WrappedKeys, privateKey, address)
	if err != nil {
		return Email{}, err
	}

	encrypted, err := decodeEnvelopeCiphertexts(envelope)
	if err != nil {
		return Email{}, err
	}

	return DecryptEmail(encrypted, sessionKey, nil)
}

// BuildEnvelope encrypts an email once under a fresh random session key and
// wraps that key for every recipient. The sender's own address must be among
// recipients so they can read their own Sent copy — this package does not add
// it implicitly, since only the caller knows which recipient is the sender.
func BuildEnvelope(email Email, recipients []Recipient) (Envelope, error) {
	if len(recipients) == 0 {
		return Envelope{}, errors.New("crypto: an envelope needs at least one recipient")
	}

	sessionKey := make([]byte, sessionKeyLen)
	if _, err := rand.Read(sessionKey); err != nil {
		return Envelope{}, fmt.Errorf("crypto: generate session key: %w", err)
	}

	encrypted, err := EncryptEmail(email, sessionKey, nil)
	if err != nil {
		return Envelope{}, err
	}

	wrappedKeys := make([]WrappedKey, 0, len(recipients))
	for _, recipient := range recipients {
		encryptedKey, err := EncryptKeysHybrid(sessionKey, recipient.PublicKey)
		if err != nil {
			return Envelope{}, fmt.Errorf("crypto: seal session key for %s: %w", recipient.Address, err)
		}

		wrappedKeys = append(wrappedKeys, WrappedKey{
			EncryptedForEmail: recipient.Address,
			EncryptedKey:      base64.StdEncoding.EncodeToString(encryptedKey.EncryptedKey),
			HybridCiphertext:  base64.StdEncoding.EncodeToString(encryptedKey.HybridCiphertext),
		})
	}

	return Envelope{
		Version:                        envelopeVersion,
		EncryptedText:                  base64.StdEncoding.EncodeToString(encrypted.Text),
		EncryptedPreview:               base64.StdEncoding.EncodeToString(encrypted.Preview),
		EncryptedAttachmentsSessionKey: base64.StdEncoding.EncodeToString(encrypted.AttachmentsSessionKey),
		WrappedKeys:                    wrappedKeys,
	}, nil
}

// DecryptPreview opens the preview a listing carries.
func DecryptPreview(wrappedKeys []WrappedKey, encryptedPreview string, privateKey []byte, address string) (string, error) {
	if encryptedPreview == "" {
		return "", nil
	}

	sessionKey, err := sessionKeyFor(wrappedKeys, privateKey, address)
	if err != nil {
		return "", err
	}

	ciphertext, err := decodeOptional(encryptedPreview)
	if err != nil {
		return "", fmt.Errorf("crypto: decode preview: %w", err)
	}

	preview, err := DecryptSymmetrically(sessionKey, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decrypt preview: %w", err)
	}

	return string(preview), nil
}

func decodeEnvelopeCiphertexts(envelope Envelope) (EncryptedEmail, error) {
	var (
		encrypted EncryptedEmail
		err       error
	)

	if encrypted.Text, err = decodeOptional(envelope.EncryptedText); err != nil {
		return EncryptedEmail{}, fmt.Errorf("crypto: decode body: %w", err)
	}
	if encrypted.Preview, err = decodeOptional(envelope.EncryptedPreview); err != nil {
		return EncryptedEmail{}, fmt.Errorf("crypto: decode preview: %w", err)
	}
	if encrypted.AttachmentsSessionKey, err = decodeOptional(envelope.EncryptedAttachmentsSessionKey); err != nil {
		return EncryptedEmail{}, fmt.Errorf("crypto: decode attachments session key: %w", err)
	}

	return encrypted, nil
}

func decodeOptional(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(value)
}
