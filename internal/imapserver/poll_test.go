package imapserver

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
	p := startPolling(context.Background(), syncer, 10*time.Millisecond, logger.New("test"))
	defer p.stopPolling()

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
	p := startPolling(context.Background(), syncer, 10*time.Millisecond, logger.New("test"))
	defer p.stopPolling()

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
	p := startPolling(context.Background(), syncer, time.Millisecond, logger.New("test"))

	select {
	case <-syncer.started:
	case <-time.After(2 * time.Second):
		t.Fatal("no cycle started")
	}

	stopped := make(chan struct{})
	go func() {
		p.stopPolling()
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
	var p *poller
	p.stopPolling()
}
