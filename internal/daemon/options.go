// Package daemon owns the lifecycle of the local mail bridge services.
package daemon

import "mail-bridge-desktop/internal/config"

// Options configures one development bridge daemon instance.
//
// TODO(auth): replace the development mailbox options with the authenticated,
// unlocked account session and the production backend connector.
type Options struct {
	Config      config.Config
	IMAPAddress string
	StateDir    string
	MailAddress string
}

// DefaultOptions loads configuration and returns development-safe defaults.
func DefaultOptions() Options {
	cfg := config.Load()
	return Options{
		Config:      cfg,
		IMAPAddress: cfg.IMAPAddr,
		StateDir:    ".bridge-data",
		MailAddress: "user@example.test",
	}
}
