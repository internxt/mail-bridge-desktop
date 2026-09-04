// Package daemon orchestrates the bridge process for one unlocked session: it
// starts the local IMAP and SMTP services, reports readiness to the parent
// application over the control channel, and shuts both services down when
// the session ends.
package daemon

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/ProtonMail/gluon/connector"
	gluonstore "github.com/ProtonMail/gluon/store"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/control"
	"mail-bridge-desktop/internal/imapserver"
	"mail-bridge-desktop/internal/imapserver/attachmentstore"
	"mail-bridge-desktop/internal/imapserver/mailconnector"
	"mail-bridge-desktop/internal/logger"
	"mail-bridge-desktop/internal/mail"
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

	log := logger.New("mail")
	service, serviceErr := mailService(ctx, options, session, log)
	if serviceErr != nil {
		log.Warn("serving fixture mail: %v", serviceErr)
	}

	imapService, err := startIMAP(ctx, options, session, service)
	if err != nil {
		_ = controlClient.SendError("", "start_imap")
		return err
	}

	smtpService, err := startSMTP(options, session, senderOrNil(service))
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

func startIMAP(ctx context.Context, options Options, session control.Session, service *mail.MailService) (*imapserver.IMAPServer, error) {
	passphrase, err := storagePassphrase(options.StateDir)
	if err != nil {
		return nil, fmt.Errorf("start IMAP: %w", err)
	}

	imapService, err := imapserver.Start(ctx, imapserver.UnlockedSession{
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
		ConnectorFactory:  connectorFactory(service),
		StoreBuilder:      attachmentStoreBuilder(service),
		LogProtocol:       options.Config.LogImapProtocol,
		PollInterval:      mailconnector.DefaultPollInterval,
	})
	if err != nil {
		return nil, fmt.Errorf("start IMAP: %w", err)
	}
	return imapService, nil
}

// connectorFactory serves the account's own mail, falling back to a fixture
// mailbox when the Mail API is not reachable.
func connectorFactory(service *mail.MailService) imapserver.ConnectorFactory {
	if service == nil {
		return imapserver.NewDevelopmentConnectorFactory([][]byte{
			[]byte("From: welcome@example.test\r\nTo: user@example.test\r\nSubject: Mail Bridge development server\r\n\r\nThe IMAP server is serving this local fixture message.\r\n"),
		})
	}

	imapLog := logger.New("imap")
	return func(ctx context.Context, _ imapserver.UnlockedSession, _ imapserver.Credentials) (connector.Connector, error) {
		return mailconnector.New(service, imapLog), nil
	}
}

// attachmentStoreBuilder keeps attachments out of the sync: messages are
// stored with their attachments declared but empty, and the files are fetched
// the first time a client opens one. Without a mail service there is nothing
// to fetch them from, so Gluon keeps its own store.
func attachmentStoreBuilder(service *mail.MailService) gluonstore.Builder {
	if service == nil {
		return nil
	}
	return attachmentstore.NewBuilder(
		&gluonstore.OnDiskStoreBuilder{},
		service,
		mail.MessageIDDomain,
		logger.New("imap"),
	)
}

// mailService builds the service that reads and sends the account's mail,
// from the session the parent sent. Its own public key is fetched once here,
// so sending mail later needs no extra round trip for it.
//
// Nothing here is read from disk: the token and the keys live as long as the
// session does, which is what makes signing out a matter of closing it.
func mailService(ctx context.Context, options Options, session control.Session, log *logger.Logger) (*mail.MailService, error) {
	backend, err := session.Backend()
	if err != nil {
		return nil, err
	}
	if backend.Token == "" {
		return nil, errors.New("the session carries no Mail API token")
	}

	client, err := api.New(options.Config, log)
	if err != nil {
		return nil, err
	}

	service := mail.New(client, mail.Account{
		Token:      backend.Token,
		Address:    session.Addresses[0],
		PrivateKey: encryptionKey(backend.EncryptionPrivateKey, log),
	}, options.Config.ServerPublicKey, log)

	if err := service.Init(ctx); err != nil {
		log.Warn("sending mail will not be readable in Sent: %v", err)
	}

	return service, nil
}

const privateKeyLen = 32

// We need to decode the base64 encryption key
func encryptionKey(encoded string, log *logger.Logger) []byte {
	if encoded == "" {
		return nil
	}

	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		log.Warn("serving mail undecrypted: decode encryption private key: %v", err)
		return nil
	}
	if len(key) != privateKeyLen {
		log.Warn("serving mail undecrypted: encryption private key is %d bytes, want %d", len(key), privateKeyLen)
		return nil
	}

	return key
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

func senderOrNil(service *mail.MailService) smtpserver.Sender {
	if service == nil {
		return nil
	}
	return service
}

func startSMTP(options Options, session control.Session, sender smtpserver.Sender) (*smtpserver.Service, error) {
	service := smtpserver.New(options.Config, smtpserver.Credentials{
		Username: session.MailClient.Username,
		Password: session.MailClient.Password,
	}, sender)
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
