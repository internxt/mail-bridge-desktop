package control

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

const startupTimeout = 30 * time.Second

// Session is supplied by the parent after it has authenticated and unlocked an
// account. BackendSession is opaque until the production backend adapter
// defines its typed schema.
type Session struct {
	AccountID      string          `json:"account_id"`
	Addresses      []string        `json:"addresses"`
	BackendSession json.RawMessage `json:"backend_session"`
	MailClient     MailClient      `json:"mail_client"`
}

// MailClient contains stable, generated credentials used only by local IMAP
// and SMTP clients. The parent owns their durable storage.
type MailClient struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Ready reports the actual local listener addresses after the bridge binds
// both services.
type Ready struct {
	IMAPAddress string `json:"imap_address"`
	SMTPAddress string `json:"smtp_address"`
	StartTLS    bool   `json:"starttls"`
}

// Client owns the bridge side of the persistent control connection.
type Client struct {
	connection io.ReadWriteCloser
	writes     sync.Mutex
}

// ControlError intentionally contains only a stable, non-secret error code.
type ControlError struct {
	Code string `json:"code"`
}

// SessionUpdate is reserved for task 4. It is defined here so the framed
// control schema is explicit even before the daemon consumes updates.
type SessionUpdate struct {
	BackendSession json.RawMessage `json:"backend_session"`
}

type message struct {
	Type      string         `json:"type"`
	RequestID string         `json:"request_id,omitempty"`
	Session   *Session       `json:"session,omitempty"`
	Ready     *Ready         `json:"ready,omitempty"`
	Update    *SessionUpdate `json:"update,omitempty"`
	Error     *ControlError  `json:"error,omitempty"`
}

type deadlineSetter interface {
	SetDeadline(time.Time) error
}
