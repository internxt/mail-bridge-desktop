package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"mail-bridge-desktop/internal/daemon"
	"mail-bridge-desktop/internal/store"
)

func main() {
	options := parseOptions(daemon.DefaultOptions())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	credentials, err := store.New(options.StateDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bridge:", err)
		os.Exit(1)
	}
	// TODO(auth): remove once the desktop client pushes the session over IPC.
	seeded, err := credentials.SeedFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bridge:", err)
		os.Exit(1)
	}
	if len(seeded) > 0 {
		fmt.Printf("Stored development credentials: %v\n", seeded)
	}

	if err := daemon.Run(ctx, options); err != nil {
		fmt.Fprintln(os.Stderr, "bridge:", err)
		os.Exit(1)
	}
}

func parseOptions(options daemon.Options) daemon.Options {
	imapAddress := flag.String("imap-address", options.IMAPAddress, "local IMAP listen address")
	stateDir := flag.String("state-dir", options.StateDir, "directory for IMAP state")
	mailAddress := flag.String("mail-address", options.MailAddress, "development mailbox address")
	flag.Parse()

	options.IMAPAddress = *imapAddress
	options.StateDir = *stateDir
	options.MailAddress = *mailAddress
	return options
}
