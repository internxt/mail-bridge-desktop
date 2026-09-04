// Package smtpserver runs a local SMTP listener for desktop mail clients to
// submit outgoing mail to. It authenticates the client, then hands each
// message to a Sender for delivery.
package smtpserver

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"mail-bridge-desktop/internal/config"
	"mail-bridge-desktop/internal/logger"
)

const (
	maxMessageBytes = 50 * 1024 * 1024
	maxRecipients   = 100
	ioTimeout       = 60 * time.Second
)

// New builds the SMTP service. The credentials are the local ones the parent
// issued for this account, the same pair the IMAP side accepts. sender is
// nil-able: a nil sender still authenticates a client, but Data reports an
// error instead of submitting anything, so a client sees why nothing sent
// rather than a message that silently vanishes.
func New(cfg config.Config, credentials Credentials, sender Sender) *Service {
	log := logger.New("smtp")

	srv := smtp.NewServer(&backend{log: log, credentials: credentials, sender: sender})
	srv.Addr = cfg.SMTPAddr
	srv.Domain = cfg.SMTPDomain
	// Cleartext credentials: only acceptable because we listen on loopback.
	// Must go away as soon as TLS is enabled.
	srv.AllowInsecureAuth = true
	srv.ReadTimeout = ioTimeout
	srv.WriteTimeout = ioTimeout
	srv.MaxMessageBytes = maxMessageBytes
	srv.MaxRecipients = maxRecipients
	srv.ErrorLog = smtpLogger{log}
	srv.Debug = debugWriter{log}

	return &Service{srv: srv, log: log}
}

func (b *backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	b.log.Info("connection from %v", c.Conn().RemoteAddr())
	return &session{log: b.log, credentials: b.credentials, sender: b.sender}, nil
}

func (s *session) AuthMechanisms() []string { return []string{sasl.Plain} }

// Commands form the SMTP server that are not implemented by the API/Crypto.
func (s *session) Auth(mech string) (sasl.Server, error) {
	if mech != sasl.Plain {
		return nil, smtp.ErrAuthUnsupported
	}
	// An empty credential set preserves development compatibility for callers
	// that have not yet adopted the authenticated startup flow.
	return sasl.NewPlainServer(func(identity, username, password string) error {
		if s.credentials.Password == "" {
			s.log.Info("auth plain from %s (development mode)", username)
			return nil
		}
		if subtle.ConstantTimeCompare([]byte(username), []byte(s.credentials.Username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(password), []byte(s.credentials.Password)) != 1 {
			return errors.New("invalid local bridge credentials")
		}
		s.log.Info("auth plain from %s", username)
		return nil
	}), nil
}

func (s *session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	s.log.Info("mail from %s", from)
	return nil
}

func (s *session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	s.log.Info("rcpt to %s", to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	if s.sender == nil {
		s.log.Warn("data from %s to %v: %d bytes, dropped (no mail service available)", s.from, s.to, len(raw))
		return errors.New("mail service unavailable")
	}

	if err := s.sender.SendEmail(context.Background(), raw, s.to); err != nil {
		s.log.Warn("send from %s to %v failed: %v", s.from, s.to, err)
		return fmt.Errorf("send: %w", err)
	}

	s.log.Info("sent %d bytes from %s to %v", len(raw), s.from, s.to)
	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.to = nil
}

func (s *session) Logout() error { return nil }

// Actions to Start/Stop de SMTP server.

// Start brings up the server and returns; the server keeps running in the
// background until Stop is called.
func (s *Service) Start() error {
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return err
	}
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
			s.log.Error("serve: %v", err)
		}
	}()
	s.log.Info("listening on %s", s.srv.Addr)
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	err := s.srv.Shutdown(ctx)
	if err != nil && !errors.Is(err, smtp.ErrServerClosed) {
		return err
	}
	s.log.Info("stopped")
	return nil
}

func (l smtpLogger) Printf(format string, v ...any) { l.log.Error(format, v...) }
func (l smtpLogger) Println(v ...any)               { l.log.Error("%s", fmt.Sprintln(v...)) }

func (w debugWriter) Write(p []byte) (int, error) {
	w.log.Info("%s", bytes.TrimRight(p, "\r\n"))
	return len(p), nil
}
