package api

import (
	"context"
	"net/http"
)

func (c *Client) DownloadAttachment(ctx context.Context, token, emailID, blobID string) ([]byte, error) {
	return c.doRaw(ctx, request{
		svc:    c.mail,
		method: http.MethodGet,
		path:   emailPath + "/" + escapeID(emailID) + "/attachment/" + escapeID(blobID),
		token:  token,
	})
}
