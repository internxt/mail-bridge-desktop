package daemon

import "mail-bridge-desktop/internal/config"

// Options configures one bridge daemon instance.
//
// The account is not here: it arrives from the parent over the control
// channel, so everything specific to a user comes from the session rather than
// from how the process was started.
type Options struct {
	Config          config.Config
	IMAPAddress     string
	StateDir        string
	ControlEndpoint string
}
