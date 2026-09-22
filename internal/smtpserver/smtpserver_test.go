package smtpserver

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"mail-bridge-desktop/internal/config"
	"mail-bridge-desktop/internal/localtls"
	"mail-bridge-desktop/internal/logger"
	"mail-bridge-desktop/internal/store"
)

type fakeSender struct {
	err    error
	raw    []byte
	to     []string
	called bool
}

func (f *fakeSender) SendEmail(ctx context.Context, raw []byte, envelopeRecipients []string) error {
	f.called = true
	f.raw = raw
	f.to = envelopeRecipients
	return f.err
}

func testSession(sender Sender) *session {
	return &session{log: logger.New("test"), sender: sender, to: []string{"bob@inxt.eu"}}
}

func TestDataSendsToTheSender(t *testing.T) {
	sender := &fakeSender{}
	s := testSession(sender)

	body := "Subject: hola\r\n\r\ncuerpo\r\n"
	if err := s.Data(strings.NewReader(body)); err != nil {
		t.Fatalf("Data: %v", err)
	}

	if !sender.called {
		t.Fatal("the sender was never called")
	}
	if string(sender.raw) != body {
		t.Errorf("raw = %q, want %q", sender.raw, body)
	}
	if len(sender.to) != 1 || sender.to[0] != "bob@inxt.eu" {
		t.Errorf("to = %v, want [bob@inxt.eu]", sender.to)
	}
}

func TestDataPropagatesSenderFailure(t *testing.T) {
	sender := &fakeSender{err: errors.New("api is down")}
	s := testSession(sender)

	if err := s.Data(strings.NewReader("data")); err == nil {
		t.Fatal("expected the sender's failure to propagate")
	}
}

func TestDataWithoutASenderFailsRatherThanDropSilently(t *testing.T) {
	s := testSession(nil)

	if err := s.Data(strings.NewReader("data")); err == nil {
		t.Fatal("expected an error when no mail service is available")
	}
}

// TestStartOffersSTARTTLSAndWithholdsAuth is the sending half of what Apple Mail
// needs. The two assertions are one requirement: a client that is told it may
// authenticate in the clear never upgrades, and Mail would rather refuse to send
// than take that offer.
func TestStartOffersSTARTTLSAndWithholdsAuth(t *testing.T) {
	credentials, err := store.NewForTesting(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	tlsConfig, _, err := localtls.Ensure(credentials)
	if err != nil {
		t.Fatalf("localtls.Ensure: %v", err)
	}

	service := New(
		config.Config{SMTPAddr: "127.0.0.1:0", SMTPDomain: "localhost"},
		Credentials{Username: "user@example.test", Password: "local-password"},
		&fakeSender{},
		tlsConfig,
	)
	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Stop(context.Background()) })

	conn, err := net.DialTimeout("tcp", service.Address(), time.Second)
	if err != nil {
		t.Fatalf("dial SMTP server: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	readSMTPLine(t, reader)

	greeting := ehlo(t, conn, reader)
	if !strings.Contains(greeting, "STARTTLS") {
		t.Errorf("EHLO = %q, want it to advertise STARTTLS", greeting)
	}
	if strings.Contains(greeting, "AUTH") {
		t.Errorf("EHLO = %q, want no AUTH before the connection is encrypted", greeting)
	}

	if _, err := fmt.Fprint(conn, "STARTTLS\r\n"); err != nil {
		t.Fatalf("write STARTTLS: %v", err)
	}
	if line := readSMTPLine(t, reader); !strings.HasPrefix(line, "220") {
		t.Fatalf("STARTTLS response = %q", line)
	}

	roots := x509.NewCertPool()
	certificate, err := x509.ParseCertificate(tlsConfig.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatalf("parse the server certificate: %v", err)
	}
	roots.AddCert(certificate)

	secure := tls.Client(conn, &tls.Config{ServerName: "localhost", RootCAs: roots})
	if err := secure.Handshake(); err != nil {
		t.Fatalf("TLS handshake: %v", err)
	}

	secureReader := bufio.NewReader(secure)
	if upgraded := ehlo(t, secure, secureReader); !strings.Contains(upgraded, "AUTH") {
		t.Errorf("EHLO over TLS = %q, want AUTH to be offered now", upgraded)
	}
}

func ehlo(t *testing.T, conn net.Conn, reader *bufio.Reader) string {
	t.Helper()

	if _, err := fmt.Fprint(conn, "EHLO localhost\r\n"); err != nil {
		t.Fatalf("write EHLO: %v", err)
	}

	var greeting strings.Builder
	for {
		line := readSMTPLine(t, reader)
		greeting.WriteString(line)

		// A hyphen after the code means another line follows; a space ends it.
		if len(line) < 4 || line[3] != '-' {
			return greeting.String()
		}
	}
}

func readSMTPLine(t *testing.T, reader *bufio.Reader) string {
	t.Helper()

	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read SMTP line: %v", err)
	}
	return line
}

// TestStartAcceptsImplicitTLS covers the way Apple Mail opens first: straight into a
// handshake, with no greeting read and no STARTTLS asked for. A port that refuses it
// leaves Mail waiting out the read timeout before it retries, which the user sees as a
// minute of nothing while their account is being set up.
func TestStartAcceptsImplicitTLS(t *testing.T) {
	credentials, err := store.NewForTesting(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	tlsConfig, _, err := localtls.Ensure(credentials)
	if err != nil {
		t.Fatalf("localtls.Ensure: %v", err)
	}

	service := New(
		config.Config{SMTPAddr: "127.0.0.1:0", SMTPDomain: "localhost"},
		Credentials{Username: "user@example.test", Password: "local-password"},
		&fakeSender{},
		tlsConfig,
	)
	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Stop(context.Background()) })

	roots := x509.NewCertPool()
	certificate, err := x509.ParseCertificate(tlsConfig.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatalf("parse the server certificate: %v", err)
	}
	roots.AddCert(certificate)

	conn, err := tls.Dial("tcp", service.Address(), &tls.Config{ServerName: "localhost", RootCAs: roots})
	if err != nil {
		t.Fatalf("open the connection with a handshake: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	if greeting := readSMTPLine(t, reader); !strings.HasPrefix(greeting, "220") {
		t.Fatalf("greeting = %q, want the server to speak SMTP once the handshake is done", greeting)
	}

	// Already encrypted, so there is nothing left to upgrade and AUTH is on offer
	// from the first EHLO.
	capabilities := ehlo(t, conn, reader)
	if !strings.Contains(capabilities, "AUTH") {
		t.Errorf("EHLO = %q, want AUTH to be offered on an encrypted connection", capabilities)
	}
	if strings.Contains(capabilities, "STARTTLS") {
		t.Errorf("EHLO = %q, want no STARTTLS to be offered on a connection that already has TLS", capabilities)
	}
}

// TestStartStillGreetsAClientThatWaits guards the other half: the client that says
// nothing until it is greeted must not be mistaken for one that failed to open.
func TestStartStillGreetsAClientThatWaits(t *testing.T) {
	credentials, err := store.NewForTesting(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	tlsConfig, _, err := localtls.Ensure(credentials)
	if err != nil {
		t.Fatalf("localtls.Ensure: %v", err)
	}

	service := New(
		config.Config{SMTPAddr: "127.0.0.1:0", SMTPDomain: "localhost"},
		Credentials{Username: "user@example.test", Password: "local-password"},
		&fakeSender{},
		tlsConfig,
	)
	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Stop(context.Background()) })

	conn, err := net.DialTimeout("tcp", service.Address(), time.Second)
	if err != nil {
		t.Fatalf("dial SMTP server: %v", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	reader := bufio.NewReader(conn)
	if greeting := readSMTPLine(t, reader); !strings.HasPrefix(greeting, "220") {
		t.Fatalf("greeting = %q, want the server to greet a client that waits", greeting)
	}
	if capabilities := ehlo(t, conn, reader); !strings.Contains(capabilities, "STARTTLS") {
		t.Errorf("EHLO = %q, want STARTTLS still offered in the clear", capabilities)
	}
}

// TestStartWithoutACertificateStillServes pins the fallback. The bridge asks for a
// certificate on every start and may not get one — a locked keychain is enough — and
// when that happens it still has to carry mail. Withdrawing AUTH as well would turn a
// degraded bridge into a useless one.
func TestStartWithoutACertificateStillServes(t *testing.T) {
	service := New(
		config.Config{SMTPAddr: "127.0.0.1:0", SMTPDomain: "localhost"},
		Credentials{Username: "user@example.test", Password: "local-password"},
		&fakeSender{},
		nil,
	)
	if err := service.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Stop(context.Background()) })

	conn, err := net.DialTimeout("tcp", service.Address(), time.Second)
	if err != nil {
		t.Fatalf("dial SMTP server: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	readSMTPLine(t, reader)

	capabilities := ehlo(t, conn, reader)
	if !strings.Contains(capabilities, "AUTH") {
		t.Errorf("EHLO = %q, want AUTH still offered when there is no TLS to require", capabilities)
	}
	if strings.Contains(capabilities, "STARTTLS") {
		t.Errorf("EHLO = %q, want no STARTTLS offered without a certificate", capabilities)
	}
}
