package crypto

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// testdata/encrypted_body.txt is a real encrypted text body built with the JS
// libraries mail-web uses, sealed for two recipients so the per-address key
// lookup is exercised. The constants below are the plaintext that went in.
const (
	aliceSeed    = "0101010101010101010101010101010101010101010101010101010101010101"
	aliceAddress = "alice@inxt.me"

	bobSeed    = "0202020202020202020202020202020202020202020202020202020202020202"
	bobAddress = "BOB@inxt.me"

	envelopeBody                  = "Hola, esto es un correo cifrado con acentos: ñ á é €"
	envelopePreview               = "Hola, esto es un correo cifrado"
	envelopeAttachmentsSessionKey = "eddcf7889aa0381d13853bfe6a870fd53f0e5794d7a5429fca6d5c1b6e466e23"
)

func encryptedBody(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("testdata/encrypted_body.txt")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	return string(body)
}

func parsedEnvelope(t *testing.T) Envelope {
	t.Helper()
	envelope, err := ParseEnvelope(encryptedBody(t))
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	return envelope
}

// TestDecryptEnvelopeMatchesJS is the end-to-end interop check: an envelope
// produced by the web client must decrypt here to the exact same plaintext.
func TestDecryptEnvelopeMatchesJS(t *testing.T) {
	envelope := parsedEnvelope(t)

	email, err := DecryptEnvelope(envelope, mustHex(t, aliceSeed), aliceAddress)
	if err != nil {
		t.Fatalf("DecryptEnvelope: %v", err)
	}

	if email.Text != envelopeBody {
		t.Errorf("body\n got %q\nwant %q", email.Text, envelopeBody)
	}
	if email.Preview != envelopePreview {
		t.Errorf("preview\n got %q\nwant %q", email.Preview, envelopePreview)
	}
	if want := mustHex(t, envelopeAttachmentsSessionKey); !bytes.Equal(email.AttachmentsSessionKey, want) {
		t.Errorf("attachments session key\n got %X\nwant %X", email.AttachmentsSessionKey, want)
	}
}

// TestDecryptEnvelopeForEachRecipient checks that one envelope opens for every
// recipient it was sealed for, each with their own key.
func TestDecryptEnvelopeForEachRecipient(t *testing.T) {
	envelope := parsedEnvelope(t)

	for _, tc := range []struct {
		name    string
		seed    string
		address string
	}{
		{"first recipient", aliceSeed, aliceAddress},
		{"second recipient", bobSeed, bobAddress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			email, err := DecryptEnvelope(envelope, mustHex(t, tc.seed), tc.address)
			if err != nil {
				t.Fatalf("DecryptEnvelope: %v", err)
			}
			if email.Text != envelopeBody {
				t.Errorf("got %q, want %q", email.Text, envelopeBody)
			}
		})
	}
}

// TestDecryptEnvelopeAddressIsCaseInsensitive matters because the label in the
// envelope keeps whatever case the sender used, while the address the bridge
// holds comes from elsewhere. The JS client lowercases both sides.
func TestDecryptEnvelopeAddressIsCaseInsensitive(t *testing.T) {
	envelope := parsedEnvelope(t)

	// bob's label is stored uppercase in the envelope; ask with lowercase.
	if _, err := DecryptEnvelope(envelope, mustHex(t, bobSeed), strings.ToLower(bobAddress)); err != nil {
		t.Fatalf("DecryptEnvelope with lowercased address: %v", err)
	}
	// And alice's is stored lowercase; ask with uppercase.
	if _, err := DecryptEnvelope(envelope, mustHex(t, aliceSeed), strings.ToUpper(aliceAddress)); err != nil {
		t.Fatalf("DecryptEnvelope with uppercased address: %v", err)
	}
}

func TestDecryptEnvelopeRejectsUnknownAddress(t *testing.T) {
	envelope := parsedEnvelope(t)

	_, err := DecryptEnvelope(envelope, mustHex(t, aliceSeed), "carol@inxt.me")
	if !errors.Is(err, ErrNoWrappedKey) {
		t.Fatalf("got %v, want ErrNoWrappedKey", err)
	}
}

// TestDecryptEnvelopeRejectsWrongKey covers the recipient using someone else's
// key: the label is found, but AES-KW rejects the unwrap.
func TestDecryptEnvelopeRejectsWrongKey(t *testing.T) {
	envelope := parsedEnvelope(t)

	if _, err := DecryptEnvelope(envelope, mustHex(t, bobSeed), aliceAddress); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestIsEncryptedBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"encrypted body", encryptedBody(t), true},
		{"plain text", "Hola, esto es texto plano", false},
		{"empty", "", false},
		{"prefix without newline", EncryptedEmailPrefix, false},
		{"prefix inside the text", "algo " + EncryptedEmailPrefix + "\nx", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsEncryptedBody(tc.body); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseEnvelopeFields(t *testing.T) {
	envelope := parsedEnvelope(t)

	if envelope.Version != "v3" {
		t.Errorf("version: got %q, want %q", envelope.Version, "v3")
	}
	if len(envelope.WrappedKeys) != 2 {
		t.Fatalf("wrapped keys: got %d, want 2", len(envelope.WrappedKeys))
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"encryptedText", envelope.EncryptedText},
		{"encryptedPreview", envelope.EncryptedPreview},
		{"encryptedAttachmentsSessionKey", envelope.EncryptedAttachmentsSessionKey},
	} {
		if field.value == "" {
			t.Errorf("%s is empty", field.name)
		}
	}
}

func TestParseEnvelopeRejectsMalformedInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"plain text", "no envelope here"},
		{"payload is not base64", EncryptedEmailPrefix + "\nnot valid base64!!"},
		{"payload is not JSON", EncryptedEmailPrefix + "\n" + base64.StdEncoding.EncodeToString([]byte("nope"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseEnvelope(tc.body); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

// TestEnvelopeJSONRoundTrip guards the JSON tags, which have to keep matching
// what the web client writes: an envelope is parsed straight off the wire, not
// only built from an API DTO.
func TestEnvelopeJSONRoundTrip(t *testing.T) {
	original := parsedEnvelope(t)

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Envelope
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.EncryptedText != original.EncryptedText {
		t.Error("encryptedText did not survive the round trip")
	}
	if len(decoded.WrappedKeys) != len(original.WrappedKeys) {
		t.Fatalf("wrapped keys: got %d, want %d", len(decoded.WrappedKeys), len(original.WrappedKeys))
	}
	if decoded.WrappedKeys[0].EncryptedForEmail != original.WrappedKeys[0].EncryptedForEmail {
		t.Error("encryptedForEmail did not survive the round trip")
	}
}
