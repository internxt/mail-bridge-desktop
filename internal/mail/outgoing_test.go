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
