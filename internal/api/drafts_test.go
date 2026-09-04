package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSaveDraftPostsAndDecodesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/email/drafts" {
			t.Errorf("path = %q, want /email/drafts", got)
		}
		if got := r.Method; got != http.MethodPost {
			t.Errorf("method = %q, want POST", got)
		}

		var body DraftEmailRequestDto
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body.Subject == nil || *body.Subject != "a medias" {
			t.Errorf("subject = %v, want a medias", body.Subject)
		}

		w.Write([]byte(`{"id":"D1","isDraft":true}`))
	}))
	defer srv.Close()

	subject := "a medias"
	res, err := newTestClient(t, srv).SaveDraft(context.Background(), "tok", DraftEmailRequestDto{Subject: &subject})
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if res.Id != "D1" {
		t.Errorf("id = %q, want D1", res.Id)
	}
}

func TestUpdateDraftPatchesTheDraftPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/email/drafts/D1" {
			t.Errorf("path = %q, want /email/drafts/D1", got)
		}
		if got := r.Method; got != http.MethodPatch {
			t.Errorf("method = %q, want PATCH", got)
		}
		w.Write([]byte(`{"id":"D1","isDraft":true}`))
	}))
	defer srv.Close()

	res, err := newTestClient(t, srv).UpdateDraft(context.Background(), "tok", "D1", DraftEmailRequestDto{})
	if err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}
	if res.Id != "D1" {
		t.Errorf("id = %q, want D1", res.Id)
	}
}

func TestDiscardDraftDeletesTheDraftPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/email/drafts/D1" {
			t.Errorf("path = %q, want /email/drafts/D1", got)
		}
		if got := r.Method; got != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := newTestClient(t, srv).DiscardDraft(context.Background(), "tok", "D1"); err != nil {
		t.Fatalf("DiscardDraft: %v", err)
	}
}
