package mail

import (
	"testing"
)

func TestParseOutgoingMessagePlainText(t *testing.T) {
	raw := "From: alice@inxt.eu\r\n" +
		"To: Bob <bob@inxt.eu>\r\n" +
		"Subject: hola\r\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
		"\r\n" +
		"cuerpo del mensaje\r\n"

	msg, err := ParseOutgoingMessage([]byte(raw), []string{"bob@inxt.eu"})
	if err != nil {
		t.Fatalf("ParseOutgoingMessage: %v", err)
	}
	if msg.Subject != "hola" {
		t.Errorf("Subject = %q, want hola", msg.Subject)
	}
	if msg.TextBody != "cuerpo del mensaje\r\n" {
		t.Errorf("TextBody = %q", msg.TextBody)
	}
	if len(msg.To) != 1 || msg.To[0].Email != "bob@inxt.eu" {
		t.Fatalf("To = %+v", msg.To)
	}
	if msg.To[0].Name == nil || *msg.To[0].Name != "Bob" {
		t.Errorf("To[0].Name = %v, want Bob", msg.To[0].Name)
	}
	if len(msg.Bcc) != 0 {
		t.Errorf("Bcc = %+v, want none", msg.Bcc)
	}
}

func TestParseOutgoingMessageEncodedSubject(t *testing.T) {
	raw := "From: alice@inxt.eu\r\n" +
		"To: bob@inxt.eu\r\n" +
		"Subject: =?utf-8?q?Reuni=C3=B3n=3A_caf=C3=A9?=\r\n" +
		"\r\n" +
		"cuerpo\r\n"

	msg, err := ParseOutgoingMessage([]byte(raw), []string{"bob@inxt.eu"})
	if err != nil {
		t.Fatalf("ParseOutgoingMessage: %v", err)
	}
	if want := "Reunión: café"; msg.Subject != want {
		t.Errorf("Subject = %q, want %q", msg.Subject, want)
	}
}

func TestParseOutgoingMessageQuotedPrintableBody(t *testing.T) {
	raw := "From: alice@inxt.eu\r\n" +
		"To: bob@inxt.eu\r\n" +
		"Subject: hola\r\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"Hola, con acentos: =C3=B1 =C3=A1\r\n"

	msg, err := ParseOutgoingMessage([]byte(raw), []string{"bob@inxt.eu"})
	if err != nil {
		t.Fatalf("ParseOutgoingMessage: %v", err)
	}
	if want := "Hola, con acentos: ñ á\r\n"; msg.TextBody != want {
		t.Errorf("TextBody = %q, want %q", msg.TextBody, want)
	}
}

func TestParseOutgoingMessageMultipartAlternative(t *testing.T) {
	raw := "From: alice@inxt.eu\r\n" +
		"To: bob@inxt.eu\r\n" +
		"Subject: hola\r\n" +
		"Content-Type: multipart/alternative; boundary=\"BOUND\"\r\n" +
		"\r\n" +
		"--BOUND\r\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"texto plano\r\n" +
		"--BOUND\r\n" +
		"Content-Type: text/html; charset=\"utf-8\"\r\n" +
		"\r\n" +
		"<p>html</p>\r\n" +
		"--BOUND--\r\n"

	msg, err := ParseOutgoingMessage([]byte(raw), []string{"bob@inxt.eu"})
	if err != nil {
		t.Fatalf("ParseOutgoingMessage: %v", err)
	}
	if msg.TextBody != "texto plano" {
		t.Errorf("TextBody = %q", msg.TextBody)
	}
	if msg.HTMLBody != "<p>html</p>" {
		t.Errorf("HTMLBody = %q", msg.HTMLBody)
	}
}

// messageWithAttachment is what a client sends once a file is attached: a
// multipart/mixed wrapping the body — itself a multipart/alternative — and the
// file beside it.
const messageWithAttachment = "From: alice@inxt.eu\r\n" +
	"To: bob@inxt.eu\r\n" +
	"Subject: con adjunto\r\n" +
	"Content-Type: multipart/mixed; boundary=\"OUTER\"\r\n" +
	"\r\n" +
	"--OUTER\r\n" +
	"Content-Type: multipart/alternative; boundary=\"INNER\"\r\n" +
	"\r\n" +
	"--INNER\r\n" +
	"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
	"\r\n" +
	"texto plano\r\n" +
	"--INNER\r\n" +
	"Content-Type: text/html; charset=\"utf-8\"\r\n" +
	"\r\n" +
	"<p>html</p>\r\n" +
	"--INNER--\r\n" +
	"--OUTER\r\n" +
	"Content-Type: text/plain; name=\"notas.txt\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"Content-Disposition: attachment; filename=\"notas.txt\"\r\n" +
	"\r\n" +
	"ZWwgY29udGVuaWRvIGRlbCBhZGp1bnRv\r\n" +
	"--OUTER--\r\n"

// TestParseOutgoingMessageReadsTheBodyBesideAnAttachment is the regression
// guard for a body that went missing: attaching a file nests the bodies one
// level deeper, and reading only the top level found neither of them, so the
// message was sent with nothing in it.
func TestParseOutgoingMessageReadsTheBodyBesideAnAttachment(t *testing.T) {
	msg, err := ParseOutgoingMessage([]byte(messageWithAttachment), []string{"bob@inxt.eu"})
	if err != nil {
		t.Fatalf("ParseOutgoingMessage: %v", err)
	}

	if msg.TextBody != "texto plano" {
		t.Errorf("TextBody = %q, want the body from inside the nested part", msg.TextBody)
	}
	if msg.HTMLBody != "<p>html</p>" {
		t.Errorf("HTMLBody = %q, want the body from inside the nested part", msg.HTMLBody)
	}
}

func TestParseOutgoingMessageExtractsTheAttachment(t *testing.T) {
	msg, err := ParseOutgoingMessage([]byte(messageWithAttachment), []string{"bob@inxt.eu"})
	if err != nil {
		t.Fatalf("ParseOutgoingMessage: %v", err)
	}

	if len(msg.Attachments) != 1 {
		t.Fatalf("got %d attachments, want 1", len(msg.Attachments))
	}

	attachment := msg.Attachments[0]
	if attachment.Name != "notas.txt" {
		t.Errorf("name = %q, want notas.txt", attachment.Name)
	}
	if attachment.ContentType != "text/plain" {
		t.Errorf("content type = %q, want text/plain", attachment.ContentType)
	}
	if want := "el contenido del adjunto"; string(attachment.Content) != want {
		t.Errorf("content = %q, want %q (base64 should be decoded)", attachment.Content, want)
	}
}

// TestParseOutgoingMessageDecodesTheAttachmentName covers the name a client
// encodes because it is not ASCII.
func TestParseOutgoingMessageDecodesTheAttachmentName(t *testing.T) {
	raw := "From: alice@inxt.eu\r\n" +
		"To: bob@inxt.eu\r\n" +
		"Subject: hola\r\n" +
		"Content-Type: multipart/mixed; boundary=\"B\"\r\n" +
		"\r\n" +
		"--B\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"cuerpo\r\n" +
		"--B\r\n" +
		"Content-Type: application/pdf\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: attachment; filename=\"=?utf-8?q?informe_a=C3=B1o.pdf?=\"\r\n" +
		"\r\n" +
		"eA==\r\n" +
		"--B--\r\n"

	msg, err := ParseOutgoingMessage([]byte(raw), []string{"bob@inxt.eu"})
	if err != nil {
		t.Fatalf("ParseOutgoingMessage: %v", err)
	}
	if len(msg.Attachments) != 1 {
		t.Fatalf("got %d attachments, want 1", len(msg.Attachments))
	}
	if want := "informe año.pdf"; msg.Attachments[0].Name != want {
		t.Errorf("name = %q, want %q", msg.Attachments[0].Name, want)
	}
}

// TestParseOutgoingMessageWithoutAttachmentsFindsNone keeps an ordinary
// message from growing attachments it never had.
func TestParseOutgoingMessageWithoutAttachmentsFindsNone(t *testing.T) {
	raw := "From: alice@inxt.eu\r\n" +
		"To: bob@inxt.eu\r\n" +
		"Subject: hola\r\n" +
		"\r\n" +
		"cuerpo\r\n"

	msg, err := ParseOutgoingMessage([]byte(raw), []string{"bob@inxt.eu"})
	if err != nil {
		t.Fatalf("ParseOutgoingMessage: %v", err)
	}
	if len(msg.Attachments) != 0 {
		t.Errorf("got %d attachments, want none", len(msg.Attachments))
	}
}

// TestParseOutgoingMessageInfersBcc is the case that matters: Thunderbird
// omits the Bcc: header when it transmits, so the only way to recover it is
// the difference between RCPT TO and what To:/Cc: address.
func TestParseOutgoingMessageInfersBcc(t *testing.T) {
	raw := "From: alice@inxt.eu\r\n" +
		"To: bob@inxt.eu\r\n" +
		"Cc: carol@inxt.eu\r\n" +
		"Subject: hola\r\n" +
		"\r\n" +
		"cuerpo\r\n"

	msg, err := ParseOutgoingMessage([]byte(raw), []string{"bob@inxt.eu", "carol@inxt.eu", "dave@inxt.eu"})
	if err != nil {
		t.Fatalf("ParseOutgoingMessage: %v", err)
	}
	if len(msg.Bcc) != 1 || msg.Bcc[0].Email != "dave@inxt.eu" {
		t.Fatalf("Bcc = %+v, want [dave@inxt.eu]", msg.Bcc)
	}
}

func TestParseOutgoingMessageBccIsCaseInsensitive(t *testing.T) {
	raw := "From: alice@inxt.eu\r\n" +
		"To: Bob@Inxt.eu\r\n" +
		"Subject: hola\r\n" +
		"\r\n" +
		"cuerpo\r\n"

	msg, err := ParseOutgoingMessage([]byte(raw), []string{"bob@inxt.eu"})
	if err != nil {
		t.Fatalf("ParseOutgoingMessage: %v", err)
	}
	if len(msg.Bcc) != 0 {
		t.Errorf("Bcc = %+v, want none (bob@inxt.eu is already in To, case aside)", msg.Bcc)
	}
}

func TestParseOutgoingMessageRejectsMalformedInput(t *testing.T) {
	if _, err := ParseOutgoingMessage([]byte("not a valid message at all\x00\x01"), nil); err == nil {
		t.Fatal("expected an error")
	}
}
