package mail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"testing"

	"mail-bridge-desktop/internal/api"
)

func emailWithAttachment(content []byte) api.EmailResponseDto {
	email := sampleEmail()
	email.Attachments = []api.EmailAttachmentDto{{
		BlobId: "B1",
		Name:   "notas.txt",
		Type:   "text/plain",
		Size:   float32(len(content)),
	}}
	email.HasAttachment = true
	return email
}

// messagePart is one part of the message, read out while it is still the
// current one: a multipart.Part stops being readable as soon as the next is
// requested.
type messagePart struct {
	header   textproto.MIMEHeader
	filename string
	content  []byte
}

// parts reads the message back the way a mail client does.
func parts(t *testing.T, literal []byte) []messagePart {
	t.Helper()

	msg, err := mail.ReadMessage(bytes.NewReader(literal))
	if err != nil {
		t.Fatalf("the message does not parse: %v\n---\n%s", err, literal)
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse Content-Type: %v", err)
	}
	if mediaType != "multipart/mixed" {
		t.Fatalf("Content-Type = %q, want multipart/mixed", mediaType)
	}

	reader := multipart.NewReader(msg.Body, params["boundary"])
	var collected []messagePart
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read part: %v\n---\n%s", err, literal)
		}

		content, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part body: %v", err)
		}
		collected = append(collected, messagePart{
			header:   part.Header,
			filename: part.FileName(),
			content:  content,
		})
	}
	return collected
}

func TestBuildLiteralWithAttachmentsCarriesTheFile(t *testing.T) {
	content := []byte("el contenido del adjunto, con acentos: ñ á é\n")
	email := emailWithAttachment(content)

	literal, err := BuildLiteralWithAttachments(email, map[string][]byte{"B1": content})
	if err != nil {
		t.Fatalf("BuildLiteralWithAttachments: %v", err)
	}

	collected := parts(t, literal)
	if len(collected) != 2 {
		t.Fatalf("got %d parts, want the body and one attachment", len(collected))
	}

	attachment := collected[1]
	if attachment.filename != "notas.txt" {
		t.Errorf("filename = %q, want notas.txt", attachment.filename)
	}
	if got := attachment.header.Get("Content-Transfer-Encoding"); got != "base64" {
		t.Errorf("encoding = %q, want base64", got)
	}

	// multipart.Part undoes quoted-printable but not base64, so this decodes
	// the part the way a client would.
	decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(attachment.content)))
	if err != nil {
		t.Fatalf("decode attachment: %v", err)
	}
	if !bytes.Equal(decoded, content) {
		t.Errorf("attachment = %q, want %q", decoded, content)
	}
}

// TestBuildLiteralWithAttachmentsKeepsTheSameSize is what lets the sync
// announce a message without downloading it: Gluon stores the size and
// structure from the literal it is given, and serves the body from the store
// later. If the two literals differ in length, the size a client is told
// stops matching what it eventually receives.
func TestBuildLiteralWithAttachmentsKeepsTheSameSize(t *testing.T) {
	content := []byte("el contenido del adjunto, con acentos: ñ á é\n")
	email := emailWithAttachment(content)

	reserved, err := BuildLiteralWithAttachments(email, nil)
	if err != nil {
		t.Fatalf("BuildLiteralWithAttachments (reserved): %v", err)
	}
	complete, err := BuildLiteralWithAttachments(email, map[string][]byte{"B1": content})
	if err != nil {
		t.Fatalf("BuildLiteralWithAttachments (complete): %v", err)
	}

	if len(reserved) != len(complete) {
		t.Errorf("reserved literal is %d bytes and the complete one %d; a client would be told the wrong size",
			len(reserved), len(complete))
	}
}

// TestBuildLiteralWithAttachmentsAnnouncesStructureWithoutBytes covers what the
// sync produces: the attachment is visible, named and typed, so a client shows
// the paperclip before anything has been downloaded.
func TestBuildLiteralWithAttachmentsAnnouncesStructureWithoutBytes(t *testing.T) {
	email := emailWithAttachment(make([]byte, 128))

	literal, err := BuildLiteralWithAttachments(email, nil)
	if err != nil {
		t.Fatalf("BuildLiteralWithAttachments: %v", err)
	}

	collected := parts(t, literal)
	if len(collected) != 2 {
		t.Fatalf("got %d parts, want the body and one attachment", len(collected))
	}
	if got := collected[1].filename; got != "notas.txt" {
		t.Errorf("filename = %q, want notas.txt even with no bytes yet", got)
	}
}

func TestBuildLiteralWithoutAttachmentsIsUnchanged(t *testing.T) {
	email := sampleEmail()

	withHelper, err := BuildLiteralWithAttachments(email, nil)
	if err != nil {
		t.Fatalf("BuildLiteralWithAttachments: %v", err)
	}
	plain, err := BuildLiteral(email)
	if err != nil {
		t.Fatalf("BuildLiteral: %v", err)
	}

	if !bytes.Equal(withHelper, plain) {
		t.Error("an email with no attachments should build the same message as before")
	}
}

// TestBuildLiteralWithAttachmentsEncodesTheFilename keeps a non-ASCII name
// from breaking the header.
func TestBuildLiteralWithAttachmentsEncodesTheFilename(t *testing.T) {
	content := []byte("x")
	email := emailWithAttachment(content)
	email.Attachments[0].Name = "informe año.txt"

	literal, err := BuildLiteralWithAttachments(email, map[string][]byte{"B1": content})
	if err != nil {
		t.Fatalf("BuildLiteralWithAttachments: %v", err)
	}

	collected := parts(t, literal)
	decoded, err := new(mime.WordDecoder).DecodeHeader(collected[1].filename)
	if err != nil {
		t.Fatalf("decode filename: %v", err)
	}
	if decoded != "informe año.txt" {
		t.Errorf("filename = %q, want informe año.txt", decoded)
	}
}
