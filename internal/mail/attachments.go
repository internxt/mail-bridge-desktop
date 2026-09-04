package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"strings"

	"mail-bridge-desktop/internal/api"
)

// attachmentLineLength is how many base64 characters go on one line, the
// customary width for a MIME body.
const attachmentLineLength = 76

// BuildLiteralWithAttachments turns an email into the RFC 5322 message an IMAP
// client expects, carrying its attachments.
//
// blobs holds the decrypted bytes of each attachment, keyed by blob ID. An
// attachment missing from it is still written in full — headers, and the exact
// number of base64 lines its bytes will occupy, filled with padding. That is
// what the sync announces: the message has the size and structure it will
// finally have, so a client sees the attachment and its size without a single
// byte having been downloaded, and the bytes themselves arrive when the client
// first asks for the body.
func BuildLiteralWithAttachments(email api.EmailResponseDto, blobs map[string][]byte) ([]byte, error) {
	if len(email.Attachments) == 0 {
		return BuildLiteral(email)
	}

	var body bytes.Buffer
	if err := writeBody(&body, email); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := writeHeaders(&buf, email); err != nil {
		return nil, err
	}

	boundary := attachmentBoundary(email)
	writeHeader(&buf, "Content-Type", fmt.Sprintf("multipart/mixed; boundary=%q", boundary))
	buf.WriteString("\r\n")

	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	buf.Write(body.Bytes())

	for _, attachment := range email.Attachments {
		fmt.Fprintf(&buf, "\r\n--%s\r\n", boundary)
		writeAttachmentPart(&buf, attachment, blobs[attachment.BlobId])
	}

	fmt.Fprintf(&buf, "\r\n--%s--\r\n", boundary)
	return buf.Bytes(), nil
}

func writeAttachmentPart(buf *bytes.Buffer, attachment api.EmailAttachmentDto, content []byte) {
	contentType := attachment.Type
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	writeHeader(buf, "Content-Type", fmt.Sprintf("%s; name=%q", contentType, attachment.Name))
	writeHeader(buf, "Content-Transfer-Encoding", "base64")
	writeHeader(buf, "Content-Disposition", fmt.Sprintf("attachment; filename=%q", encodedFilename(attachment.Name)))
	buf.WriteString("\r\n")

	if content != nil {
		writeBase64(buf, base64.StdEncoding.EncodeToString(content))
		return
	}

	writeBase64(buf, strings.Repeat("A", base64.StdEncoding.EncodedLen(attachmentSize(attachment))))
}

func writeBase64(buf *bytes.Buffer, encoded string) {
	for len(encoded) > attachmentLineLength {
		buf.WriteString(encoded[:attachmentLineLength])
		buf.WriteString("\r\n")
		encoded = encoded[attachmentLineLength:]
	}
	buf.WriteString(encoded)
	buf.WriteString("\r\n")
}

func attachmentSize(attachment api.EmailAttachmentDto) int {
	if attachment.Size <= 0 {
		return 0
	}
	return int(attachment.Size)
}

func encodedFilename(name string) string {
	if isASCII(name) {
		return name
	}
	return mime.QEncoding.Encode("utf-8", name)
}

func isASCII(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] > 127 {
			return false
		}
	}
	return true
}

func attachmentBoundary(email api.EmailResponseDto) string {
	parts := make([]string, 0, len(email.Attachments)+1)
	parts = append(parts, email.Id)
	for _, attachment := range email.Attachments {
		parts = append(parts, attachment.BlobId)
	}
	return "mixed" + boundary(parts...)
}
