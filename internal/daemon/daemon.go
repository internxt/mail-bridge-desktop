package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"mail-bridge-desktop/internal/imapserver"
	"mail-bridge-desktop/internal/smtpserver"
)

const shutdownTimeout = 10 * time.Second

// Run starts the local bridge services, waits for context cancellation, and
// shuts down every started service before returning.
func Run(ctx context.Context, options Options) error {
	imapService, err := startIMAP(ctx, options)
	if err != nil {
		return err
	}

	smtpService, err := startSMTP(options)
	if err != nil {
		_ = imapService.Close(context.Background())
		return err
	}

	reportConnectionSettings(imapService, options)
	<-ctx.Done()

	return shutdownServices(imapService, smtpService)
}

func startIMAP(ctx context.Context, options Options) (*imapserver.IMAPServer, error) {
	// TODO(auth): replace this development session and fixture connector with
	// the account session and decrypted mailbox connector from the real backend.
	service, err := imapserver.Start(ctx, imapserver.UnlockedSession{
		AccountID: "development-account",
		Addresses: []string{options.MailAddress},
	}, imapserver.Config{
		ListenAddress: options.IMAPAddress,
		DataDir:       options.StateDir,
		ConnectorFactory: imapserver.NewDevelopmentConnectorFactory([][]byte{
			[]byte("From: welcome@example.test\r\nTo: user@example.test\r\nSubject: Mail Bridge development server\r\n\r\nThe IMAP server is serving this local fixture message.\r\n"),
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("start IMAP: %w", err)
	}
	return service, nil
}

func startSMTP(options Options) (*smtpserver.Service, error) {
	service := smtpserver.New(options.Config)
	if err := service.Start(); err != nil {
		return nil, fmt.Errorf("start SMTP: %w", err)
	}
	return service, nil
}

// TODO: This should be forwarded through ip to the ui
func reportConnectionSettings(imapService *imapserver.IMAPServer, options Options) {
	status := imapService.Status()
	fmt.Printf("IMAP server: %s\nUsername: %s\nPassword: %s\n", status.Address, status.Credentials.Username, status.Credentials.Password)
	fmt.Printf("SMTP server: %s\n", options.Config.SMTPAddr)
	fmt.Println("IMAP serves development fixture mail; SMTP accepts development submissions but does not deliver them yet.")
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
