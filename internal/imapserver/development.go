package imapserver

import (
	"context"
	"fmt"
	"time"

	"github.com/ProtonMail/gluon/connector"
	"github.com/ProtonMail/gluon/imap"
)

// NewDevelopmentConnectorFactory supplies an in-memory mailbox for local
// IMAP development. Production code must provide a connector backed by the
// backend API and the account's unlocked key material.
func NewDevelopmentConnectorFactory(messages [][]byte) ConnectorFactory {
	return func(ctx context.Context, session UnlockedSession, credentials Credentials) (connector.Connector, error) {
		conn := connector.NewDummy(
			session.Addresses,
			[]byte(credentials.Password),
			time.Millisecond,
			imap.NewFlagSet(`\\Answered`, `\\Seen`, `\\Flagged`, `\\Deleted`),
			imap.NewFlagSet(`\\Answered`, `\\Seen`, `\\Flagged`, `\\Deleted`),
			imap.NewFlagSet(),
		)

		for _, message := range messages {
			if _, _, err := conn.CreateMessage(ctx, imap.MailboxID("0"), message, imap.NewFlagSet(), time.Now()); err != nil {
				return nil, fmt.Errorf("add development message: %w", err)
			}
		}
		conn.ClearUpdates()
		return conn, nil
	}
}
