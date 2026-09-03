package imapserver

import (
	"context"
	"time"

	"mail-bridge-desktop/internal/logger"
)

// DefaultPollInterval is how often the account is checked for changes when a
// caller does not say.
const DefaultPollInterval = 30 * time.Second

// synchronizer is a connector that can bring itself up to date. The mail
// connector is one; the development fixture is not.
type synchronizer interface {
	Sync(context.Context) error
}

type poller struct {
	sync     synchronizer
	interval time.Duration
	log      *logger.Logger

	stop context.CancelFunc
	done chan struct{}
}

// startPolling runs sync on a timer until stopPolling is called.
func startPolling(ctx context.Context, sync synchronizer, interval time.Duration, log *logger.Logger) *poller {
	pollCtx, stop := context.WithCancel(context.WithoutCancel(ctx))

	p := &poller{
		sync:     sync,
		interval: interval,
		log:      log,
		stop:     stop,
		done:     make(chan struct{}),
	}

	go p.run(pollCtx)
	return p
}

func (p *poller) run(ctx context.Context) {
	defer close(p.done)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.sync.Sync(ctx); err != nil {
				p.log.Warn("could not refresh the mailbox: %v", err)
			}
		}
	}
}

// stopPolling ends the loop and waits for the cycle in flight to finish.
func (p *poller) stopPolling() {
	if p == nil {
		return
	}
	p.stop()
	<-p.done
}
