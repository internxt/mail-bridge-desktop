package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookupRecipientKeysDecodesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/email/keys/lookup" {
			t.Errorf("path = %q, want /email/keys/lookup", got)
		}
		if got := r.Method; got != http.MethodPost {
			t.Errorf("method = %q, want POST", got)
		}

		var body LookupRecipientKeysRequestDto
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := body.Addresses; len(got) != 2 || got[0] != "alice@inxt.eu" {
			t.Errorf("addresses = %v, want [alice@inxt.eu bob@example.com]", got)
		}

		w.Write([]byte(`{"recipients":[{"address":"alice@inxt.eu","publicKey":"abc123"},{"address":"bob@example.com","publicKey":null}]}`))
	}))
	defer srv.Close()

	recipients, err := newTestClient(t, srv).LookupRecipientKeys(context.Background(), "tok", []string{"alice@inxt.eu", "bob@example.com"})
	if err != nil {
		t.Fatalf("LookupRecipientKeys: %v", err)
	}
	if len(recipients) != 2 {
		t.Fatalf("got %d recipients, want 2", len(recipients))
	}
	if recipients[0].PublicKey == nil || *recipients[0].PublicKey != "abc123" {
		t.Errorf("alice's public key = %v, want abc123", recipients[0].PublicKey)
	}
	if recipients[1].PublicKey != nil {
		t.Errorf("bob's public key = %v, want nil (no Internxt key)", *recipients[1].PublicKey)
	}
}

func TestSendEmailPostsAndDecodesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/email/send" {
			t.Errorf("path = %q, want /email/send", got)
		}

		var body SendEmailRequestDto
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body.Subject != "hola" {
			t.Errorf("subject = %q, want hola", body.Subject)
		}
		if len(body.To) != 1 || body.To[0].Email != "bob@inxt.eu" {
			t.Errorf("to = %+v, want [bob@inxt.eu]", body.To)
		}

		w.Write([]byte(`{"id":"M1"}`))
	}))
	defer srv.Close()

	res, err := newTestClient(t, srv).SendEmail(context.Background(), "tok", SendEmailRequestDto{
		Subject: "hola",
		To:      []EmailAddressDto{{Email: "bob@inxt.eu"}},
	})
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if res.Id != "M1" {
		t.Errorf("id = %q, want M1", res.Id)
	}
}

func TestGetMailAccountKeysDecodesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/users/me/mail-account/keys" {
			t.Errorf("path = %q, want /users/me/mail-account/keys", got)
		}
		w.Write([]byte(`{"address":"alice@inxt.eu","encryptionPrivateKey":"enc","publicKey":"pub","recoveryPrivateKey":"rec"}`))
	}))
	defer srv.Close()

	keys, err := newTestClient(t, srv).GetMailAccountKeys(context.Background(), "tok")
	if err != nil {
		t.Fatalf("GetMailAccountKeys: %v", err)
	}
	if keys.PublicKey != "pub" {
		t.Errorf("public key = %q, want pub", keys.PublicKey)
	}
}
