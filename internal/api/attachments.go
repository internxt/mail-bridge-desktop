package api

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
)

const (
	attachmentUploadPath = emailPath + "/attachment"
	attachmentField      = "attachments"
	MaxAttachmentBytes   = 25 * 1024 * 1024
)

// UploadAttachment stores a file and returns the reference an email attaches
// it by.
func (c *Client) UploadAttachment(ctx context.Context, token, name, contentType string, content []byte) (UploadAttachmentResponseDto, error) {
	if len(content) > MaxAttachmentBytes {
		return UploadAttachmentResponseDto{}, fmt.Errorf("api: attachment %q is %d bytes, over the %d-byte limit", name, len(content), MaxAttachmentBytes)
	}

	body, formContentType, err := attachmentForm(name, contentType, content)
	if err != nil {
		return UploadAttachmentResponseDto{}, err
	}

	var res UploadAttachmentResponseDto
	err = c.do(ctx, request{
		svc:         c.mail,
		method:      http.MethodPost,
		path:        attachmentUploadPath,
		token:       token,
		body:        body,
		contentType: formContentType,
	}, &res)

	return res, err
}

// attachmentForm builds the multipart body in full, so a retry can send the
// same bytes again.
func attachmentForm(name, contentType string, content []byte) ([]byte, string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf("form-data; name=%q; filename=%q", attachmentField, escapeQuotes(name)))
	header.Set("Content-Type", contentType)

	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, "", fmt.Errorf("api: build attachment form: %w", err)
	}
	if _, err := part.Write(content); err != nil {
		return nil, "", fmt.Errorf("api: build attachment form: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("api: build attachment form: %w", err)
	}

	return buf.Bytes(), writer.FormDataContentType(), nil
}

// escapeQuotes keeps a filename from ending the quoted form-data parameter
// early, which is what mime/multipart does for its own fields.
func escapeQuotes(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(value)
}

func (c *Client) DownloadAttachment(ctx context.Context, token, emailID, blobID string) ([]byte, error) {
	return c.doRaw(ctx, request{
		svc:    c.mail,
		method: http.MethodGet,
		path:   emailPath + "/" + escapeID(emailID) + "/attachment/" + escapeID(blobID),
		token:  token,
	})
}
