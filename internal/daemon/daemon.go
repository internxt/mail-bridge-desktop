package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"mail-bridge-desktop/internal/control"
	"mail-bridge-desktop/internal/imapserver"
	"mail-bridge-desktop/internal/smtpserver"
	"mail-bridge-desktop/internal/store"
)

const shutdownTimeout = 10 * time.Second

// Run starts the local bridge services, waits for context cancellation, and
// shuts down every started service before returning.
func Run(ctx context.Context, options Options) error {
	if options.ControlEndpoint == "" {
		return errors.New("control endpoint is required")
	}
	controlClient, err := control.Connect(ctx, options.ControlEndpoint)
	if err != nil {
		return err
	}
	defer controlClient.Close()
	session, err := controlClient.ReceiveStartSession(ctx)
	if err != nil {
		return err
	}

	imapService, err := startIMAP(ctx, options, session)
	if err != nil {
		_ = controlClient.SendError("", "start_imap")
		return err
	}

	smtpService, err := startSMTP(options, session)
	if err != nil {
		_ = imapService.Close(context.Background())
		_ = controlClient.SendError("", "start_smtp")
		return err
	}

	imapStatus := imapService.Status()
	if err := controlClient.SendReady(control.Ready{
		IMAPAddress: imapStatus.Address,
		SMTPAddress: options.Config.SMTPAddr,
		StartTLS:    imapStatus.StartTLS,
	}); err != nil {
		_ = smtpService.Stop(context.Background())
		_ = imapService.Close(context.Background())
		return fmt.Errorf("report bridge readiness: %w", err)
	}

	reportStarted()
	<-ctx.Done()

	return shutdownServices(imapService, smtpService)
}

func startIMAP(ctx context.Context, options Options, session control.Session) (*imapserver.IMAPServer, error) {
	passphrase, err := storagePassphrase(options.StateDir)
	if err != nil {
		return nil, fmt.Errorf("start IMAP: %w", err)
	}

	// TODO(backend): use session.BackendSession in the production connector.
	// Until that exists, the current connector remains fixture-only.
	service, err := imapserver.Start(ctx, imapserver.UnlockedSession{
		AccountID: session.AccountID,
		Addresses: session.Addresses,
	}, imapserver.Config{
		ListenAddress: options.IMAPAddress,
		DataDir:       options.StateDir,
		LocalCredentials: imapserver.Credentials{
			Username: session.MailClient.Username,
			Password: session.MailClient.Password,
		},
		StoragePassphrase: passphrase,
		ConnectorFactory: imapserver.NewDevelopmentConnectorFactory([][]byte{
			[]byte("From: welcome@example.test\r\nTo: user@example.test\r\nSubject: Mail Bridge development server\r\n\r\nThe IMAP server is serving this local fixture message.\r\n"),
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("start IMAP: %w", err)
	}
	return service, nil
}

// storagePassphrase reads the key encrypting Gluon's message cache, generating
// and storing it the first time.
func storagePassphrase(stateDir string) ([]byte, error) {
	credentials, err := store.New(stateDir)
	if err != nil {
		return nil, err
	}
	return imapserver.EnsureStoragePassphrase(credentials)
}

func startSMTP(options Options, session control.Session) (*smtpserver.Service, error) {
	service := smtpserver.New(options.Config, smtpserver.Credentials{
		Username: session.MailClient.Username,
		Password: session.MailClient.Password,
	})
	if err := service.Start(); err != nil {
		return nil, fmt.Errorf("start SMTP: %w", err)
	}
	return service, nil
}

func reportStarted() {
	fmt.Println("Bridge started after authenticated Drive Desktop startup handshake.")
	fmt.Println("Press Ctrl+C to stop.")
}

func shutdownServices(imapService *imapserver.IMAPServer, smtpService *smtpserver.Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return errors.Join(
		imapService.Close(ctx),
		smtpService.Stop(ctx),
	)
}
