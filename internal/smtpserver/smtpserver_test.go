package smtpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mail-bridge-desktop/internal/logger"
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
