package imapserver

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ProtonMail/gluon/imap"
	"github.com/ProtonMail/gluon/store"
)

// recordingStore wraps Gluon's own store, noting every read and letting a test
// rewrite what a fetch returns.
type recordingStore struct {
	inner store.Store

	mutex   sync.Mutex
	gets    []imap.InternalMessageID
	rewrite func([]byte) []byte
}

func (s *recordingStore) Get(messageID imap.InternalMessageID) ([]byte, error) {
	literal, err := s.inner.Get(messageID)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock()
	s.gets = append(s.gets, messageID)
	rewrite := s.rewrite
	s.mutex.Unlock()

	if rewrite != nil {
		return rewrite(literal), nil
	}
	return literal, nil
}

func (s *recordingStore) Set(messageID imap.InternalMessageID, reader io.Reader) error {
	return s.inner.Set(messageID, reader)
}

func (s *recordingStore) Delete(messageID ...imap.InternalMessageID) error {
	return s.inner.Delete(messageID...)
}

func (s *recordingStore) Close() error                            { return s.inner.Close() }
func (s *recordingStore) List() ([]imap.InternalMessageID, error) { return s.inner.List() }

func (s *recordingStore) readCount() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.gets)
}

type recordingStoreBuilder struct {
	inner store.Builder
	store *recordingStore
}

func (b *recordingStoreBuilder) New(dir, userID string, passphrase []byte) (store.Store, error) {
	inner, err := b.inner.New(dir, userID, passphrase)
	if err != nil {
		return nil, err
	}
	b.store.inner = inner
	return b.store, nil
}

func (b *recordingStoreBuilder) Delete(dir, userID string) error {
	return b.inner.Delete(dir, userID)
}

// TestStoreBuilderResolvesLiteralsOnFetch is the premise the attachment
// support rests on: Gluon accepts a store of our own, asks it for the literal
// when a client fetches a body, and serves back whatever it returns.
//
// That is what allows a message to be synced without its attachments and
// completed the first time it is opened, rather than downloading every
// attachment in a folder on every sync.
func TestStoreBuilderResolvesLiteralsOnFetch(t *testing.T) {
	t.Parallel()

	recorder := &recordingStore{
		rewrite: func(literal []byte) []byte {
			return []byte(strings.Replace(string(literal), "placeholder body", "resolved on fetch", 1))
		},
	}

	server, err := Start(context.Background(), UnlockedSession{
		AccountID: "account-1",
		Addresses: []string{"user@example.test"},
	}, Config{
		ListenAddress:    "127.0.0.1:0",
		DataDir:          t.TempDir(),
		LocalCredentials: Credentials{Password: "local-password"},
		ConnectorFactory: NewDevelopmentConnectorFactory([][]byte{
			[]byte("From: sender@example.test\r\nTo: user@example.test\r\nSubject: fixture\r\n\r\nplaceholder body\r\n"),
		}),
		StoreBuilder: &recordingStoreBuilder{inner: &store.OnDiskStoreBuilder{}, store: recorder},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	conn, err := net.DialTimeout("tcp", server.Status().Address, time.Second)
	if err != nil {
		t.Fatalf("dial IMAP server: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	readLine(t, reader)

	credentials := server.Status().Credentials
	writeCommand(t, conn, "a1 LOGIN %s %s\r\n", credentials.Username, credentials.Password)
	if response := readThroughTag(t, reader, "a1"); !strings.Contains(response, "a1 OK") {
		t.Fatalf("LOGIN response = %q", response)
	}

	writeCommand(t, conn, "a2 SELECT INBOX\r\n")
	if response := readThroughTag(t, reader, "a2"); !strings.Contains(response, "a2 OK") {
		t.Fatalf("SELECT response = %q", response)
	}

	// Metadata alone must not reach the store: it is answered from Gluon's
	// database, which is what keeps a sync from having to download bodies.
	writeCommand(t, conn, "a3 FETCH 1 (RFC822.SIZE BODYSTRUCTURE)\r\n")
	if response := readThroughTag(t, reader, "a3"); !strings.Contains(response, "a3 OK") {
		t.Fatalf("FETCH metadata response = %q", response)
	}
	if count := recorder.readCount(); count != 0 {
		t.Errorf("the store was read %d times for metadata alone, want 0", count)
	}

	writeCommand(t, conn, "a4 FETCH 1 (BODY.PEEK[])\r\n")
	response := readThroughTag(t, reader, "a4")
	if !strings.Contains(response, "a4 OK") {
		t.Fatalf("FETCH body response = %q", response)
	}

	if recorder.readCount() == 0 {
		t.Fatal("fetching a body never reached the store, so attachments cannot be resolved there")
	}
	if !strings.Contains(response, "resolved on fetch") {
		t.Errorf("FETCH served %q, want the literal the store returned", response)
	}
}
