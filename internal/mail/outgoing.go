package mail

import (
	"bytes"
	"encoding/base64"
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

	content, err := readBody(msg.Header, msg.Body)
	if err != nil {
		return commands.OutgoingMessage{}, err
	}

	return commands.OutgoingMessage{
		Subject:          subject,
		TextBody:         content.text,
		HTMLBody:         content.html,
		Attachments:      content.attachments,
		InReplyToEmailID: repliedEmailID(msg.Header.Get("In-Reply-To")),
		To:               to,
		Cc:               cc,
		Bcc:              bccFrom(to, cc, envelopeRecipients),
	}, nil
}

// repliedEmailID is the email this message replies to, when that email is one
// the bridge served.
func repliedEmailID(inReplyTo string) string {
	if inReplyTo == "" {
		return ""
	}

	ids := strings.Fields(inReplyTo)
	emailID, found := EmailIDFromMessageID(ids[len(ids)-1])
	if !found {
		return ""
	}
	return emailID
}

// messageContent is what a message carries: its bodies, and the files
// attached to it.
type messageContent struct {
	text        string
	html        string
	attachments []commands.OutgoingAttachment
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

// readBody returns the bodies of a message and the files hanging off it.
//
// A simple message has one body; a multipart/alternative one can carry both,
// mirroring what writeAlternative in mime.go produces on the way out. A
// message with attachments wraps that in a multipart/mixed, so the parts are
// walked recursively rather than only at the top level.
func readBody(header mail.Header, body io.Reader) (content messageContent, err error) {
	mediaType, params := mediaTypeOf(header.Get("Content-Type"))

	if strings.HasPrefix(mediaType, "multipart/") {
		err := readParts(&content, body, params["boundary"])
		return content, err
	}

	text, err := io.ReadAll(decodedPartReader(textproto.MIMEHeader(header), body))
	if err != nil {
		return messageContent{}, fmt.Errorf("read body: %w", err)
	}
	if mediaType == "text/html" {
		content.html = string(text)
	} else {
		content.text = string(text)
	}
	return content, nil
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

// readParts walks one multipart level, descending into any nested one.
func readParts(content *messageContent, body io.Reader, boundary string) error {
	if boundary == "" {
		return fmt.Errorf("read multipart body: no boundary")
	}

	reader := multipart.NewReader(body, boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read multipart body: %w", err)
		}

		if err := readPart(content, part); err != nil {
			return err
		}
	}
}

func readPart(content *messageContent, part *multipart.Part) error {
	mediaType, params := mediaTypeOf(part.Header.Get("Content-Type"))

	if strings.HasPrefix(mediaType, "multipart/") {
		return readParts(content, part, params["boundary"])
	}

	decoded, err := io.ReadAll(decodedPartReader(part.Header, part))
	if err != nil {
		return fmt.Errorf("read part: %w", err)
	}

	if name := attachmentName(part, params); name != "" {
		content.attachments = append(content.attachments, commands.OutgoingAttachment{
			Name:        name,
			ContentType: mediaType,
			Content:     decoded,
		})
		return nil
	}

	switch mediaType {
	case "text/plain":
		content.text = string(decoded)
	case "text/html":
		content.html = string(decoded)
	}
	return nil
}

// attachmentName returns the file name a part travels under, or empty when the
// part is not a file.
func attachmentName(part *multipart.Part, contentTypeParams map[string]string) string {
	disposition, dispositionParams := mediaTypeOf(part.Header.Get("Content-Disposition"))

	name := dispositionParams["filename"]
	if name == "" {
		name = contentTypeParams["name"]
	}
	if name == "" {
		if strings.EqualFold(disposition, "attachment") {
			return "attachment"
		}
		return ""
	}

	if decoded, err := new(mime.WordDecoder).DecodeHeader(name); err == nil {
		return decoded
	}
	return name
}

// decodedPartReader undoes a part's Content-Transfer-Encoding: base64 for the
// files a client attaches, quoted-printable for the text it writes, and the
// bytes as they are for anything else.
func decodedPartReader(header textproto.MIMEHeader, r io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, newlineStripper{r})
	default:
		return r
	}
}

type newlineStripper struct{ inner io.Reader }

func (s newlineStripper) Read(p []byte) (int, error) {
	read, err := s.inner.Read(p)

	kept := 0
	for i := 0; i < read; i++ {
		if p[i] != '\r' && p[i] != '\n' {
			p[kept] = p[i]
			kept++
		}
	}

	// Read must not report zero bytes with a nil error, which is what stripping
	// a chunk of pure line breaks would do.
	if kept == 0 && err == nil {
		return s.Read(p)
	}
	return kept, err
}
