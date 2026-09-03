package imapserver

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/ProtonMail/gluon"
	"github.com/ProtonMail/gluon/connector"

	"mail-bridge-desktop/internal/imapserver/mailconnector"
)

// UnlockedSession identifies an account whose authentication and encryption
// keys have already been unlocked by the desktop application.
//
// TODO(auth): add the minimal key/session handle required by the production connector.
// Do not add an account password to this type: passwords are used
// only by the application while unlocking this session.
type UnlockedSession struct {
	AccountID string
	Addresses []string
}

// Credentials are the local IMAP credentials configured in a desktop client.
// They are distinct from the user's main account credentials.
type Credentials struct {
	Username string
	Password string
}

// Status contains the connection details the desktop application should show
// while the IMAP server is running.
type Status struct {
	Address     string
	Credentials Credentials
	StartTLS    bool
}

// ConnectorFactory creates the mailbox connector for an unlocked account.
//
// TODO(backend): replace the development connector with an implementation
// that fetches encrypted mail from the backend, decrypts it locally, and
// converts IMAP-originated mutations into backend API operations.
type ConnectorFactory func(context.Context, UnlockedSession, Credentials) (connector.Connector, error)

// Config controls one local IMAP service instance.
type Config struct {
	ListenAddress     string
	DataDir           string
	TLSConfig         *tls.Config
	LocalCredentials  Credentials
	StoragePassphrase []byte
	ConnectorFactory  ConnectorFactory
	LogProtocol       bool
	PollInterval      time.Duration
}

// IMAPServer owns a running IMAP service, its listener, and its local client
// credentials.
type IMAPServer struct {
	mutex       sync.Mutex
	server      *gluon.Server
	listener    net.Listener
	stopServing context.CancelFunc
	poller      *mailconnector.Poller

	status      Status
	credentials []byte
	started     bool
}
