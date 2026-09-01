//go:build windows

package control

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func dial(ctx context.Context, endpoint string) (io.ReadWriteCloser, error) {
	if !strings.HasPrefix(endpoint, `\\.\pipe\`) {
		return nil, fmt.Errorf("control pipe must begin with \\\\.\\pipe\\")
	}
	name, err := windows.UTF16PtrFromString(endpoint)
	if err != nil {
		return nil, err
	}
	for {
		handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err == nil {
			return os.NewFile(uintptr(handle), endpoint), nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != windows.ERROR_PIPE_BUSY {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}
