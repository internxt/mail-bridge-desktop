//go:build darwin || linux

package control

import (
	"context"
	"io"
	"net"
)

func dial(ctx context.Context, endpoint string) (io.ReadWriteCloser, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", endpoint)
}
