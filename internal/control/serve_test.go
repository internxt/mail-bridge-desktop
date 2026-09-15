package control

import (
	"bytes"
	"context"
	"io"
	"testing"
)

// framed writes each message the way the parent would send it, so Serve reads
// them off one buffer as it would off the connection.
func framed(t *testing.T, messages ...Message) io.ReadWriteCloser {
	t.Helper()

	var buffer bytes.Buffer
	for _, message := range messages {
		if err := WriteMessage(context.Background(), &buffer, message); err != nil {
			t.Fatalf("WriteMessage: %v", err)
		}
	}
	return readWriteCloser{Reader: &buffer}
}

type readWriteCloser struct {
	io.Reader
}

func (readWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (readWriteCloser) Close() error                { return nil }

func TestServeCallsOnResyncForEveryResyncMessage(t *testing.T) {
	client := &Client{connection: framed(t,
		Message{Type: resyncType},
		Message{Type: resyncType},
	)}

	calls := 0
	err := client.Serve(context.Background(), Events{OnResync: func() { calls++ }})

	// Serve only returns when the connection ends, which here is EOF.
	if err == nil {
		t.Fatal("expected an error when the connection ends, got nil")
	}
	if calls != 2 {
		t.Errorf("OnResync ran %d times, want 2", calls)
	}
}

// TestServeIgnoresOtherMessages covers the default branch: a parent speaking a
// newer protocol must not stop this bridge from serving.
func TestServeIgnoresOtherMessages(t *testing.T) {
	client := &Client{connection: framed(t,
		Message{Type: sessionUpdateType},
		Message{Type: resyncType},
	)}

	calls := 0
	_ = client.Serve(context.Background(), Events{OnResync: func() { calls++ }})

	if calls != 1 {
		t.Errorf("OnResync ran %d times, want 1", calls)
	}
}

// TestServeWithoutHandlers checks a nil handler is ignored rather than a panic.
func TestServeWithoutHandlers(t *testing.T) {
	client := &Client{connection: framed(t, Message{Type: resyncType})}

	if err := client.Serve(context.Background(), Events{}); err == nil {
		t.Fatal("expected an error when the connection ends, got nil")
	}
}

func TestServeStopsWhenContextIsDone(t *testing.T) {
	client := &Client{connection: framed(t, Message{Type: resyncType})}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := client.Serve(ctx, Events{OnResync: func() {
		t.Error("OnResync ran after the context was cancelled")
	}}); err == nil {
		t.Fatal("expected an error, got nil")
	}
}
