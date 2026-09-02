package store

import (
	"errors"
	"fmt"
	"os"
)

// Keys the bridge stores. They are named here so every package refers to the
// same strings.
const (
	KeyToken    = "token"
	KeyMnemonic = "mnemonic"
	KeyEmail    = "email"
)

// SeedFromEnv fills an empty store with credentials taken from the
// environment, so the daemon can run before the IPC channel with the desktop
// client exists.
//
// Values come from the .env, which config.Load already reads from .env.
// Nothing happens when they are unset, and stored values are
// never overwritten: a real session always wins over a development one.
//
// It reports which keys it wrote, for the caller to log.
//
// TODO(auth): drop this once the desktop client pushes the session over IPC.
func (s *Store) SeedFromEnv() ([]string, error) {
	seeds := map[string]string{
		KeyToken: os.Getenv("BRIDGE_DEV_TOKEN"),
	}

	var seeded []string
	for key, value := range seeds {
		if value == "" {
			continue
		}
		if _, err := s.Get(key); err == nil {
			continue
		} else if !errors.Is(err, ErrNotFound) {
			return seeded, fmt.Errorf("store: check %s: %w", key, err)
		}

		if err := s.Set(key, []byte(value)); err != nil {
			return seeded, fmt.Errorf("store: seed %s: %w", key, err)
		}
		seeded = append(seeded, key)
	}
	return seeded, nil
}
