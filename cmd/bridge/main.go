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
	options, reset := parseOptions(daemon.DefaultOptions())

	// A reset never touches the control socket or starts a server: it runs on
	// logout, when there is no session left to serve, only state to wipe.
	if reset {
		if err := daemon.Reset(options.StateDir); err != nil {
			fmt.Fprintln(os.Stderr, "bridge: reset:", err)
			os.Exit(1)
		}
		return
	}

	context, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := daemon.Run(context, options); err != nil {
		fmt.Fprintln(os.Stderr, "bridge:", err)
		os.Exit(1)
	}
}

func parseOptions(options daemon.Options) (daemon.Options, bool) {
	imapAddress := flag.String("imap-address", options.IMAPAddress, "local IMAP listen address")
	stateDir := flag.String("state-dir", options.StateDir, "directory for IMAP state")
	controlEndpoint := flag.String("control-endpoint", options.ControlEndpoint, "Drive Desktop control socket path (Unix) or named pipe (Windows)")
	reset := flag.Bool("reset", false, "clear all stored bridge state for -state-dir and exit")
	flag.Parse()

	options.IMAPAddress = *imapAddress
	options.StateDir = *stateDir
	options.ControlEndpoint = *controlEndpoint
	return options, *reset
}
