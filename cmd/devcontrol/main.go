// Command devcontrol stands in for Drive Desktop during development.
//
// It creates the control endpoint, waits for the bridge to connect, sends it a
// session built from the .env, and prints the settings a mail client needs once
// the bridge reports its listeners.
//
// Run it before the bridge:
//
//	make dev-control    # this command
//	make run            # the bridge, in another terminal
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"mail-bridge-desktop/internal/daemon"
	"mail-bridge-desktop/internal/development"
)

func main() {
	defaults := daemon.DefaultOptions()
	endpoint := flag.String("control-endpoint", defaults.ControlEndpoint, "control socket path to create")
	stateDir := flag.String("state-dir", defaults.StateDir, "directory holding the development password")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *endpoint, *stateDir); err != nil {
		fmt.Fprintln(os.Stderr, "devcontrol:", err)
		os.Exit(1)
	}
}

func run(context context.Context, endpoint, stateDir string) error {
	session, err := development.SessionFromEnv(stateDir)
	if err != nil {
		return err
	}

	listener, err := listen(endpoint)
	if err != nil {
		return err
	}
	defer listener.Close()

	fmt.Printf("Waiting for the bridge on %s\n", endpoint)
	fmt.Println("Start it with: make run")

	ready, connection, err := development.Serve(context, listener, session)
	if err != nil {
		return err
	}
	// Held open until this command stops: the bridge treats a closed control
	// channel as its parent going away.
	defer connection.Close()

	development.ReportConnectionSettings(session, ready)

	<-context.Done()
	return nil
}

// listen creates the endpoint, removing a socket left behind by a previous run.
// The bridge dials this path, so a stale file would make it connect to nothing.
func listen(endpoint string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(endpoint), 0o700); err != nil {
		return nil, fmt.Errorf("create endpoint directory: %w", err)
	}
	if err := os.Remove(endpoint); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale endpoint: %w", err)
	}

	listener, err := net.Listen("unix", endpoint)
	if err != nil {
		return nil, fmt.Errorf("listen on control endpoint: %w", err)
	}
	return listener, nil
}
