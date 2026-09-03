package development

import (
	"fmt"

	"mail-bridge-desktop/internal/control"
)

// ReportConnectionSettings prints what a mail client needs to add the account.
//
// Addresses are printed whole, host and port together, so they can be copied
// straight into a client's settings. Encryption gets an explicit word because
// clients try TLS first and fail confusingly when the server does not offer it.
//
// This belongs to the parent, not the bridge: Drive Desktop shows these in its
// interface, and here they go to the terminal instead.
func ReportConnectionSettings(session control.Session, ready control.Ready) {
	fmt.Println()
	fmt.Println("The bridge is running. Add this account to a mail client:")
	fmt.Println()
	fmt.Printf("  Username:  %s\n", session.MailClient.Username)
	fmt.Printf("  Password:  %s\n", session.MailClient.Password)
	fmt.Println()
	fmt.Printf("  IMAP:      %s\n", ready.IMAPAddress)
	fmt.Printf("  SMTP:      %s\n", ready.SMTPAddress)
	fmt.Println()

	if ready.StartTLS {
		fmt.Println("  Security:  STARTTLS")
	} else {
		fmt.Println("  Security:  none — turn TLS/SSL off, and allow an insecure password")
	}

	fmt.Println()
	fmt.Println("The password is stored, so it stays the same across restarts.")
	fmt.Println("IMAP serves the account's mail; SMTP accepts submissions but does not send them yet.")
	fmt.Println("Press Ctrl+C to stop.")
}
