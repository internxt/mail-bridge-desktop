package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"mail-bridge-desktop/internal/daemon"
)

func main() {
	options := parseOptions(daemon.DefaultOptions())
	context, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	

	if err := daemon.Run(context, options); err != nil {
		fmt.Fprintln(os.Stderr, "bridge:", err)
		os.Exit(1)
	}
}

func parseOptions(options daemon.Options) daemon.Options {
	imapAddress := flag.String("imap-address", options.IMAPAddress, "local IMAP listen address")
	stateDir := flag.String("state-dir", options.StateDir, "directory for IMAP state")
	controlEndpoint := flag.String("control-endpoint", options.ControlEndpoint, "Drive Desktop control socket path (Unix) or named pipe (Windows)")
	flag.Parse()

	options.IMAPAddress = *imapAddress
	options.StateDir = *stateDir
	options.ControlEndpoint = *controlEndpoint
	return options
}
