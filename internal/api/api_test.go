package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mail-bridge-desktop/internal/config"
	"mail-bridge-desktop/internal/logger"
)

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := New(config.Config{MailAPI: srv.URL}, logger.New("test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// The keystore fields come back at the top level of the response, the way the
// previous prototype consumed them.
const userKeysResponse = `{
	"address": "user@inxt.com",
	"publicKey": "cHVi",
	"encryptionPrivateKey": "ZW5j",
	"recoveryPrivateKey": "cmVj"
}`

func TestGetUserKeysDecodesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/users/me/mail-account/keys" {
			t.Errorf("path = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q, want Bearer tok", got)
		}
		if got := r.Header.Get("internxt-client"); got != "mail-web" {
			t.Errorf("internxt-client = %q, want mail-web", got)
		}
		w.Write([]byte(userKeysResponse))
	}))
	defer srv.Close()

	keys, err := newTestClient(t, srv).GetUserKeys(context.Background(), "tok")
	if err != nil {
		t.Fatalf("GetUserKeys: %v", err)
	}

	want := UserKeys{
		Address:              "user@inxt.com",
		PublicKey:            "cHVi",
		EncryptionPrivateKey: "ZW5j",
		RecoveryPrivateKey:   "cmVj",
	}
	if keys != want {
		t.Fatalf("got %+v, want %+v", keys, want)
	}
}

func TestGetUserKeysUnauthorizedIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "token expired", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).GetUserKeys(context.Background(), "tok")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("errors.Is(err, ErrUnauthorized) = false, err = %v", err)
	}

	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected *Error with 401, got %v", err)
	}
}

// GET is idempotent, so a transient failure is retried.
func TestGetUserKeysRetriesOnServerError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, "nope", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(userKeysResponse))
	}))
	defer srv.Close()

	keys, err := newTestClient(t, srv).GetUserKeys(context.Background(), "tok")
	if err != nil {
		t.Fatalf("GetUserKeys: %v", err)
	}
	if keys.Address != "user@inxt.com" {
		t.Fatalf("got %+v", keys)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("server calls = %d, want 3", got)
	}
}

func TestGetUserKeysContextCancellationAborts(t *testing.T) {
	// The handler waits for the client to go away rather than sleeping a fixed
	// time, so closing the server at the end of the test is immediate.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := newTestClient(t, srv).GetUserKeys(ctx, "tok"); err == nil {
		t.Fatal("expected an error")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("took %v, expected the context to cut it short", elapsed)
	}
}

func TestNewRequiresMailURL(t *testing.T) {
	if _, err := New(config.Config{}, logger.New("test")); err == nil {
		t.Fatal("expected an error when MAIL_API_URL is missing")
	}
}
