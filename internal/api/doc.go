// Package api talks to the two Internxt APIs the bridge needs: drive, which
// handles authentication, and mail, which handles messages and attachments.
//
// Every request goes through do, which is where timeouts, retries and error
// translation live, so the endpoint methods stay thin.
package api
