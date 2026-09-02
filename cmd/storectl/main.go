// Command storectl inspects and clears the bridge's credential store.
//
// The store outlives the process, and what it holds is invisible from the
// outside: this tells you what is in there, and lets you start over.
//
// Clearing is not free. It drops the passphrase encrypting Gluon's message
// cache, so the mail already synchronised on disk becomes unreadable and every
// mailbox is fetched again.
package main

import (
	"flag"
	"fmt"
	"os"

	"mail-bridge-desktop/internal/daemon"
	"mail-bridge-desktop/internal/store"
)

func main() {
	stateDir := flag.String("state-dir", daemon.DefaultOptions().StateDir, "directory holding the store's state")
	flag.Parse()

	if err := run(flag.Arg(0), *stateDir); err != nil {
		fmt.Fprintln(os.Stderr, "storectl:", err)
		os.Exit(1)
	}
}

func run(command, stateDir string) error {
	credentials, err := store.New(stateDir)
	if err != nil {
		return err
	}

	switch command {
	case "list":
		return list(credentials)
	case "clear":
		return clearStore(credentials)
	default:
		return fmt.Errorf("unknown command %q, want list or clear", command)
	}
}

// list reports which keys are stored, never their values: they are
// credentials, and printing them would leave them in the terminal's history.
func list(credentials *store.Store) error {
	keys, err := credentials.Keys()
	if err != nil {
		return fmt.Errorf("list keys: %w", err)
	}
	if len(keys) == 0 {
		fmt.Println("The store is empty.")
		return nil
	}

	fmt.Printf("%d key(s) stored:\n", len(keys))
	for _, key := range keys {
		fmt.Println("  " + key)
	}
	return nil
}

func clearStore(credentials *store.Store) error {
	keys, err := credentials.Keys()
	if err != nil {
		return fmt.Errorf("list keys: %w", err)
	}
	if len(keys) == 0 {
		fmt.Println("The store is already empty.")
		return nil
	}

	if err := credentials.Clear(); err != nil {
		return fmt.Errorf("clear store: %w", err)
	}

	fmt.Printf("Removed %d key(s): %v\n", len(keys), keys)
	fmt.Println("Gluon's message cache is now unreadable, so every mailbox will be synchronised again.")
	return nil
}
