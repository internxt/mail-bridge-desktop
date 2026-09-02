// protocolLogger writes the IMAP conversation to the bridge's log.
//
// It exists to answer "what is the client actually asking for?", which is
// otherwise invisible: a mail client that shows nothing may be failing to send
// a command, or sending one the server answers badly, and the two look the
// same from outside.
//
// Gluon hands each side of the conversation an io.Writer and writes raw
// protocol bytes to it, so this turns those into log lines.

package imapserver

import (
	"bytes"
	"strings"

	"mail-bridge-desktop/internal/logger"
)

// direction labels which side of the conversation a line came from.
type direction string

const (
	fromClient direction = "client ->"
	fromServer direction = "server ->"
)

type protocolLogger struct {
	log       *logger.Logger
	direction direction

	// partial holds bytes from a write that did not end on a line boundary.
	// Gluon writes when it pleases, not once per command, so a line can arrive
	// in pieces.
	partial []byte
}

func newProtocolLogger(log *logger.Logger, from direction) *protocolLogger {
	return &protocolLogger{log: log, direction: from}
}

func (l *protocolLogger) Write(payload []byte) (int, error) {
	l.partial = append(l.partial, payload...)

	for {
		end := bytes.IndexByte(l.partial, '\n')
		if end < 0 {
			break
		}
		line := strings.TrimRight(string(l.partial[:end]), "\r")
		l.partial = l.partial[end+1:]

		if line != "" {
			l.log.Info("%s %s", l.direction, line)
		}
	}

	return len(payload), nil
}
