package attachmentstore

import (
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/ProtonMail/gluon/imap"
	"github.com/ProtonMail/gluon/rfc822"
	"github.com/ProtonMail/gluon/store"

	"mail-bridge-desktop/internal/logger"
)

const (
	messageIDHeader = "Message-ID"
	internalIDKey   = "X-Pm-Gluon-Id"
)

func NewBuilder(inner store.Builder, resolver Resolver, messageIDDomain string, log *logger.Logger) *Builder {
	return &Builder{inner: inner, resolver: resolver, messageIDDomain: messageIDDomain, log: log}
}

func (b *Builder) New(dir, userID string, passphrase []byte) (store.Store, error) {
	inner, err := b.inner.New(dir, userID, passphrase)
	if err != nil {
		return nil, err
	}

	return &Store{
		inner:           inner,
		resolver:        b.resolver,
		messageIDDomain: b.messageIDDomain,
		log:             b.log,
		resolved:        make(map[imap.InternalMessageID]bool),
	}, nil
}

func (b *Builder) Delete(dir, userID string) error { return b.inner.Delete(dir, userID) }

// Get returns a message, downloading its attachments the first time it is
// asked for.
//
// Gluon reads this when a client fetches a body, and answers everything else —
// size, structure, flags — from its own database, so this is the one place
// where a file has to travel and the only moment it does.
//
// A message that cannot be completed is served as it was stored: its
// attachments arrive empty, which still lets the client read the mail rather
// than failing the fetch outright.
func (s *Store) Get(messageID imap.InternalMessageID) ([]byte, error) {
	literal, err := s.inner.Get(messageID)
	if err != nil {
		return nil, err
	}

	if s.isResolved(messageID) {
		return literal, nil
	}

	emailID, found := emailIDOf(literal, s.messageIDDomain)
	if !found {
		s.markResolved(messageID)
		return literal, nil
	}

	complete, err := s.resolver.ResolveAttachments(context.Background(), emailID, literal)
	if err != nil {
		s.log.Warn("serving message %s without its attachments: %v", emailID, err)
		return literal, nil
	}

	complete, err = carryInternalID(literal, complete)
	if err != nil {
		s.log.Warn("serving message %s without its attachments: %v", emailID, err)
		return literal, nil
	}

	if err := s.inner.Set(messageID, bytes.NewReader(complete)); err != nil {
		s.log.Warn("could not store the completed message %s: %v", emailID, err)
	}

	s.markResolved(messageID)
	return complete, nil
}

func (s *Store) Set(messageID imap.InternalMessageID, reader io.Reader) error {
	s.forget(messageID)
	return s.inner.Set(messageID, reader)
}

func (s *Store) Delete(messageID ...imap.InternalMessageID) error {
	for _, id := range messageID {
		s.forget(id)
	}
	return s.inner.Delete(messageID...)
}

func (s *Store) Close() error                            { return s.inner.Close() }
func (s *Store) List() ([]imap.InternalMessageID, error) { return s.inner.List() }

func (s *Store) isResolved(messageID imap.InternalMessageID) bool {
	s.resolvedMutex.Lock()
	defer s.resolvedMutex.Unlock()
	return s.resolved[messageID]
}

func (s *Store) markResolved(messageID imap.InternalMessageID) {
	s.resolvedMutex.Lock()
	defer s.resolvedMutex.Unlock()
	s.resolved[messageID] = true
}

func (s *Store) forget(messageID imap.InternalMessageID) {
	s.resolvedMutex.Lock()
	defer s.resolvedMutex.Unlock()
	delete(s.resolved, messageID)
}

// emailIDOf reads the account's own ID for a message out of its Message-ID,
// which internal/mail builds from it. A message without one is not ours to
// complete — a draft a client just appended, for instance.
func emailIDOf(literal []byte, messageIDDomain string) (string, bool) {
	value, err := rfc822.GetHeaderValue(literal, messageIDHeader)
	if err != nil || value == "" {
		return "", false
	}

	id := strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(value), ">"), "<")
	address, domain, found := strings.Cut(id, "@")
	if !found || address == "" || domain != messageIDDomain {
		return "", false
	}
	return address, true
}

// carryInternalID copies onto the rebuilt message the header Gluon stamps on
// what it stores.
func carryInternalID(stored, rebuilt []byte) ([]byte, error) {
	internalID, err := rfc822.GetHeaderValue(stored, internalIDKey)
	if err != nil || internalID == "" {
		return rebuilt, nil
	}
	return rfc822.SetHeaderValue(rebuilt, internalIDKey, internalID)
}
