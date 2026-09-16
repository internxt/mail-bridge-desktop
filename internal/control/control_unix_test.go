//go:build darwin || linux

package control

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// socketPath returns a short path for a Unix socket.
//
// t.TempDir() is not usable here: on macOS it sits under /var/folders/... and
// the resulting path exceeds the ~104 byte limit the kernel imposes on socket
// addresses, which surfaces as a puzzling "bind: invalid argument".
func socketPath(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "ctl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return filepath.Join(dir, "c.sock")
}

func TestConnectReceivesStartSessionAndSendsReady(t *testing.T) {
	path := socketPath(t)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	ready := make(chan Message, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		if err := WriteMessage(context.Background(), connection, Message{
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
		response, err := ReadMessage(context.Background(), connection)
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

// TestSendSyncProgressReachesTheParent is the round trip over a real socket.
// It is what catches the Progress field being missing from Message: decoding
// rejects unknown fields, so the parent would fail to read what we sent.
func TestSendSyncProgressReachesTheParent(t *testing.T) {
	path := socketPath(t)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	received := make(chan Message, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()

		message, err := ReadMessage(context.Background(), connection)
		if err == nil {
			received <- message
		}
	}()

	client, err := Connect(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.SendSyncProgress(SyncProgress{Downloaded: 37, Total: 142, Percent: 26}); err != nil {
		t.Fatal(err)
	}

	message := <-received
	if message.Type != syncProgressType {
		t.Fatalf("type = %q, want %q", message.Type, syncProgressType)
	}
	if message.Progress == nil {
		t.Fatal("the message carried no progress")
	}
	if want := (SyncProgress{Downloaded: 37, Total: 142, Percent: 26}); *message.Progress != want {
		t.Errorf("got %+v, want %+v", *message.Progress, want)
	}
}

func TestSendSyncProgressRejectsNoTotal(t *testing.T) {
	client := &Client{}

	if err := client.SendSyncProgress(SyncProgress{Downloaded: 1}); err == nil {
		t.Fatal("SendSyncProgress: expected an error, got nil")
	}
	if err := client.SendSyncStarted(SyncStarted{}); err == nil {
		t.Fatal("SendSyncStarted: expected an error, got nil")
	}
}

// TestSyncMessagesReachTheParentInOrder covers the whole cycle a parent draws
// a bar from: the total up front, then progress, then the close that says it
// is over. It is also what catches a payload field missing from Message,
// since decoding rejects unknown fields.
func TestSyncMessagesReachTheParentInOrder(t *testing.T) {
	path := socketPath(t)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	received := make(chan []Message, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()

		var messages []Message
		for i := 0; i < 3; i++ {
			message, err := ReadMessage(context.Background(), connection)
			if err != nil {
				return
			}
			messages = append(messages, message)
		}
		received <- messages
	}()

	client, err := Connect(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.SendSyncStarted(SyncStarted{Total: 142}); err != nil {
		t.Fatal(err)
	}
	if err := client.SendSyncProgress(SyncProgress{Downloaded: 37, Total: 142, Percent: 26}); err != nil {
		t.Fatal(err)
	}
	if err := client.SendSyncFinished(SyncFinished{Downloaded: 142, Total: 142}); err != nil {
		t.Fatal(err)
	}

	messages := <-received

	if messages[0].Type != syncStartedType || messages[0].Started == nil {
		t.Fatalf("first message = %+v, want a %s", messages[0], syncStartedType)
	}
	if messages[0].Started.Total != 142 {
		t.Errorf("started with a total of %d, want 142", messages[0].Started.Total)
	}

	if messages[1].Type != syncProgressType || messages[1].Progress == nil {
		t.Fatalf("second message = %+v, want a %s", messages[1], syncProgressType)
	}

	if messages[2].Type != syncFinishedType || messages[2].Finished == nil {
		t.Fatalf("third message = %+v, want a %s", messages[2], syncFinishedType)
	}
	if want := (SyncFinished{Downloaded: 142, Total: 142}); *messages[2].Finished != want {
		t.Errorf("got %+v, want %+v", *messages[2].Finished, want)
	}
}

// TestSendSyncFinishedCarriesAFailure is the case the parent needs in order to
// stop a bar that will not fill: the sync closed short of its total, and said
// why.
func TestSendSyncFinishedCarriesAFailure(t *testing.T) {
	path := socketPath(t)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	received := make(chan Message, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()

		if message, err := ReadMessage(context.Background(), connection); err == nil {
			received <- message
		}
	}()

	client, err := Connect(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.SendSyncFinished(SyncFinished{Downloaded: 40, Total: 120, Code: "fetch_bodies"}); err != nil {
		t.Fatal(err)
	}

	message := <-received
	if message.Finished == nil {
		t.Fatal("the message carried no finish")
	}
	if want := (SyncFinished{Downloaded: 40, Total: 120, Code: "fetch_bodies"}); *message.Finished != want {
		t.Errorf("got %+v, want %+v", *message.Finished, want)
	}
}
