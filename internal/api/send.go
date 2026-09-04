package api

import (
	"context"
	"net/http"
)

const (
	emailSendPath           = emailPath + "/send"
	emailKeysLookupPath     = emailPath + "/keys/lookup"
	userMailAccountKeysPath = "/users/me/mail-account/keys"
)

// LookupRecipientKeys returns each address's public encryption key, or a nil
// key for an address that does not have one — an external recipient, or one
// on a domain the Mail API does not manage.
func (c *Client) LookupRecipientKeys(ctx context.Context, token string, addresses []string) ([]RecipientKeyDto, error) {
	var res LookupRecipientKeysResponseDto

	err := c.do(ctx, request{
		svc:    c.mail,
		method: http.MethodPost,
		path:   emailKeysLookupPath,
		token:  token,
		body:   LookupRecipientKeysRequestDto{Addresses: addresses},
	}, &res)

	return res.Recipients, err
}

// SendEmail submits a composed email to be delivered.
func (c *Client) SendEmail(ctx context.Context, token string, email SendEmailRequestDto) (EmailCreatedResponseDto, error) {
	var res EmailCreatedResponseDto

	err := c.do(ctx, request{
		svc:    c.mail,
		method: http.MethodPost,
		path:   emailSendPath,
		token:  token,
		body:   email,
	}, &res)

	return res, err
}

// GetMailAccountKeys returns the caller's own encryption keys, including the
// public key a sender seals their own Sent copy with.
func (c *Client) GetMailAccountKeys(ctx context.Context, token string) (MailAccountKeysResponseDto, error) {
	var res MailAccountKeysResponseDto

	err := c.do(ctx, request{
		svc:    c.mail,
		method: http.MethodGet,
		path:   userMailAccountKeysPath,
		token:  token,
	}, &res)

	return res, err
}
