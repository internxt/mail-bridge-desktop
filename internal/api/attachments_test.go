package api

import (
	"bytes"
	"context"
	"io"
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

func TestUploadAttachmentSendsAMultipartForm(t *testing.T) {
	content := []byte{0x89, 'P', 'N', 'G', 0x00, 0xff}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/email/attachment" {
			t.Errorf("path = %q, want /email/attachment", got)
		}

		file, header, err := r.FormFile("attachments")
		if err != nil {
			t.Fatalf("read the uploaded file: %v", err)
		}
		defer file.Close()

		if header.Filename != "foto.png" {
			t.Errorf("filename = %q, want foto.png", header.Filename)
		}
		if got := header.Header.Get("Content-Type"); got != "image/png" {
			t.Errorf("part Content-Type = %q, want image/png", got)
		}

		uploaded, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read the uploaded bytes: %v", err)
		}
		if !bytes.Equal(uploaded, content) {
			t.Errorf("uploaded %v, want the bytes as they were given", uploaded)
		}

		w.Write([]byte(`{"blobId":"B1","name":"foto.png","size":6,"type":"image/png"}`))
	}))
	defer srv.Close()

	res, err := newTestClient(t, srv).UploadAttachment(context.Background(), "tok", "foto.png", "image/png", content)
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if res.BlobId != "B1" {
		t.Errorf("blobId = %q, want B1", res.BlobId)
	}
}

// TestUploadAttachmentRefusesAnOversizedFile catches the limit before the
// bytes travel, so the caller gets a clear reason rather than a 413.
func TestUploadAttachmentRefusesAnOversizedFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("an oversized attachment should never reach the server")
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).UploadAttachment(context.Background(), "tok", "grande.bin", "application/octet-stream", make([]byte, MaxAttachmentBytes+1))
	if err == nil {
		t.Fatal("expected an error for a file over the limit")
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
