package daemon

import (
	"errors"
	"fmt"
	"os"

	"mail-bridge-desktop/internal/store"
)

// Reset wipes everything the bridge has stored for stateDir: it clears the
// credential store, then removes the state directory itself — which also
// holds Gluon's databases and caches, the store's own disk files, and the
// control socket. It is what a logout uses to leave no trace of the account.
//
// A missing state directory or an already-empty store is success. Both steps
// are attempted even if one fails, so a problem clearing the store never
// stops the directory from being removed, and vice versa.
func Reset(stateDir string) error {
	credentials, err := store.New(stateDir)
	return reset(stateDir, credentials, err)
}

func reset(stateDir string, credentials *store.Store, openErr error) error {
	var errs []error

	switch {
	case openErr != nil:
		errs = append(errs, fmt.Errorf("open store: %w", openErr))
	default:
		if err := credentials.Clear(); err != nil {
			errs = append(errs, fmt.Errorf("clear store: %w", err))
		}
	}

	if err := os.RemoveAll(stateDir); err != nil {
		errs = append(errs, fmt.Errorf("remove state directory: %w", err))
	}

	return errors.Join(errs...)
}
