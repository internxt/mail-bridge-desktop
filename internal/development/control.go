// Package development stands in for Drive Desktop while the desktop client
// cannot yet launch the bridge.
//
// It speaks the same control protocol the real parent does, so the daemon
// connects, receives a session and starts exactly as it would in production.
// Keeping the pretence at this level is what keeps development out of the
// daemon: nothing in internal/daemon knows whether its parent is Drive Desktop
// or this package.
//
// Nothing outside cmd/devcontrol should import it.
package development

import (
	"context"
	"errors"
	"fmt"
	"net"

	"mail-bridge-desktop/internal/control"
)

// Control message types. The constants in internal/control are unexported, so
// the strings are repeated here; they are part of the wire protocol and change
// only when it does.
const (
	startSessionType = "start_session"
	readyType        = "ready"
	errorType        = "error"
)

// Serve waits for one bridge to connect, hands it the session, and returns the
// listener addresses the bridge reports back, along with the live connection.
//
// The connection stays open on purpose: the control channel is what tells the
// bridge its parent is still alive, and closing it here would shut the bridge
// down the moment it finished starting. The caller closes it when it is done.
//
// It serves a single connection because that is what the real parent does: one
// desktop client supervising one bridge.
func Serve(ctx context.Context, listener net.Listener, session control.Session) (control.Ready, net.Conn, error) {
	connection, err := listener.Accept()
	if err != nil {
		return control.Ready{}, nil, fmt.Errorf("accept bridge connection: %w", err)
	}

	ready, err := handshake(ctx, connection, session)
	if err != nil {
		_ = connection.Close()
		return control.Ready{}, nil, err
	}

	return ready, connection, nil
}

// handshake sends the session and waits for the bridge to report its listeners.
func handshake(ctx context.Context, connection net.Conn, session control.Session) (control.Ready, error) {
	if err := control.WriteMessage(ctx, connection, control.Message{
		Type:    startSessionType,
		Session: &session,
	}); err != nil {
		return control.Ready{}, fmt.Errorf("send session: %w", err)
	}

	return receiveReady(ctx, connection)
}

// receiveReady waits for the bridge to report its listeners, or to say why it
// could not start.
func receiveReady(ctx context.Context, connection net.Conn) (control.Ready, error) {
	message, err := control.ReadMessage(ctx, connection)
	if err != nil {
		return control.Ready{}, fmt.Errorf("read bridge reply: %w", err)
	}

	switch {
	case message.Type == errorType && message.Error != nil:
		// The bridge only sends a code: details stay on its side so control
		// messages never carry secrets.
		return control.Ready{}, fmt.Errorf("bridge failed to start: %s", message.Error.Code)
	case message.Type != readyType || message.Ready == nil:
		return control.Ready{}, errors.New("expected a ready message from the bridge")
	}

	return *message.Ready, nil
}
