package api

import (
	"context"
	"net/http"
)

const emailDraftsPath = emailPath + "/drafts"

func (c *Client) SaveDraft(ctx context.Context, token string, draft DraftEmailRequestDto) (EmailResponseDto, error) {
	var res EmailResponseDto

	err := c.do(ctx, request{
		svc:    c.mail,
		method: http.MethodPost,
		path:   emailDraftsPath,
		token:  token,
		body:   draft,
	}, &res)

	return res, err
}

func (c *Client) UpdateDraft(ctx context.Context, token, draftID string, draft DraftEmailRequestDto) (EmailResponseDto, error) {
	var res EmailResponseDto

	err := c.do(ctx, request{
		svc:    c.mail,
		method: http.MethodPatch,
		path:   emailDraftsPath + "/" + escapeID(draftID),
		token:  token,
		body:   draft,
	}, &res)

	return res, err
}

func (c *Client) DiscardDraft(ctx context.Context, token, draftID string) error {
	return c.do(ctx, request{
		svc:    c.mail,
		method: http.MethodDelete,
		path:   emailDraftsPath + "/" + escapeID(draftID),
		token:  token,
	}, nil)
}
