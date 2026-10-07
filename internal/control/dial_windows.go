//go:build windows

package control

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Microsoft/go-winio"
)

func dial(ctx context.Context, endpoint string) (io.ReadWriteCloser, error) {
	if !strings.HasPrefix(endpoint, `\\.\pipe\`) {
		return nil, fmt.Errorf("control pipe must begin with \\\\.\\pipe\\")
	}
	// winio provides the implementation for an overlapped pipe
	// allowing incoming and outgoing messages to be handled concurrently
	// without block eachother
	return winio.DialPipeContext(ctx, endpoint)
}
