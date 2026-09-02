package imapserver

import (
	"context"
	"testing"

	"github.com/ProtonMail/gluon/connector"
)

func testAuthConnector() connector.Connector {
	return withAuthorization(nil, Credentials{
		Username: "user@inxt.me",
		Password: "s3cret-bridge-password",
	})
}

func TestAuthorize(t *testing.T) {
	authorizer, ok := testAuthConnector().(interface {
		Authorize(context.Context, string, []byte) bool
	})
	if !ok {
		t.Fatal("the wrapped connector does not implement Authorize")
	}

	for _, tc := range []struct {
		name     string
		username string
		password string
		want     bool
	}{
		{"correct credentials", "user@inxt.me", "s3cret-bridge-password", true},
		{"wrong password", "user@inxt.me", "not-the-password", false},
		{"empty password", "user@inxt.me", "", false},
		{"wrong username", "someone@inxt.me", "s3cret-bridge-password", false},
		{"empty username", "", "s3cret-bridge-password", false},

		// Mail clients do not preserve the case a user typed, and an address
		// is the same address either way.
		{"uppercased username", "USER@INXT.ME", "s3cret-bridge-password", true},
		{"mixed case username", "User@Inxt.Me", "s3cret-bridge-password", true},

		// The password is a secret, so its case is significant.
		{"uppercased password", "user@inxt.me", "S3CRET-BRIDGE-PASSWORD", false},

		// A prefix of the password must not be enough.
		{"password prefix", "user@inxt.me", "s3cret", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := authorizer.Authorize(context.Background(), tc.username, []byte(tc.password))
			if got != tc.want {
				t.Errorf("Authorize(%q, %q) = %v, want %v", tc.username, tc.password, got, tc.want)
			}
		})
	}
}
