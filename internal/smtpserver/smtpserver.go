// Package smtpserver runs a local SMTP listener for desktop mail clients to
// submit outgoing mail to. It authenticates the client, then hands each
// message to a Sender for delivery.
package smtpserver

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
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
//
// tlsConfig is nil-able too, and what it decides is whether a client is allowed
// to authenticate in the clear.
func New(cfg config.Config, credentials Credentials, sender Sender, tlsConfig *tls.Config) *Service {
	log := logger.New("smtp")

	srv := smtp.NewServer(&backend{log: log, credentials: credentials, sender: sender})
	srv.Addr = cfg.SMTPAddr
	srv.Domain = cfg.SMTPDomain
	srv.TLSConfig = tlsConfig
	srv.AllowInsecureAuth = tlsConfig == nil
	srv.ReadTimeout = ioTimeout
	srv.WriteTimeout = ioTimeout
	srv.MaxMessageBytes = maxMessageBytes
	srv.MaxRecipients = maxRecipients
	srv.ErrorLog = smtpLogger{log}

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
			s.log.Info("authenticated (development mode)")
			return nil
		}
		if subtle.ConstantTimeCompare([]byte(username), []byte(s.credentials.Username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(password), []byte(s.credentials.Password)) != 1 {
			s.log.Warn("rejected a client offering the wrong bridge credentials")
			return errors.New("invalid local bridge credentials")
		}
		s.log.Info("authenticated")
		return nil
	}), nil
}

// The envelope is recorded but not logged: who the user writes to is theirs, and a
// count is enough to follow what a session did.

func (s *session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	s.log.Info("accepted an envelope sender")
	return nil
}

func (s *session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	s.log.Info("accepted recipient %d", len(s.to))
	return nil
}

func (s *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	if s.sender == nil {
		s.log.Warn("dropped a message of %d bytes for %d recipients: no mail service available", len(raw), len(s.to))
		return errors.New("mail service unavailable")
	}

	if err := s.sender.SendEmail(context.Background(), raw, s.to); err != nil {
		s.log.Warn("could not send a message of %d bytes to %d recipients: %v", len(raw), len(s.to), err)
		return fmt.Errorf("send: %w", err)
	}

	s.log.Info("sent a message of %d bytes to %d recipients", len(raw), len(s.to))
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
	s.listener = ln
	go func() {
		if err := s.srv.Serve(acceptEither(ln, s.srv.TLSConfig)); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
			s.log.Error("serve: %v", err)
		}
	}()
	s.log.Info("listening on %s", s.srv.Addr)
	return nil
}

// Address is where the service actually bound, which is only the configured
// address until a port of 0 asks the system to choose one.
func (s *Service) Address() string {
	if s.listener == nil {
		return s.srv.Addr
	}
	return s.listener.Addr().String()
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
