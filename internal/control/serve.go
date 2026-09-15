package control

import (
	"context"
	"fmt"
)

type Events struct {
	OnResync func()
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
			return fmt.Errorf("read control message: %w", err)
		}

		switch message.Type {
		case resyncType:
			if events.OnResync != nil {
				events.OnResync()
			}
		default:
		}
	}
}
