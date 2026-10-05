//go:build windows

package control

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

type observedReadConnection struct {
	io.ReadWriteCloser
	reading chan struct{}
}

func (c *observedReadConnection) Read(data []byte) (int, error) {
	select {
	case <-c.reading:
	default:
		close(c.reading)
	}
	return c.ReadWriteCloser.Read(data)
}

func TestProgressIsSentWhileWaitingForParentCommand(t *testing.T) {
	endpoint := fmt.Sprintf(`\\.\pipe\mail-bridge-progress-test-%d`, time.Now().UnixNano())
	listener, err := winio.ListenPipe(endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	accepted := make(chan net.Conn, 1)
	acceptErrors := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			acceptErrors <- err
			return
		}
		accepted <- connection
	}()
	client, err := Connect(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	var parent net.Conn
	select {
	case parent = <-accepted:
	case err := <-acceptErrors:
		_ = client.Close()
		t.Fatal(err)
	}
	defer parent.Close()
	reading := make(chan struct{})
	client.connection = &observedReadConnection{ReadWriteCloser: client.connection, reading: reading}
	served := make(chan error, 1)
	go func() { served <- client.Serve(ctx, Events{}) }()
	defer func() {
		cancel()
		// Release a synchronous read in the broken transport so a failed test exits.
		_ = WriteMessage(context.Background(), parent, Message{Type: resyncType})
		_ = client.Close()
	}()
	<-reading
	// Give the command read time to enter the Windows kernel before sending progress.
	time.Sleep(100 * time.Millisecond)
	if err := parent.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	received := make(chan Message, 1)
	readErrors := make(chan error, 1)
	go func() {
		message, err := ReadMessage(context.Background(), parent)
		if err != nil {
			readErrors <- err
			return
		}
		received <- message
	}()
	sent := make(chan error, 1)
	go func() { sent <- client.SendSyncProgress(SyncProgress{Downloaded: 4, Total: 10, Percent: 40}) }()
	select {
	case err := <-sent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("progress is blocked until the parent sends a command")
	}
	var message Message
	select {
	case message = <-received:
	case err := <-readErrors:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("parent did not receive progress")
	}
	if message.Type != syncProgressType || message.Progress == nil || message.Progress.Percent != 40 {
		t.Fatalf("unexpected progress message: %+v", message)
	}
}
