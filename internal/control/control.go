// Package control implements the parent-owned, persistent bridge control channel.
// The bridge is always the client and the parent creates the private endpoint
// and sends StartSession before any mail service starts.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const startupTimeout = 30 * time.Second

// Connect dials the private endpoint created by the parent. It does not send
// a greeting: the parent sends StartSession immediately after accepting the
// bridge connection.
func Connect(ctx context.Context, endpoint string) (*Client, error) {
	startupCtx, cancel := withStartupTimeout(ctx)
	defer cancel()

	connection, err := dial(startupCtx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("dial control endpoint: %w", err)
	}
	return &Client{connection: connection}, nil
}

// ReceiveStartSession receives and validates the first control message. It is
// the only message accepted before the daemon starts its services.
func (client *Client) ReceiveStartSession(ctx context.Context) (Session, error) {
	startupCtx, cancel := withStartupTimeout(ctx)
	defer cancel()

	message, err := ReadMessage(startupCtx, client.connection)
	if err != nil {
		return Session{}, fmt.Errorf("read start session: %w", err)
	}
	if message.Type != startSessionType || message.Session == nil {
		return Session{}, errors.New("expected start_session control message")
	}
	if err := validateSession(*message.Session); err != nil {
		return Session{}, err
	}
	return *message.Session, nil
}

// SendReady reports successful listener startup while keeping the control
// connection open for later parent messages.
func (client *Client) SendReady(ready Ready) error {
	if ready.IMAPAddress == "" || ready.SMTPAddress == "" {
		return errors.New("ready requires IMAP and SMTP addresses")
	}
	return client.send(context.Background(), Message{Type: readyType, Ready: &ready})
}

// SendError reports a stable non-secret error code to the parent. Detailed
// errors stay local so control messages never accidentally expose secrets.
func (client *Client) SendError(requestID, code string) error {
	if code == "" {
		return errors.New("control error requires a code")
	}
	return client.send(context.Background(), Message{
		Type:      errorType,
		RequestID: requestID,
		Error:     &ControlError{Code: code},
	})
}

// Close closes the control channel. Task 3 will also use channel closure to
// cancel the daemon and clear its in-memory session material.
func (client *Client) Close() error { return client.connection.Close() }

func (client *Client) send(ctx context.Context, message Message) error {
	client.writes.Lock()
	defer client.writes.Unlock()
	return WriteMessage(ctx, client.connection, message)
}

func validateSession(session Session) error {
	if session.AccountID == "" {
		return errors.New("start session requires account_id")
	}
	if len(session.Addresses) == 0 || session.Addresses[0] == "" {
		return errors.New("start session requires an address")
	}
	if len(session.BackendSession) == 0 || !json.Valid(session.BackendSession) {
		return errors.New("start session requires valid backend_session")
	}
	if session.MailClient.Username == "" || session.MailClient.Password == "" {
		return errors.New("start session requires local mail credentials")
	}
	return nil
}

func withStartupTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, startupTimeout)
}
