package control

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

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

// BackendSession is what the bridge needs in order to act for the account:
// the Mail API token, and the keys that open encrypted mail.
//
// It travels inside Session.BackendSession as opaque JSON, so this shape is
// agreed between the parent and the backend adapter rather than by the framing
// in this package. Nothing here is persisted: it lives as long as the session.
type BackendSession struct {
	Token                string `json:"token"`
	EncryptionPrivateKey string `json:"encryption_private_key,omitempty"`
	EncryptionPublicKey  string `json:"encryption_public_key,omitempty"`
}

// Ready reports the actual local listener addresses after the bridge binds
// both services.
type Ready struct {
	IMAPAddress string `json:"imap_address"`
	SMTPAddress string `json:"smtp_address"`
	StartTLS    bool   `json:"starttls"`

	// Certificate is the servers' own, base64 DER, for the parent to ask the
	// system to trust. Empty when the bridge is serving without TLS.
	Certificate string `json:"certificate,omitempty"`
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

// SyncStarted opens a sync that has work to do, naming the total up front so
// the parent can draw an empty bar before the first message is downloaded.
type SyncStarted struct {
	Total int `json:"total"`
}

// SyncProgress reports how far a sync has got through downloading the messages
// it found new. It is sent only while there is mail to download, so a cycle
// that finds nothing sends nothing.
//
// Percent is computed here rather than left to the parent, so every consumer
// shows the same number.
type SyncProgress struct {
	Downloaded int `json:"downloaded"`
	Total      int `json:"total"`
	Percent    int `json:"percent"`
}

// SyncFinished closes a sync the parent was watching. It follows the last
// SyncProgress whether the sync completed or gave up, so a bar never sits
// part-filled waiting for an update that is not coming.
type SyncFinished struct {
	Downloaded int    `json:"downloaded"`
	Total      int    `json:"total"`
	Code       string `json:"code,omitempty"`
}

type Message struct {
	Type      string         `json:"type"`
	RequestID string         `json:"request_id,omitempty"`
	Session   *Session       `json:"session,omitempty"`
	Ready     *Ready         `json:"ready,omitempty"`
	Update    *SessionUpdate `json:"update,omitempty"`
	Error     *ControlError  `json:"error,omitempty"`
	Started   *SyncStarted   `json:"started,omitempty"`
	Progress  *SyncProgress  `json:"progress,omitempty"`
	Finished  *SyncFinished  `json:"finished,omitempty"`
}

type deadlineSetter interface {
	SetDeadline(time.Time) error
}
