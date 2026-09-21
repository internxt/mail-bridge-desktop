package imapserver

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"mail-bridge-desktop/internal/localtls"
	"mail-bridge-desktop/internal/store"
)

func TestStartServesDevelopmentMailbox(t *testing.T) {
	t.Parallel()

	server, err := Start(context.Background(), UnlockedSession{
		AccountID: "account-1",
		Addresses: []string{"user@example.test"},
	}, Config{
		ListenAddress:    "127.0.0.1:0",
		DataDir:          t.TempDir(),
		LocalCredentials: Credentials{Password: "local-password"},
		ConnectorFactory: NewDevelopmentConnectorFactory([][]byte{
			[]byte("From: sender@example.test\r\nTo: user@example.test\r\nSubject: fixture\r\n\r\nHello from Gluon.\r\n"),
		}),
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	status := server.Status()
	if status.Credentials.Password != "local-password" {
		t.Fatalf("Status() password = %q, want the one it was started with", status.Credentials.Password)
	}
	if status.StartTLS {
		t.Fatal("STARTTLS is unexpectedly enabled without a TLS configuration")
	}

	conn, err := net.DialTimeout("tcp", status.Address, time.Second)
	if err != nil {
		t.Fatalf("dial IMAP server: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	if line := readLine(t, reader); !strings.Contains(line, "OK") {
		t.Fatalf("greeting = %q, want IMAP OK greeting", line)
	}

	writeCommand(t, conn, "a1 LOGIN %s %s\r\n", status.Credentials.Username, status.Credentials.Password)
	if response := readThroughTag(t, reader, "a1"); !strings.Contains(response, "a1 OK") {
		t.Fatalf("LOGIN response = %q", response)
	}

	writeCommand(t, conn, "a2 LIST \"\" \"*\"\r\n")
	if response := readThroughTag(t, reader, "a2"); !strings.Contains(response, "INBOX") || !strings.Contains(response, "a2 OK") {
		t.Fatalf("LIST response = %q", response)
	}

	writeCommand(t, conn, "a3 SELECT INBOX\r\n")
	if response := readThroughTag(t, reader, "a3"); !strings.Contains(response, "1 EXISTS") || !strings.Contains(response, "a3 OK") {
		t.Fatalf("SELECT response = %q", response)
	}

	writeCommand(t, conn, "a4 FETCH 1 (BODY.PEEK[])\r\n")
	if response := readThroughTag(t, reader, "a4"); !strings.Contains(response, "Subject: fixture") || !strings.Contains(response, "a4 OK") {
		t.Fatalf("FETCH response = %q", response)
	}
}

// TestStartOffersSTARTTLS is what makes the bridge usable from Apple Mail, which
// will not send a password over a connection it cannot encrypt. The capability
// has to be advertised before login, and the upgrade has to survive a real
// handshake — a certificate the client rejects looks exactly like a server that
// is not there.
func TestStartOffersSTARTTLS(t *testing.T) {
	t.Parallel()

	credentials, err := store.NewForTesting(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	tlsConfig, _, err := localtls.Ensure(credentials)
	if err != nil {
		t.Fatalf("localtls.Ensure: %v", err)
	}

	server, err := Start(context.Background(), UnlockedSession{
		AccountID: "account-1",
		Addresses: []string{"user@example.test"},
	}, Config{
		ListenAddress:    "127.0.0.1:0",
		DataDir:          t.TempDir(),
		TLSConfig:        tlsConfig,
		LocalCredentials: Credentials{Password: "local-password"},
		ConnectorFactory: NewDevelopmentConnectorFactory(nil),
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	status := server.Status()
	if !status.StartTLS {
		t.Fatal("Status() reports no STARTTLS; the parent would tell the user there is none")
	}

	conn, err := net.DialTimeout("tcp", status.Address, time.Second)
	if err != nil {
		t.Fatalf("dial IMAP server: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	readLine(t, reader)

	writeCommand(t, conn, "a1 CAPABILITY\r\n")
	if response := readThroughTag(t, reader, "a1"); !strings.Contains(response, "STARTTLS") {
		t.Fatalf("CAPABILITY = %q, want it to advertise STARTTLS", response)
	}

	writeCommand(t, conn, "a2 STARTTLS\r\n")
	if response := readThroughTag(t, reader, "a2"); !strings.Contains(response, "a2 OK") {
		t.Fatalf("STARTTLS response = %q", response)
	}

	// The certificate is the bridge's own, so the client is told to expect it
	// rather than to skip verification: that is the check being made here.
	roots := x509.NewCertPool()
	roots.AddCert(leaf(t, tlsConfig))

	secure := tls.Client(conn, &tls.Config{ServerName: "localhost", RootCAs: roots})
	if err := secure.Handshake(); err != nil {
		t.Fatalf("TLS handshake: %v", err)
	}

	secureReader := bufio.NewReader(secure)
	writeCommand(t, secure, "a3 LOGIN %s %s\r\n", status.Credentials.Username, status.Credentials.Password)
	if response := readThroughTag(t, secureReader, "a3"); !strings.Contains(response, "a3 OK") {
		t.Fatalf("LOGIN over TLS = %q", response)
	}
}

func leaf(t *testing.T, config *tls.Config) *x509.Certificate {
	t.Helper()

	certificate, err := x509.ParseCertificate(config.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatalf("parse the server certificate: %v", err)
	}
	return certificate
}

func TestStartRejectsNonLoopbackListener(t *testing.T) {
	t.Parallel()

	_, err := Start(context.Background(), UnlockedSession{
		AccountID: "account-1",
		Addresses: []string{"user@example.test"},
	}, Config{
		ListenAddress:    "0.0.0.0:1143",
		DataDir:          t.TempDir(),
		LocalCredentials: Credentials{Password: "local-password"},
		ConnectorFactory: NewDevelopmentConnectorFactory(nil),
	})
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("Start() error = %v, want loopback validation error", err)
	}
}

func TestStartRequiresPassword(t *testing.T) {
	t.Parallel()

	_, err := Start(context.Background(), UnlockedSession{
		AccountID: "account-1",
		Addresses: []string{"user@example.test"},
	}, Config{
		ListenAddress:    "127.0.0.1:0",
		DataDir:          t.TempDir(),
		ConnectorFactory: NewDevelopmentConnectorFactory(nil),
	})
	if err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("Start() error = %v, want a missing password error", err)
	}
}

func writeCommand(t *testing.T, conn net.Conn, format string, values ...any) {
	t.Helper()
	if _, err := fmt.Fprintf(conn, format, values...); err != nil {
		t.Fatalf("write IMAP command: %v", err)
	}
}

func readLine(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read IMAP response: %v", err)
	}
	return line
}

func readThroughTag(t *testing.T, reader *bufio.Reader, tag string) string {
	t.Helper()
	var response strings.Builder
	for {
		line := readLine(t, reader)
		response.WriteString(line)
		if strings.HasPrefix(line, tag+" ") {
			return response.String()
		}
	}
}
