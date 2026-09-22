package smtpserver

import (
	"crypto/tls"
	"errors"
	"net"
	"os"
	"time"
)

const (
	// tlsRecord is the first byte of a TLS handshake. A client that speaks SMTP in
	// the clear never opens with it, because it waits for the server's greeting.
	tlsRecord = 0x16

	// greetingPause is how long a new connection is given to show its hand before it
	// is taken for one that expects the greeting first. A handshake arrives at once
	// over loopback, so this is only ever spent on clients that are waiting anyway.
	greetingPause = 250 * time.Millisecond
)

// acceptEither lets the one SMTP port take both kinds of client: those that open with
// a TLS handshake and those that start in the clear and ask for STARTTLS.
//
// Apple Mail tries the handshake first, and a port that will not take it leaves Mail
// waiting out the read timeout before it retries — a minute of apparent nothing in the
// middle of setting up an account. Serving both is what spares the user that, and it
// costs no second port: the first byte a client sends already says which it is.
func acceptEither(listener net.Listener, config *tls.Config) net.Listener {
	if config == nil {
		return listener
	}
	return eitherListener{Listener: listener, config: config}
}

type eitherListener struct {
	net.Listener
	config *tls.Config
}

func (l eitherListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}

		ready, err := l.classify(conn)
		if err != nil {
			conn.Close()
			continue
		}
		return ready, nil
	}
}

// classify decides how a connection is speaking, and hands back one that still reads
// whole from its first byte.
func (l eitherListener) classify(conn net.Conn) (net.Conn, error) {
	if err := conn.SetReadDeadline(time.Now().Add(greetingPause)); err != nil {
		return nil, err
	}

	var opening [1]byte
	read, err := conn.Read(opening[:])

	if clearErr := conn.SetReadDeadline(time.Time{}); clearErr != nil {
		return nil, clearErr
	}

	switch {
	case read == 0 && isTimeout(err):
		return conn, nil
	case read == 0:
		return nil, err
	}

	opened := &replayConn{Conn: conn, first: opening[0]}
	if opening[0] == tlsRecord {
		return tls.Server(opened, l.config), nil
	}
	return opened, nil
}

func isTimeout(err error) bool {
	return errors.Is(err, os.ErrDeadlineExceeded)
}

// replayConn gives back the byte the connection was judged by, so whatever takes it
// over still sees the stream from the start.
type replayConn struct {
	net.Conn
	first    byte
	replayed bool
}

func (c *replayConn) Read(b []byte) (int, error) {
	if c.replayed || len(b) == 0 {
		return c.Conn.Read(b)
	}

	c.replayed = true
	b[0] = c.first
	return 1, nil
}
