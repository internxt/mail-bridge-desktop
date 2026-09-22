package control

import (
	"context"
	"errors"
	"fmt"
	"io"
)

type Events struct {
	OnResync        func()
	OnSessionUpdate func(BackendSession)
}

// isDisconnect reports whether the read failed because the parent went away rather than
// because it sent something the bridge could not read.
func isDisconnect(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// Serve reads control messages until the connection closes or ctx is done,
// dispatching each one to its handler.
func (client *Client) Serve(ctx context.Context, events Events) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		message, err := ReadMessage(ctx, client.connection)
		if err != nil {
			if !isDisconnect(err) && ctx.Err() == nil {
				_ = client.SendError("", malformedFrameCode)
			}
			return fmt.Errorf("read control message: %w", err)
		}

		switch message.Type {
		case resyncType:
			if events.OnResync != nil {
				events.OnResync()
			}

		case sessionUpdateType:
			if events.OnSessionUpdate == nil || message.Update == nil {
				continue
			}
			backend, err := message.Update.Backend()
			if err != nil {
				_ = client.SendError("", badSessionUpdateCode)
				return fmt.Errorf("decode session update: %w", err)
			}
			events.OnSessionUpdate(backend)

		default:
		}
	}
}
