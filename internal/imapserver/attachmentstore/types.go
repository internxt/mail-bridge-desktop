package attachmentstore

import (
	"context"
	"sync"

	"github.com/ProtonMail/gluon/imap"
	"github.com/ProtonMail/gluon/store"

	"mail-bridge-desktop/internal/logger"
)

type Resolver interface {
	ResolveAttachments(ctx context.Context, emailID string, literal []byte) ([]byte, error)
}

type Store struct {
	inner           store.Store
	resolver        Resolver
	messageIDDomain string
	log             *logger.Logger

	resolvedMutex sync.Mutex
	resolved      map[imap.InternalMessageID]bool
}

type Builder struct {
	inner           store.Builder
	resolver        Resolver
	messageIDDomain string
	log             *logger.Logger
}
