package mailconnector

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"mail-bridge-desktop/internal/logger"
)

// countingSyncer records how often it ran and can be held mid-cycle, so a test
// can close the server while a sync is in flight.
type countingSyncer struct {
	mutex   sync.Mutex
	runs    int
	err     error
	release chan struct{}
	started chan struct{}
}

func (s *countingSyncer) Sync(ctx context.Context) error {
	s.mutex.Lock()
	s.runs++
	s.mutex.Unlock()

	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		default:
		}
	}
	if s.release != nil {
		<-s.release
	}
	return s.err
}

func (s *countingSyncer) count() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.runs
}

func TestPollerRunsOnItsInterval(t *testing.T) {
	syncer := &countingSyncer{}
	p := StartPolling(context.Background(), syncer, 10*time.Millisecond, logger.New("test"))
	defer p.Stop()

	deadline := time.After(2 * time.Second)
	for syncer.count() < 3 {
		select {
		case <-deadline:
			t.Fatalf("only %d cycles ran, want at least 3", syncer.count())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestPollerKeepsGoingAfterAFailure covers a cycle that could not reach the
// API: the mailbox a client has stays as it was, and the next cycle tries
// again.
func TestPollerKeepsGoingAfterAFailure(t *testing.T) {
	syncer := &countingSyncer{err: errors.New("api is down")}
	p := StartPolling(context.Background(), syncer, 10*time.Millisecond, logger.New("test"))
	defer p.Stop()

	deadline := time.After(2 * time.Second)
	for syncer.count() < 2 {
		select {
		case <-deadline:
			t.Fatal("the poller gave up after a failed cycle")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestStopPollingWaitsForTheCycleInFlight is the guarantee that keeps shutdown
// from panicking: a cycle writes to the connector's updates channel, and
// closing the server closes that channel, so stopping has to mean the cycle has
// finished rather than merely been told to.
func TestStopPollingWaitsForTheCycleInFlight(t *testing.T) {
	syncer := &countingSyncer{
		release: make(chan struct{}),
		started: make(chan struct{}, 1),
	}
	p := StartPolling(context.Background(), syncer, time.Millisecond, logger.New("test"))

	select {
	case <-syncer.started:
	case <-time.After(2 * time.Second):
		t.Fatal("no cycle started")
	}

	stopped := make(chan struct{})
	go func() {
		p.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("stopPolling returned while a cycle was still running")
	case <-time.After(50 * time.Millisecond):
	}

	close(syncer.release)

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stopPolling did not return once the cycle finished")
	}
}

// TestStopPollingOnNothing keeps Close simple: a server with polling switched
// off has no poller, and stopping it is not a special case.
func TestStopPollingOnNothing(t *testing.T) {
	var p *Poller
	p.Stop()
}

// TestResyncSyncsWithoutWaitingForTheInterval is the point of Resync: an
// interval long enough that no tick can fire during the test, so the only
// thing that could have run the cycle is the request itself.
func TestResyncSyncsWithoutWaitingForTheInterval(t *testing.T) {
	syncer := &countingSyncer{}
	p := StartPolling(context.Background(), syncer, time.Hour, logger.New("test"))
	defer p.Stop()

	p.Resync()

	deadline := time.After(2 * time.Second)
	for syncer.count() < 1 {
		select {
		case <-deadline:
			t.Fatal("the request did not run a cycle")
		case <-time.After(time.Millisecond):
		}
	}
}

// TestResyncCollapsesABurst covers the buffered channel: requests arriving
// while a cycle is in flight must not queue up one cycle each, or a burst
// would leave the poller syncing long after the burst ended.
func TestResyncCollapsesABurst(t *testing.T) {
	syncer := &countingSyncer{
		release: make(chan struct{}),
		started: make(chan struct{}, 1),
	}
	p := StartPolling(context.Background(), syncer, time.Hour, logger.New("test"))
	defer p.Stop()

	p.Resync()
	select {
	case <-syncer.started:
	case <-time.After(2 * time.Second):
		t.Fatal("no cycle started")
	}

	// Held mid-cycle, so every one of these lands while a sync is running.
	for i := 0; i < 10; i++ {
		p.Resync()
	}
	close(syncer.release)

	select {
	case <-syncer.started:
	case <-time.After(2 * time.Second):
		t.Fatal("the pending request did not run")
	}

	// One cycle for the first request, one for the whole burst behind it.
	if got := syncer.count(); got > 2 {
		t.Errorf("ran %d cycles, want at most 2", got)
	}
}

// TestResyncRestartsTheInterval is what "start the 30 seconds over" means: a
// cycle that runs on request pushes the next scheduled one a full interval
// away, rather than leaving whatever was left of the current one.
func TestResyncRestartsTheInterval(t *testing.T) {
	syncer := &countingSyncer{}
	interval := 300 * time.Millisecond

	p := StartPolling(context.Background(), syncer, interval, logger.New("test"))
	defer p.Stop()

	// Most of the way through the interval, then ask for a sync.
	time.Sleep(interval - 50*time.Millisecond)
	p.Resync()

	// Had the ticker kept running, its tick would land within this window.
	time.Sleep(100 * time.Millisecond)

	if got := syncer.count(); got != 1 {
		t.Errorf("ran %d cycles, want 1: the interval did not restart", got)
	}
}

// TestResyncOnNothing mirrors TestStopPollingOnNothing: a server without
// polling still answers the parent's request, by doing nothing.
func TestResyncOnNothing(t *testing.T) {
	var p *Poller
	p.Resync()
}
