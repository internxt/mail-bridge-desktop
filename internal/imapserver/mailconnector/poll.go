package mailconnector

import (
	"context"
	"time"

	"mail-bridge-desktop/internal/logger"
)

// DefaultPollInterval is how often the account is checked for changes when a
// caller does not say.
const DefaultPollInterval = 30 * time.Second

// StartPolling runs sync on a timer until Stop is called.
func StartPolling(ctx context.Context, sync Synchronizer, interval time.Duration, log *logger.Logger) *Poller {
	pollCtx, stop := context.WithCancel(context.WithoutCancel(ctx))

	p := &Poller{
		sync:     sync,
		interval: interval,
		log:      log,

		reset: make(chan struct{}, 1),
		stop:  stop,
		done:  make(chan struct{}),
	}

	go p.run(pollCtx)
	return p
}

// Resync syncs now and restarts the interval, so a caller that knows the
// account changed does not wait out the rest of the cycle.
//
// It never blocks, and a request arriving while a sync runs waits its turn
// rather than starting a second one. Requests that pile up behind it are
// dropped: one sync leaves the account up to date for all of them.
func (p *Poller) Resync() {
	if p == nil {
		return
	}

	select {
	case p.reset <- struct{}{}:
	default:
	}
}

func (p *Poller) run(ctx context.Context) {
	defer close(p.done)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-p.reset:
			p.syncOnce(withRequestedSync(ctx))
			ticker.Reset(p.interval)

		case <-ticker.C:
			p.syncOnce(ctx)
		}
	}
}

func (p *Poller) syncOnce(ctx context.Context) {
	if err := p.sync.Sync(ctx); err != nil {
		p.log.Warn("could not refresh the mailbox: %v", err)
	}
}

// Stop ends the loop and waits for the cycle in flight to finish.
func (p *Poller) Stop() {
	if p == nil {
		return
	}
	p.stop()
	<-p.done
}
