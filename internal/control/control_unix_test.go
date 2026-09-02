//go:build darwin || linux

package control

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
)

func TestConnectReceivesStartSessionAndSendsReady(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	ready := make(chan message, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		if err := writeMessage(context.Background(), connection, message{
			Type: startSessionType,
			Session: &Session{
				AccountID:      "account-1",
				Addresses:      []string{"user@example.test"},
				BackendSession: json.RawMessage(`{"access_token":"redacted"}`),
				MailClient:     MailClient{Username: "user@example.test", Password: "local-password"},
			},
		}); err != nil {
			return
		}
		response, err := readMessage(context.Background(), connection)
		if err == nil {
			ready <- response
		}
	}()

	client, err := Connect(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	session, err := client.ReceiveStartSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if session.AccountID != "account-1" || session.MailClient.Password != "local-password" {
		t.Fatalf("unexpected session: %+v", session)
	}
	if err := client.SendReady(Ready{IMAPAddress: "127.0.0.1:1143", SMTPAddress: "127.0.0.1:2025"}); err != nil {
		t.Fatal(err)
	}

	response := <-ready
	if response.Type != readyType || response.Ready == nil || response.Ready.IMAPAddress != "127.0.0.1:1143" || response.Ready.SMTPAddress != "127.0.0.1:2025" {
		t.Fatalf("unexpected ready response: %+v", response)
	}
}
