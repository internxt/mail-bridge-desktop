// Package daemon owns the lifecycle of the local mail bridge services.
package daemon

import (
	"path/filepath"

	"mail-bridge-desktop/internal/config"
)

const DefaultStateDir = ".bridge-data"

// DefaultOptions loads configuration and returns development-safe defaults.
//
// The control endpoint defaults to a path inside the state directory, so a
// bridge and a parent both started without flags meet at the same place. Drive
// Desktop creates its own and passes it with -control-endpoint.
func DefaultOptions() Options {
	config := config.Load()
	options := Options{
		Config:          config,
		IMAPAddress:     config.IMAPAddr,
		StateDir:        DefaultStateDir,
		ControlEndpoint: filepath.Join(DefaultStateDir, "control.sock"),
	}
	return options
}
