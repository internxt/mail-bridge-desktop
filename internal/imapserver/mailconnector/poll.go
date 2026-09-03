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
		stop:     stop,
		done:     make(chan struct{}),
	}

	go p.run(pollCtx)
	return p
}

func (p *Poller) run(ctx context.Context) {
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

// Stop ends the loop and waits for the cycle in flight to finish.
func (p *Poller) Stop() {
	if p == nil {
		return
	}
	p.stop()
	<-p.done
}
