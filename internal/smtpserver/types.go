package smtpserver

import (
	"context"

	"mail-bridge-desktop/internal/logger"

	"github.com/emersion/go-smtp"
)

type Sender interface {
	SendEmail(ctx context.Context, raw []byte, envelopeRecipients []string) error
}

type backend struct {
	log         *logger.Logger
	credentials Credentials
	sender      Sender
}

// Credentials are the local mail-client credentials issued by Drive Desktop.
// They are distinct from the user's authenticated Drive session.
type Credentials struct {
	Username string
	Password string
}

// session implements smtp.Session and smtp.AuthSession.
type session struct {
	log         *logger.Logger
	credentials Credentials
	sender      Sender
	from        string
	to          []string
}

type Service struct {
	srv *smtp.Server
	log *logger.Logger
}

// smtpLogger adapts our logger to the interface go-smtp expects.
type smtpLogger struct{ log *logger.Logger }

// debugWriter dumps the SMTP dialogue to the logger while debugging.
type debugWriter struct{ log *logger.Logger }
