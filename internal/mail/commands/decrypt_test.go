package commands

import (
	"context"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"mail-bridge-desktop/internal/api"
)

// testdata/encrypted_body.txt is a real encrypted body built with the JS
// libraries the web client uses, sealed for the two addresses below.
const (
	testAddress    = "alice@inxt.me"
	testPrivateKey = "0101010101010101010101010101010101010101010101010101010101010101"

	otherAddress    = "BOB@inxt.me"
	otherPrivateKey = "0202020202020202020202020202020202020202020202020202020202020202"

	testPlaintext = "Hola, esto es un correo cifrado con acentos: ñ á é €"
)

func testAccount(t *testing.T, address, privateKey string) Account {
	t.Helper()
	key, err := hex.DecodeString(privateKey)
	if err != nil {
		t.Fatalf("bad hex in test: %v", err)
	}
	return Account{Address: address, PrivateKey: key}
}

func encryptedEmail(t *testing.T) api.EmailResponseDto {
	t.Helper()
	body, err := os.ReadFile("testdata/encrypted_body.txt")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	text := string(body)
	return api.EmailResponseDto{Id: "M1", Subject: "cifrado", TextBody: &text}
}

// TestGetEmailDecryptsBody is the check that the whole chain is wired: an
// encrypted email fetched through the command comes back readable.
func TestGetEmailDecryptsBody(t *testing.T) {
	client := &fakeClient{thread: []api.EmailResponseDto{encryptedEmail(t)}}

	email, err := GetEmail(context.Background(), client, "tok", "M1", testAccount(t, testAddress, testPrivateKey))
	if err != nil {
		t.Fatalf("GetEmail: %v", err)
	}

	if got := deref(email.HtmlBody); got != testPlaintext {
		t.Errorf("body\n got %q\nwant %q", got, testPlaintext)
	}
	if email.TextBody != nil {
		t.Error("the encrypted text body should have been cleared")
	}
}

// TestGetEmailLeavesPlaintextAlone covers the common case: an unencrypted
// email must pass through untouched.
func TestGetEmailLeavesPlaintextAlone(t *testing.T) {
	text := "Hola, esto es texto plano"
	html := "<p>Hola</p>"
	client := &fakeClient{thread: []api.EmailResponseDto{
		{Id: "M1", TextBody: &text, HtmlBody: &html},
	}}

	email, err := GetEmail(context.Background(), client, "tok", "M1", testAccount(t, testAddress, testPrivateKey))
	if err != nil {
		t.Fatalf("GetEmail: %v", err)
	}

	if deref(email.TextBody) != text {
		t.Errorf("text body changed: %q", deref(email.TextBody))
	}
	if deref(email.HtmlBody) != html {
		t.Errorf("html body changed: %q", deref(email.HtmlBody))
	}
}

// TestGetEmailKeepsEncryptedBodyWhenItCannotDecrypt is the behaviour that
// keeps a mailbox usable: a message the account cannot open is still served,
// with the error explaining why, so the client shows it instead of nothing.
func TestGetEmailKeepsEncryptedBodyWhenItCannotDecrypt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account Account
	}{
		{"no keys at all", Account{}},
		{"address the envelope was not sealed for", testAccount(t, "carol@inxt.me", testPrivateKey)},
		{"another recipient's key", testAccount(t, testAddress, otherPrivateKey)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeClient{thread: []api.EmailResponseDto{encryptedEmail(t)}}

			email, err := GetEmail(context.Background(), client, "tok", "M1", tc.account)
			if err == nil {
				t.Fatal("expected an error explaining why it could not decrypt")
			}
			if email.Id != "M1" {
				t.Fatalf("the email should still be returned, got id %q", email.Id)
			}
			if !strings.HasPrefix(deref(email.TextBody), "INTERNXT-ENCRYPTED-EMAIL-v1") {
				t.Error("the envelope should have been left untouched")
			}
		})
	}
}

// TestGetEmailDecryptsForEachRecipient checks the per-address key lookup: the
// same envelope opens for whichever recipient asks, with their own key.
func TestGetEmailDecryptsForEachRecipient(t *testing.T) {
	for _, tc := range []struct {
		name    string
		address string
		key     string
	}{
		{"first recipient", testAddress, testPrivateKey},
		{"second recipient", otherAddress, otherPrivateKey},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeClient{thread: []api.EmailResponseDto{encryptedEmail(t)}}

			email, err := GetEmail(context.Background(), client, "tok", "M1", testAccount(t, tc.address, tc.key))
			if err != nil {
				t.Fatalf("GetEmail: %v", err)
			}
			if got := deref(email.HtmlBody); got != testPlaintext {
				t.Errorf("got %q, want %q", got, testPlaintext)
			}
		})
	}
}
