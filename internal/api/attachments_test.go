package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadAttachmentReturnsRawBytes(t *testing.T) {
	blob := []byte{0x00, 0x01, 0xff, 0xfe, 'n', 'o', 't', ' ', 'j', 's', 'o', 'n'}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/email/M1/attachment/B1" {
			t.Errorf("path = %q, want /email/M1/attachment/B1", got)
		}
		if got := r.Method; got != http.MethodGet {
			t.Errorf("method = %q, want GET", got)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(blob)
	}))
	defer srv.Close()

	downloaded, err := newTestClient(t, srv).DownloadAttachment(context.Background(), "tok", "M1", "B1")
	if err != nil {
		t.Fatalf("DownloadAttachment: %v", err)
	}
	if !bytes.Equal(downloaded, blob) {
		t.Errorf("downloaded %v, want the bytes as they were served", downloaded)
	}
}

func TestDownloadAttachmentReportsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := newTestClient(t, srv).DownloadAttachment(context.Background(), "tok", "M1", "B1"); err == nil {
		t.Fatal("expected an error for a blob that is not there")
	}
}
