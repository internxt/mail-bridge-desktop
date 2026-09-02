package imapserver

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
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
