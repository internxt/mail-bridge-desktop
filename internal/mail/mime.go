package mail

import (
	"bytes"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"

	"mail-bridge-desktop/internal/api"
)

// messageIDDomain labels the Message-IDs the bridge generates for emails the
// API returns without one.
const messageIDDomain = "mail-bridge.internxt.local"

// BuildLiteral turns an email into the RFC 5322 message an IMAP client expects.
//
// The API returns an email as separate fields, not as a message, so the literal
// has to be assembled here. Clients cache messages by UID, so the same email
// must always produce the same bytes: nothing in here may depend on the current
// time or on random values.
func BuildLiteral(email api.EmailResponseDto) ([]byte, error) {
	var buf bytes.Buffer

	if err := writeHeaders(&buf, email); err != nil {
		return nil, err
	}
	if err := writeBody(&buf, email); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeHeaders(buf *bytes.Buffer, email api.EmailResponseDto) error {
	date, err := parseDate(email)
	if err != nil {
		return err
	}

	writeHeader(buf, "MIME-Version", "1.0")
	writeHeader(buf, "Message-ID", messageID(email))
	writeHeader(buf, "Date", date.Format(time.RFC1123Z))
	// Subjects are encoded because they routinely carry non-ASCII text.
	writeHeader(buf, "Subject", mime.QEncoding.Encode("utf-8", email.Subject))
	writeHeader(buf, "From", formatAddresses(email.From))
	writeHeader(buf, "To", formatAddresses(email.To))
	writeHeader(buf, "Cc", formatAddresses(email.Cc))
	writeHeader(buf, "Reply-To", formatAddresses(email.ReplyTo))

	// Bcc is deliberately left out: it must not travel with the message.
	return nil
}

// writeHeader skips empty values, since an empty header is worse than none.
func writeHeader(buf *bytes.Buffer, name, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(buf, "%s: %s\r\n", name, value)
}

func writeBody(buf *bytes.Buffer, email api.EmailResponseDto) error {
	text := deref(email.TextBody)
	html := deref(email.HtmlBody)

	switch {
	case text != "" && html != "":
		return writeAlternative(buf, text, html)
	case html != "":
		return writeSinglePart(buf, "text/html", html)
	default:
		// An email with neither body still has to be a valid message, so an
		// empty text part stands in for it.
		return writeSinglePart(buf, "text/plain", text)
	}
}

func writeSinglePart(buf *bytes.Buffer, contentType, body string) error {
	writeHeader(buf, "Content-Type", contentType+`; charset="utf-8"`)
	writeHeader(buf, "Content-Transfer-Encoding", "quoted-printable")
	buf.WriteString("\r\n")
	return writeQuotedPrintable(buf, body)
}

// writeAlternative writes both bodies, letting the client pick. The plain text
// part goes first, as the least capable alternative.
func writeAlternative(buf *bytes.Buffer, text, html string) error {
	writer := multipart.NewWriter(buf)
	if err := writer.SetBoundary(boundary(text, html)); err != nil {
		return fmt.Errorf("build message: set boundary: %w", err)
	}

	writeHeader(buf, "Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", writer.Boundary()))
	buf.WriteString("\r\n")

	for _, part := range []struct{ contentType, body string }{
		{"text/plain", text},
		{"text/html", html},
	} {
		w, err := writer.CreatePart(map[string][]string{
			"Content-Type":              {part.contentType + `; charset="utf-8"`},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return fmt.Errorf("build message: create part: %w", err)
		}
		qp := quotedprintable.NewWriter(w)
		if _, err := qp.Write([]byte(part.body)); err != nil {
			return fmt.Errorf("build message: write part: %w", err)
		}
		if err := qp.Close(); err != nil {
			return fmt.Errorf("build message: close part: %w", err)
		}
	}
	return writer.Close()
}

func writeQuotedPrintable(buf *bytes.Buffer, body string) error {
	w := quotedprintable.NewWriter(buf)
	if _, err := w.Write([]byte(body)); err != nil {
		return fmt.Errorf("build message: write body: %w", err)
	}
	return w.Close()
}

// messageID reuses the email's own identifier, so refetching the same email
// yields the same Message-ID and clients do not duplicate it.
func messageID(email api.EmailResponseDto) string {
	return fmt.Sprintf("<%s@%s>", email.Id, messageIDDomain)
}

// boundary derives a separator from the content, keeping the literal stable
// across calls. multipart would otherwise pick a random one every time.
func boundary(parts ...string) string {
	var sum uint64 = 14695981039346656037 // FNV-1a offset basis
	for _, part := range parts {
		for i := 0; i < len(part); i++ {
			sum ^= uint64(part[i])
			sum *= 1099511628211 // FNV-1a prime
		}
	}
	return fmt.Sprintf("%016x", sum)
}

// parseDate prefers the moment the email was sent, falling back to when it was
// received.
func parseDate(email api.EmailResponseDto) (time.Time, error) {
	raw := deref(email.SentAt)
	if raw == "" {
		raw = email.ReceivedAt
	}
	if raw == "" {
		return time.Time{}, fmt.Errorf("build message %s: no date", email.Id)
	}

	date, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("build message %s: parse date %q: %w", email.Id, raw, err)
	}
	return date, nil
}

// formatAddresses renders an address list, encoding display names that carry
// non-ASCII characters.
func formatAddresses(addresses []api.EmailAddressDto) string {
	formatted := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address.Email == "" {
			continue
		}
		formatted = append(formatted, (&mail.Address{
			Name:    deref(address.Name),
			Address: address.Email,
		}).String())
	}
	return strings.Join(formatted, ", ")
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
