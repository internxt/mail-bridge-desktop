package daemon

import "mail-bridge-desktop/internal/config"

// Options configures one development bridge daemon instance.
// TODO(auth): replace the development mailbox options with the authenticated,
// unlocked account session and the production backend connector.
type Options struct {
	Config          config.Config
	IMAPAddress     string
	StateDir        string
	ControlEndpoint string
}
