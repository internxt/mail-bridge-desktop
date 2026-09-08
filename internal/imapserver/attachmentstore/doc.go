// Package attachmentstore keeps the messages an IMAP client reads, completing
// them with their attachments the first time one is opened.
//
// It exists so that keeping the mailbox up to date stays cheap: a folder is
// brought in step with the account without downloading the files hanging off
// every message in it, and a file only travels when somebody actually asks to
// read the message carrying it.
package attachmentstore
