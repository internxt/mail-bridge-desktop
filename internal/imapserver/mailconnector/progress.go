package mailconnector

import (
	"sync"
	"time"
)

const progressInterval = 250 * time.Millisecond

type progressReporter struct {
	events   SyncEvents
	total    int
	interval time.Duration

	mutex      sync.Mutex
	done       int
	lastReport time.Time
}

// newProgressReporter returns nil when there is nothing to report: no listener,
// or no new mail. A nil reporter's methods do nothing, which is what keeps a
// quiet poll quiet without the caller having to check.
func newProgressReporter(events SyncEvents, total int) *progressReporter {
	if events.OnProgress == nil || total <= 0 {
		return nil
	}

	if events.OnStarted != nil {
		events.OnStarted(total)
	}

	return &progressReporter{events: events, total: total, interval: progressInterval}
}

// advance records one downloaded body, reporting it when due. The last one
// always reports, so the parent is never left short of 100%.
//
// Reporting happens under the lock so reports reach the parent in order: a
// stale number arriving after the final one would leave a finished sync
// showing a part-filled bar. It costs a socket write while the other fetch
// goroutines wait, which the throttle keeps rare.
func (p *progressReporter) advance() {
	if p == nil {
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.done++
	now := time.Now()
	if p.done != p.total && now.Sub(p.lastReport) < p.interval {
		return
	}
	p.lastReport = now

	p.events.OnProgress(p.done, p.total, percentOf(p.done, p.total))
}

// finish closes the sync the parent has been watching, saying how far it got
// and why it stopped if it stopped early. It is what keeps a bar from sitting
// part-filled after a sync gave up.
//
// code is empty when the sync did all the work it set out to do.
func (p *progressReporter) finish(code string) {
	if p == nil || p.events.OnFinished == nil {
		return
	}

	p.mutex.Lock()
	done := p.done
	p.mutex.Unlock()

	p.events.OnFinished(done, p.total, code)
}

func percentOf(done, total int) int {
	if total <= 0 {
		return 100
	}
	return done * 100 / total
}
