package imapserver

import (
	"context"
	"crypto/subtle"
	"strings"

	"github.com/ProtonMail/gluon/connector"
)

// authConnector wraps a connector and takes over authorisation, leaving every
// other operation to the connector it embeds.
//
// It exists because Gluon's own Dummy connector compares passwords with
// bytes.Compare, which returns as soon as two bytes differ and so takes longer
// the more of the password an attacker gets right. Authorisation is the one
// method worth owning, and embedding means the other thirteen come for free.
type authConnector struct {
	connector.Connector
	username string
	password []byte
}

func withAuthorization(conn connector.Connector, credentials Credentials) connector.Connector {
	return &authConnector{
		Connector: conn,
		username:  credentials.Username,
		password:  []byte(credentials.Password),
	}
}

// Authorize reports whether a mail client may open this mailbox.
//
// The username is matched case-insensitively because mail clients do not
// preserve the case a user typed, and an address is the same address either
// way. The password is compared in constant time so a failed attempt reveals
// nothing about how much of it was right.
func (c *authConnector) Authorize(_ context.Context, username string, password []byte) bool {
	sameUser := strings.EqualFold(username, c.username)
	samePassword := subtle.ConstantTimeCompare(password, c.password) == 1

	return sameUser && samePassword
}
