package store

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

func TestEncryptionPrivateKeyRoundTrip(t *testing.T) {
	s, _, _ := newTestStore(t)

	want := bytes.Repeat([]byte{0xAB}, privateKeyLen)
	if err := s.Set(KeyEncryptionPrivateKey, []byte(base64.StdEncoding.EncodeToString(want))); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := EncryptionPrivateKey(s)
	if err != nil {
		t.Fatalf("EncryptionPrivateKey: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %X, want %X", got, want)
	}
}

// TestEncryptionPrivateKeyRejectsWrongLength guards the check that turns a bad
// key into a clear error here, instead of an unexplained decryption failure
// later on.
func TestEncryptionPrivateKeyRejectsWrongLength(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
	}{
		{"too short", privateKeyLen - 1},
		{"too long", privateKeyLen + 1},
		{"empty", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := newTestStore(t)
			encoded := base64.StdEncoding.EncodeToString(make([]byte, tc.size))
			if err := s.Set(KeyEncryptionPrivateKey, []byte(encoded)); err != nil {
				t.Fatalf("Set: %v", err)
			}

			if _, err := EncryptionPrivateKey(s); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestEncryptionPrivateKeyRejectsBadBase64(t *testing.T) {
	s, _, _ := newTestStore(t)
	if err := s.Set(KeyEncryptionPrivateKey, []byte("not valid base64!!")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if _, err := EncryptionPrivateKey(s); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestEncryptionPrivateKeyWhenMissing(t *testing.T) {
	s, _, _ := newTestStore(t)

	if _, err := EncryptionPrivateKey(s); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestPublicKeyRoundTrip(t *testing.T) {
	s, _, _ := newTestStore(t)

	// A hybrid public key is over a kilobyte, so this also exercises a value
	// well past a token's size.
	want := bytes.Repeat([]byte{0xCD}, 1216)
	if err := s.Set(KeyPublicKey, []byte(base64.StdEncoding.EncodeToString(want))); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := PublicKey(s)
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %d bytes, want %d", len(got), len(want))
	}
}

func TestPublicKeyRejectsBadBase64(t *testing.T) {
	s, _, _ := newTestStore(t)
	if err := s.Set(KeyPublicKey, []byte("not valid base64!!")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if _, err := PublicKey(s); err == nil {
		t.Fatal("expected an error, got nil")
	}
}
