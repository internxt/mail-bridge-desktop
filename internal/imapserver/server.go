package imapserver

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/ProtonMail/gluon"

	bridgeconfig "mail-bridge-desktop/internal/config"
)

// Start prepares the IMAP service, makes the mailbox available to email
// clients, and returns its lifecycle manager.
func Start(ctx context.Context, session UnlockedSession, config Config) (*IMAPServer, error) {
	config, err := resolveConfig(session, config)
	if err != nil {
		return nil, err
	}

	gluonServer, err := createGluonServer(config)
	if err != nil {
		return nil, err
	}
	defer closeOnFailure(gluonServer, &err)

	if err = connectMailbox(ctx, gluonServer, session, config); err != nil {
		return nil, err
	}

	listener, err := startServing(ctx, gluonServer, config.ListenAddress)
	if err != nil {
		return nil, err
	}

	return &IMAPServer{
		server:      gluonServer,
		listener:    listener,
		credentials: []byte(config.LocalCredentials.Password),
		started:     true,
		status: Status{
			Address:     listener.Addr().String(),
			Credentials: config.LocalCredentials,
			StartTLS:    config.TLSConfig != nil,
		},
	}, nil
}

func resolveConfig(session UnlockedSession, config Config) (Config, error) {
	if config.ListenAddress == "" {
		config.ListenAddress = bridgeconfig.Load().IMAPAddr
	}
	if err := validate(session, config); err != nil {
		return Config{}, err
	}
	if config.LocalCredentials.Username == "" {
		config.LocalCredentials.Username = session.Addresses[0]
	}

	// A generated passphrase only suits a throwaway server: Gluon's cache is
	// encrypted with it, so a new one on every start leaves the existing cache
	// unreadable and resynchronises every mailbox. Callers meant to outlive a
	// single run pass a stored one, from EnsureStoragePassphrase.
	if len(config.StoragePassphrase) == 0 {
		passphrase, err := randomBytes(storagePassphraseBytes)
		if err != nil {
			return Config{}, fmt.Errorf("generate IMAP storage passphrase: %w", err)
		}
		config.StoragePassphrase = passphrase
	}
	return config, nil
}

func createGluonServer(config Config) (*gluon.Server, error) {
	if err := os.MkdirAll(config.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create IMAP data directory: %w", err)
	}

	options := []gluon.Option{
		gluon.WithDataDir(filepath.Join(config.DataDir, "data")),
		gluon.WithDatabaseDir(filepath.Join(config.DataDir, "database")),
	}
	if config.TLSConfig != nil {
		options = append(options, gluon.WithTLS(config.TLSConfig))
	}

	gluonServer, err := gluon.New(options...)
	if err != nil {
		return nil, fmt.Errorf("create Gluon server: %w", err)
	}
	return gluonServer, nil
}

func connectMailbox(ctx context.Context, gluonServer *gluon.Server, session UnlockedSession, config Config) error {
	conn, err := config.ConnectorFactory(ctx, session, config.LocalCredentials)
	if err != nil {
		return fmt.Errorf("create mailbox connector: %w", err)
	}

	// Whether the connector can synchronise is asked of the original, not of
	// the wrapper below: authConnector embeds the Connector interface, which
	// does not declare Sync, so the method is not promoted and a type
	// assertion on the wrapper would quietly find nothing.
	synchronizer, canSync := conn.(interface{ Sync(context.Context) error })

	// Authorisation is enforced here rather than left to each connector, so
	// every mailbox is reached through the same check.
	conn = withAuthorization(conn, config.LocalCredentials)

	if _, err := gluonServer.AddUser(ctx, conn, config.StoragePassphrase); err != nil {
		return fmt.Errorf("add IMAP user: %w", err)
	}
	if canSync {
		if err := synchronizer.Sync(ctx); err != nil {
			return fmt.Errorf("perform initial mailbox sync: %w", err)
		}
	}
	return nil
}

func startServing(ctx context.Context, gluonServer *gluon.Server, address string) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen for IMAP: %w", err)
	}
	if err := gluonServer.Serve(ctx, listener); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("serve IMAP: %w", err)
	}
	return listener, nil
}

func closeOnFailure(gluonServer *gluon.Server, result *error) {
	if *result != nil {
		_ = gluonServer.Close(context.Background())
	}
}

// Status returns current mail-client connection settings. It returns an empty
// status after Close has cleared the local credentials.
func (s *IMAPServer) Status() Status {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.status
}

// Errors returns asynchronous IMAP service errors.
func (s *IMAPServer) Errors() <-chan error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.server == nil {
		return nil
	}
	return s.server.GetErrorCh()
}

// Close stops the IMAP service and removes the local bridge password from
// memory. It is safe to call more than once.
func (s *IMAPServer) Close(ctx context.Context) error {
	s.mutex.Lock()
	if !s.started {
		s.mutex.Unlock()
		return nil
	}
	s.started = false
	listener := s.listener
	gluonServer := s.server
	s.listener = nil
	s.server = nil
	clear(s.credentials)
	s.credentials = nil
	s.status.Credentials.Password = ""
	s.mutex.Unlock()

	var result error
	if listener != nil {
		result = errors.Join(result, listener.Close())
	}
	if gluonServer != nil {
		result = errors.Join(result, gluonServer.Close(ctx))
	}
	return result
}

func validate(session UnlockedSession, config Config) error {
	if session.AccountID == "" {
		return errors.New("unlocked session account ID is required")
	}
	if len(session.Addresses) == 0 || session.Addresses[0] == "" {
		return errors.New("unlocked session needs at least one mail address")
	}
	if config.DataDir == "" {
		return errors.New("IMAP data directory is required")
	}
	if config.ConnectorFactory == nil {
		return errors.New("IMAP connector factory is required")
	}
	if config.LocalCredentials.Password == "" {
		return errors.New("local IMAP password is required")
	}
	if config.ListenAddress != "" {
		host, _, err := net.SplitHostPort(config.ListenAddress)
		if err != nil {
			return fmt.Errorf("invalid IMAP listen address: %w", err)
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			return errors.New("IMAP listener must bind to loopback")
		}
	}
	return nil
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return nil, err
	}
	return value, nil
}
