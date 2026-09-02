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

const emailListResponse = `{
	"emails": [{"id": "M1", "threadId": "T1", "subject": "hola", "isRead": true}],
	"total": 1,
	"hasMoreMails": false
}`

func TestGetUserFolderDecodesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/email" {
			t.Errorf("path = %q, want /email", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q, want Bearer tok", got)
		}
		if got := r.Header.Get("internxt-client"); got != "mail-web" {
			t.Errorf("internxt-client = %q, want mail-web", got)
		}
		if got := r.URL.Query().Get("mailbox"); got != "inbox" {
			t.Errorf("mailbox = %q, want inbox", got)
		}
		// Regression: an int formatted with string() would arrive as a control
		// character rather than its digits.
		if got := r.URL.Query().Get("limit"); got != "5" {
			t.Errorf("limit = %q, want 5", got)
		}
		w.Write([]byte(emailListResponse))
	}))
	defer srv.Close()

	res, err := newTestClient(t, srv).GetUserFolder(context.Background(), "tok", ListEmailsOptions{
		Mailbox: MailboxInbox,
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("GetUserFolder: %v", err)
	}
	if len(res.Emails) != 1 || res.Emails[0].Subject != "hola" || !res.Emails[0].IsRead {
		t.Fatalf("got %+v", res.Emails)
	}
}

func TestGetMailboxesDecodesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/email/mailboxes" {
			t.Errorf("path = %q, want /email/mailboxes", got)
		}
		w.Write([]byte(`[{"id":"mb1","name":"Inbox","type":"inbox","totalEmails":3,"unreadEmails":1}]`))
	}))
	defer srv.Close()

	mailboxes, err := newTestClient(t, srv).GetMailboxes(context.Background(), "tok")
	if err != nil {
		t.Fatalf("GetMailboxes: %v", err)
	}
	if len(mailboxes) != 1 || mailboxes[0].Name != "Inbox" {
		t.Fatalf("got %+v", mailboxes)
	}
	if mailboxes[0].Type == nil || *mailboxes[0].Type != MailboxResponseDtoTypeInbox {
		t.Fatalf("type = %v, want inbox", mailboxes[0].Type)
	}
}

func TestGetThreadDecodesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/email/threads/T1" {
			t.Errorf("path = %q, want /email/threads/T1", got)
		}
		w.Write([]byte(`[{"id":"M1","threadId":"T1","subject":"hola","textBody":"cuerpo"}]`))
	}))
	defer srv.Close()

	thread, err := newTestClient(t, srv).GetThread(context.Background(), "tok", "T1")
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if len(thread) != 1 || thread[0].Id != "M1" {
		t.Fatalf("got %+v", thread)
	}
}

func TestUnauthorizedIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "token expired", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).GetUserFolder(context.Background(), "tok", ListEmailsOptions{})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("errors.Is(err, ErrUnauthorized) = false, err = %v", err)
	}

	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected *Error with 401, got %v", err)
	}
}

// GET is idempotent, so a transient failure is retried.
func TestGetRetriesOnServerError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, "nope", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(emailListResponse))
	}))
	defer srv.Close()

	if _, err := newTestClient(t, srv).GetUserFolder(context.Background(), "tok", ListEmailsOptions{}); err != nil {
		t.Fatalf("GetUserFolder: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("server calls = %d, want 3", got)
	}
}

func TestContextCancellationAborts(t *testing.T) {
	// The handler waits for the client to go away rather than sleeping a fixed
	// time, so closing the server at the end of the test is immediate.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := newTestClient(t, srv).GetUserFolder(ctx, "tok", ListEmailsOptions{}); err == nil {
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
