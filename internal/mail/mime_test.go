package mail

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"mail-bridge-desktop/internal/api"
)

func ptr(s string) *string { return &s }

func sampleEmail() api.EmailResponseDto {
	return api.EmailResponseDto{
		Id:         "M1a2b3c",
		ThreadId:   "T1a2b3c",
		Subject:    "Reunión de mañana: café ☕",
		ReceivedAt: "2026-06-15T10:30:00Z",
		SentAt:     ptr("2026-06-15T10:29:55Z"),
		From:       []api.EmailAddressDto{{Email: "alice@inxt.com", Name: ptr("Alice Smith")}},
		To:         []api.EmailAddressDto{{Email: "bob@inxt.com"}},
		TextBody:   ptr("Hola equipo,\r\nAquí están las notas.\r\n"),
	}
}

// parseLiteral builds the message and parses it back, which is what a mail
// client will do.
func parseLiteral(t *testing.T, email api.EmailResponseDto) *mail.Message {
	t.Helper()

	literal, err := BuildLiteral(email)
	if err != nil {
		t.Fatalf("BuildLiteral: %v", err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(literal))
	if err != nil {
		t.Fatalf("the message does not parse: %v\n---\n%s", err, literal)
	}
	return msg
}

func TestBuildLiteralHeaders(t *testing.T) {
	msg := parseLiteral(t, sampleEmail())

	// A non-ASCII subject has to survive the round trip.
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil {
		t.Fatalf("decode subject: %v", err)
	}
	if want := "Reunión de mañana: café ☕"; subject != want {
		t.Errorf("Subject = %q, want %q", subject, want)
	}

	from, err := msg.Header.AddressList("From")
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	if len(from) != 1 || from[0].Address != "alice@inxt.com" || from[0].Name != "Alice Smith" {
		t.Errorf("From = %+v", from)
	}

	// The date has to be RFC 5322, not the ISO-8601 the API returns.
	date, err := msg.Header.Date()
	if err != nil {
		t.Fatalf("Date: %v", err)
	}
	if got := date.UTC().Format("2006-01-02T15:04:05Z"); got != "2026-06-15T10:29:55Z" {
		t.Errorf("Date = %s, want the SentAt value", got)
	}

	if got := msg.Header.Get("Message-ID"); !strings.Contains(got, "M1a2b3c") {
		t.Errorf("Message-ID = %q, want it to carry the email ID", got)
	}
}

// Clients cache by identity, so the same email must always produce the same
// bytes.
func TestBuildLiteralIsStable(t *testing.T) {
	email := sampleEmail()
	email.HtmlBody = ptr("<p>Hola equipo</p>")

	first, err := BuildLiteral(email)
	if err != nil {
		t.Fatalf("BuildLiteral: %v", err)
	}
	second, err := BuildLiteral(email)
	if err != nil {
		t.Fatalf("BuildLiteral: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("two builds of the same email differ")
	}
}

func TestBuildLiteralTextOnly(t *testing.T) {
	msg := parseLiteral(t, sampleEmail())

	if got := msg.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", got)
	}
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "Hola equipo") {
		t.Errorf("body = %q", body)
	}
}

func TestBuildLiteralMultipart(t *testing.T) {
	email := sampleEmail()
	email.HtmlBody = ptr("<p>Hola equipo</p>")

	msg := parseLiteral(t, email)

	contentType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse Content-Type: %v", err)
	}
	if contentType != "multipart/alternative" {
		t.Fatalf("Content-Type = %q, want multipart/alternative", contentType)
	}

	reader := multipart.NewReader(msg.Body, params["boundary"])
	var types []string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		mediaType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("parse part Content-Type: %v", err)
		}
		types = append(types, mediaType)
	}

	// Plain text goes first: it is the least capable alternative.
	if len(types) != 2 || types[0] != "text/plain" || types[1] != "text/html" {
		t.Fatalf("parts = %v, want [text/plain text/html]", types)
	}
}

// An email with no body at all still has to be a valid message.
func TestBuildLiteralWithoutBody(t *testing.T) {
	email := sampleEmail()
	email.TextBody = nil

	msg := parseLiteral(t, email)
	if _, err := io.ReadAll(msg.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
}

// Bcc must never travel with the message.
func TestBuildLiteralOmitsBcc(t *testing.T) {
	email := sampleEmail()
	email.Bcc = []api.EmailAddressDto{{Email: "hidden@inxt.com"}}

	literal, err := BuildLiteral(email)
	if err != nil {
		t.Fatalf("BuildLiteral: %v", err)
	}
	if bytes.Contains(literal, []byte("hidden@inxt.com")) {
		t.Fatal("the Bcc address leaked into the message")
	}
}

func TestBuildLiteralRejectsBadDate(t *testing.T) {
	email := sampleEmail()
	email.SentAt = nil
	email.ReceivedAt = "15/06/2026"

	if _, err := BuildLiteral(email); err == nil {
		t.Fatal("expected an error for an unparseable date")
	}
}
