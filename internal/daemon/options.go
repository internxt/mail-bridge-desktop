// Package daemon owns the lifecycle of the local mail bridge services.
package daemon

import "mail-bridge-desktop/internal/config"

// DefaultOptions loads configuration and returns development-safe defaults.
func DefaultOptions() Options {
	config := config.Load()
	options := Options{
		Config:      config,
		IMAPAddress: config.IMAPAddr,
		StateDir:    ".bridge-data",
	}
	return options
}
