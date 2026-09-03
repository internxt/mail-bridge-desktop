// Package imapserver runs the local IMAP service for the mail bridge: it
// starts and stops the server, authenticates the desktop mail client, and
// manages its listener and credentials for as long as the bridge is running.
//
// What a client sees once connected — the account's mailboxes and messages —
// comes from the mailconnector subpackage.
package imapserver
