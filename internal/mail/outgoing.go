package mail

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/mail/commands"
)

// ParseOutgoingMessage reads the raw RFC 5322 message an SMTP client handed
// over and extracts what SendEmail needs.
//
// to and cc come from the message's own To:/Cc: headers, which a client like
// Thunderbird writes with their real roles. bcc is whatever envelope
// recipient (envelopeRecipients, the SMTP RCPT TO list) is not addressed by
// either: SMTP itself does not distinguish bcc, and a well-behaved client
// omits the Bcc: header when it transmits the message so the other
// recipients never see it — this is how real mail transfer agents recover it
// too.
func ParseOutgoingMessage(raw []byte, envelopeRecipients []string) (commands.OutgoingMessage, error) {
	parsed, err := parseOutgoingMessage(raw, envelopeRecipients)
	if err != nil {
		return commands.OutgoingMessage{}, fmt.Errorf("parse outgoing message: %w", err)
	}
	return parsed, nil
}

func parseOutgoingMessage(raw []byte, envelopeRecipients []string) (commands.OutgoingMessage, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return commands.OutgoingMessage{}, err
	}

	subject, err := decodeSubject(msg.Header.Get("Subject"))
	if err != nil {
		return commands.OutgoingMessage{}, fmt.Errorf("decode subject: %w", err)
	}

	to, err := addressList(msg.Header, "To")
	if err != nil {
		return commands.OutgoingMessage{}, err
	}
	cc, err := addressList(msg.Header, "Cc")
	if err != nil {
		return commands.OutgoingMessage{}, err
	}

	textBody, htmlBody, err := readBody(msg.Header, msg.Body)
	if err != nil {
		return commands.OutgoingMessage{}, err
	}

	return commands.OutgoingMessage{
		Subject:  subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
		To:       to,
		Cc:       cc,
		Bcc:      bccFrom(to, cc, envelopeRecipients),
	}, nil
}

func decodeSubject(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	return new(mime.WordDecoder).DecodeHeader(raw)
}

func addressList(header mail.Header, field string) ([]api.EmailAddressDto, error) {
	if header.Get(field) == "" {
		return nil, nil
	}

	parsed, err := header.AddressList(field)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", field, err)
	}

	addresses := make([]api.EmailAddressDto, 0, len(parsed))
	for _, address := range parsed {
		dto := api.EmailAddressDto{Email: address.Address}
		if address.Name != "" {
			dto.Name = &address.Name
		}
		addresses = append(addresses, dto)
	}
	return addresses, nil
}

// bccFrom is the envelope recipients that neither To nor Cc addresses,
// case-insensitively. Duplicates are left as they arrive: the send path
// deduplicates every recipient anyway.
func bccFrom(to, cc []api.EmailAddressDto, envelopeRecipients []string) []api.EmailAddressDto {
	addressed := make(map[string]bool, len(to)+len(cc))
	for _, group := range [][]api.EmailAddressDto{to, cc} {
		for _, address := range group {
			addressed[strings.ToLower(address.Email)] = true
		}
	}

	var bcc []api.EmailAddressDto
	for _, recipient := range envelopeRecipients {
		if !addressed[strings.ToLower(recipient)] {
			bcc = append(bcc, api.EmailAddressDto{Email: recipient})
		}
	}
	return bcc
}

// readBody returns the plain-text and HTML bodies of a message, whichever it
// carries. A simple message has only one; a multipart/alternative one can
// carry both, mirroring what writeAlternative in mime.go produces on the way
// out.
func readBody(header mail.Header, body io.Reader) (text, html string, err error) {
	mediaType, params := mediaTypeOf(header.Get("Content-Type"))

	if strings.HasPrefix(mediaType, "multipart/") {
		return readMultipartBody(body, params["boundary"])
	}

	content, err := io.ReadAll(decodedPartReader(textproto.MIMEHeader(header), body))
	if err != nil {
		return "", "", fmt.Errorf("read body: %w", err)
	}
	if mediaType == "text/html" {
		return "", string(content), nil
	}
	return string(content), "", nil
}

// mediaTypeOf parses a Content-Type, treating a missing or unparseable one as
// the plain text a message without it is assumed to be, rather than failing:
// a body that arrived is better served as text than rejected outright.
func mediaTypeOf(contentType string) (string, map[string]string) {
	if contentType == "" {
		return "text/plain", nil
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "text/plain", nil
	}
	return mediaType, params
}

func readMultipartBody(body io.Reader, boundary string) (text, html string, err error) {
	reader := multipart.NewReader(body, boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", fmt.Errorf("read multipart body: %w", err)
		}

		mediaType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			continue
		}

		content, err := io.ReadAll(decodedPartReader(part.Header, part))
		if err != nil {
			return "", "", fmt.Errorf("read part: %w", err)
		}

		switch mediaType {
		case "text/plain":
			text = string(content)
		case "text/html":
			html = string(content)
		}
	}
	return text, html, nil
}

// decodedPartReader undoes a part's Content-Transfer-Encoding, mirroring what
// writeAlternative in mime.go applies on the way out: quoted-printable, or
// the bytes as they are for anything else.
func decodedPartReader(header textproto.MIMEHeader, r io.Reader) io.Reader {
	if strings.EqualFold(header.Get("Content-Transfer-Encoding"), "quoted-printable") {
		return quotedprintable.NewReader(r)
	}
	return r
}
