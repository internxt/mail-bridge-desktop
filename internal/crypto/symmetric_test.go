package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex in test: %v", err)
	}
	return b
}

// TestUnwrapKeyRFC3394 checks UnwrapKey against the published test vectors in
// RFC 3394, section 4. These are the authoritative vectors for AES-KW, and
// since the algorithm is hand-written here, they are what makes it
// trustworthy.
func TestUnwrapKeyRFC3394(t *testing.T) {
	cases := []struct {
		name    string
		kek     string
		wrapped string
		want    string
	}{
		{
			name:    "128-bit key with 128-bit KEK",
			kek:     "000102030405060708090A0B0C0D0E0F",
			wrapped: "1FA68B0A8112B447AEF34BD8FB5A7B829D3E862371D2CFE5",
			want:    "00112233445566778899AABBCCDDEEFF",
		},
		{
			name:    "128-bit key with 256-bit KEK",
			kek:     "000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F",
			wrapped: "64E8C3F9CE0F5BA263E9777905818A2A93C8191E7D6E8AE7",
			want:    "00112233445566778899AABBCCDDEEFF",
		},
		{
			name:    "256-bit key with 256-bit KEK",
			kek:     "000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F",
			wrapped: "28C9F404C4B810F4CBCCB35CFB87F8263F5786E2D80ED326CBC7F0E71A99F43BFB988B9B7A02DD21",
			want:    "00112233445566778899AABBCCDDEEFF000102030405060708090A0B0C0D0E0F",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := UnwrapKey(mustHex(t, tc.wrapped), mustHex(t, tc.kek))
			if err != nil {
				t.Fatalf("UnwrapKey: %v", err)
			}
			if want := mustHex(t, tc.want); !bytes.Equal(got, want) {
				t.Errorf("got %X, want %X", got, want)
			}
		})
	}
}

// The 256-bit-key-with-256-bit-KEK vector is the shape this project actually
// uses: a 32-byte email key wrapped under the 32-byte hybrid shared secret.

func TestUnwrapKeyRejectsWrongKEK(t *testing.T) {
	wrapped := mustHex(t, "28C9F404C4B810F4CBCCB35CFB87F8263F5786E2D80ED326CBC7F0E71A99F43BFB988B9B7A02DD21")
	wrongKEK := make([]byte, 32) // all zeros, not the KEK from the vector

	_, err := UnwrapKey(wrapped, wrongKEK)
	if !errors.Is(err, ErrKeyUnwrap) {
		t.Fatalf("got %v, want ErrKeyUnwrap", err)
	}
}

func TestUnwrapKeyRejectsMalformedInput(t *testing.T) {
	kek := make([]byte, 32)

	for _, tc := range []struct {
		name string
		size int
	}{
		{"too short", 16},
		{"not a multiple of 8", 28},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := UnwrapKey(make([]byte, tc.size), kek); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

// encryptLikeJS mirrors the JS library's layout, appending the IV after the
// ciphertext, so the tests exercise the same byte order the API returns.
func encryptLikeJS(t *testing.T, key, plaintext, iv, aux []byte) []byte {
	t.Helper()

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("NewGCM: %v", err)
	}

	return append(aead.Seal(nil, iv, plaintext, aux), iv...)
}

func TestDecryptSymmetricallyRoundTrip(t *testing.T) {
	key := mustHex(t, "000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F")
	iv := mustHex(t, "000102030405060708090A0B")
	plaintext := []byte("Hola, esto es un correo con acentos: ñ á é €")

	for _, tc := range []struct {
		name string
		aux  []byte
	}{
		{"without aux", nil},
		{"with aux", []byte("associated data")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encrypted := encryptLikeJS(t, key, plaintext, iv, tc.aux)

			got, err := DecryptSymmetrically(key, encrypted, tc.aux)
			if err != nil {
				t.Fatalf("DecryptSymmetrically: %v", err)
			}
			if !bytes.Equal(got, plaintext) {
				t.Errorf("got %q, want %q", got, plaintext)
			}
		})
	}
}

// TestDecryptSymmetricallyIVIsAtTheEnd is the regression guard for the layout
// quirk: if someone "fixes" the code to read the IV from the front, this fails.
func TestDecryptSymmetricallyIVIsAtTheEnd(t *testing.T) {
	key := mustHex(t, "000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F")
	iv := mustHex(t, "0F0E0D0C0B0A09080706050403020100")[:12]
	encrypted := encryptLikeJS(t, key, []byte("payload"), iv, nil)

	if tail := encrypted[len(encrypted)-ivLen:]; !bytes.Equal(tail, iv) {
		t.Fatalf("test setup is wrong: tail %X is not the IV %X", tail, iv)
	}
	if _, err := DecryptSymmetrically(key, encrypted, nil); err != nil {
		t.Fatalf("DecryptSymmetrically: %v", err)
	}
}

func TestDecryptSymmetricallyRejectsTampering(t *testing.T) {
	key := mustHex(t, "000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F")
	iv := mustHex(t, "000102030405060708090A0B")
	encrypted := encryptLikeJS(t, key, []byte("payload"), iv, []byte("aux"))

	t.Run("flipped ciphertext bit", func(t *testing.T) {
		tampered := bytes.Clone(encrypted)
		tampered[0] ^= 0x01
		if _, err := DecryptSymmetrically(key, tampered, []byte("aux")); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("mismatched aux", func(t *testing.T) {
		if _, err := DecryptSymmetrically(key, encrypted, []byte("different")); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("payload shorter than the IV", func(t *testing.T) {
		if _, err := DecryptSymmetrically(key, make([]byte, ivLen-1), nil); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}
