package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

const (
	emailPath          string = "/email"
	emailMailboxesPath string = emailPath + "/mailboxes"
	emailThreadsPath   string = emailPath + "/threads/"
)

func (o ListEmailsOptions) query() url.Values {
	q := url.Values{}
	if o.Mailbox != "" {
		q.Set("mailbox", string(o.Mailbox))
	}

	// strconv.Itoa, not string(): converting an int with string() yields the
	// character with that code point, not its digits.
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}

	if o.Position > 0 {
		q.Set("position", strconv.Itoa(o.Position))
	}

	if o.AnchorID != "" {
		q.Set("anchorId", o.AnchorID)
	}

	if o.Unread != nil {
		q.Set("unread", strconv.FormatBool(*o.Unread))
	}

	return q
}

func (c *Client) GetUserFolder(ctx context.Context, token string, opts ListEmailsOptions) (EmailListResponseDto, error) {
	var res EmailListResponseDto

	err := c.do(ctx, request{
		svc:    c.mail,
		method: http.MethodGet,
		path:   emailPath,
		query:  opts.query(),
		token:  token,
	}, &res)

	return res, err
}

func (c *Client) GetMailboxes(ctx context.Context, token string) ([]MailboxResponseDto, error) {
	var res []MailboxResponseDto

	err := c.do(ctx, request{
		svc:    c.mail,
		method: http.MethodGet,
		path:   emailMailboxesPath,
		token:  token,
	}, &res)

	return res, err
}

func (c *Client) GetThread(ctx context.Context, token, threadID string) ([]EmailResponseDto, error) {
	var res []EmailResponseDto

	err := c.do(ctx, request{
		svc:    c.mail,
		method: http.MethodGet,
		path:   emailThreadsPath + url.PathEscape(threadID),
		token:  token,
	}, &res)

	return res, err
}
