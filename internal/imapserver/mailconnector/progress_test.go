package mailconnector

import (
	"sync"
	"testing"
	"time"
)

// recorder collects what a reporter reported, from whichever goroutine
// reported it.
type recorder struct {
	mutex    sync.Mutex
	starts   []int
	reports  [][3]int
	finishes []finish
}

// finish is one OnFinished call, kept so a test can assert the sync was closed
// and with what.
type finish struct {
	done  int
	total int
	code  string
}

func (r *recorder) recordStart(total int) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.starts = append(r.starts, total)
}

func (r *recorder) allStarts() []int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([]int(nil), r.starts...)
}

func (r *recorder) record(done, total, percent int) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.reports = append(r.reports, [3]int{done, total, percent})
}

func (r *recorder) recordFinish(done, total int, code string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.finishes = append(r.finishes, finish{done: done, total: total, code: code})
}

func (r *recorder) all() [][3]int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([][3]int(nil), r.reports...)
}

func (r *recorder) allFinishes() []finish {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([]finish(nil), r.finishes...)
}

func (r *recorder) events() SyncEvents {
	return SyncEvents{
		OnStarted:  r.recordStart,
		OnProgress: r.record,
		OnFinished: r.recordFinish,
	}
}

// TestNewProgressReporterOnNothingToReport is what keeps a poll that finds no
// new mail from telling the parent anything at all.
func TestNewProgressReporterOnNothingToReport(t *testing.T) {
	for _, tc := range []struct {
		name   string
		report func(done, total, percent int)
		total  int
	}{
		{"no new mail", func(int, int, int) {}, 0},
		{"a negative total", func(int, int, int) {}, -1},
		{"nobody listening", nil, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newProgressReporter(SyncEvents{OnProgress: tc.report}, tc.total)
			if p != nil {
				t.Fatalf("got %v, want nil", p)
			}
			p.advance() // must not panic
		})
	}
}

func TestProgressReporterReportsTheFirstAdvance(t *testing.T) {
	var recorded recorder

	p := newProgressReporter(SyncEvents{OnProgress: recorded.record}, 10)
	p.advance()

	reports := recorded.all()
	if len(reports) != 1 {
		t.Fatalf("got %d reports, want 1", len(reports))
	}
	if want := [3]int{1, 10, 10}; reports[0] != want {
		t.Errorf("got %v, want %v", reports[0], want)
	}
}

// TestProgressReporterThrottles is the 250ms rule: a burst of completions has
// to collapse into one report, or a big sync floods the control channel.
func TestProgressReporterThrottles(t *testing.T) {
	var recorded recorder

	p := newProgressReporter(SyncEvents{OnProgress: recorded.record}, 100)
	for i := 0; i < 50; i++ {
		p.advance()
	}

	// The first advance reports, and the other 49 land inside the interval.
	if got := len(recorded.all()); got != 1 {
		t.Fatalf("got %d reports, want 1", got)
	}

	// A short interval checks the next report without sleeping a quarter of
	// a second for it.
	p.interval = time.Millisecond
	time.Sleep(2 * time.Millisecond)
	p.advance()

	if got := len(recorded.all()); got != 2 {
		t.Errorf("got %d reports, want 2 once the interval had passed", got)
	}
}

// TestProgressReporterAlwaysReportsTheLast guards the end of the bar: the
// throttle must never swallow the report that says the sync is done.
func TestProgressReporterAlwaysReportsTheLast(t *testing.T) {
	var recorded recorder

	p := newProgressReporter(SyncEvents{OnProgress: recorded.record}, 4)
	for i := 0; i < 4; i++ {
		p.advance()
	}

	reports := recorded.all()
	last := reports[len(reports)-1]
	if want := [3]int{4, 4, 100}; last != want {
		t.Errorf("last report is %v, want %v", last, want)
	}
}

// TestProgressReporterUnderConcurrentAdvances mirrors the real caller: the
// body fetch runs fetchBodyConcurrency goroutines, all advancing at once.
func TestProgressReporterUnderConcurrentAdvances(t *testing.T) {
	var recorded recorder
	const (
		perWorker = 64
		total     = fetchBodyConcurrency * perWorker
	)

	p := newProgressReporter(SyncEvents{OnProgress: recorded.record}, total)

	var workers sync.WaitGroup
	for i := 0; i < fetchBodyConcurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < perWorker; j++ {
				p.advance()
			}
		}()
	}
	workers.Wait()

	reports := recorded.all()
	if len(reports) == 0 {
		t.Fatal("no reports at all")
	}

	// Reports have to arrive in order. Out of order, the parent would be left
	// showing whichever landed last rather than the furthest along.
	previous := 0
	for _, report := range reports {
		if report[0] > total {
			t.Fatalf("reported %d of %d: counted past the total", report[0], total)
		}
		if report[0] <= previous {
			t.Fatalf("reported %d after %d: progress went backwards", report[0], previous)
		}
		previous = report[0]
	}

	if want := [3]int{total, total, 100}; reports[len(reports)-1] != want {
		t.Errorf("last report is %v, want %v", reports[len(reports)-1], want)
	}
}

// TestProgressReporterStartsBeforeTheFirstDownload is what lets the parent
// draw an empty bar: the total is named when the sync opens, not once the
// first body has arrived.
func TestProgressReporterStartsBeforeTheFirstDownload(t *testing.T) {
	var recorded recorder

	p := newProgressReporter(recorded.events(), 12)

	starts := recorded.allStarts()
	if len(starts) != 1 {
		t.Fatalf("got %d starts, want 1", len(starts))
	}
	if starts[0] != 12 {
		t.Errorf("started with a total of %d, want 12", starts[0])
	}
	if got := len(recorded.all()); got != 0 {
		t.Errorf("reported %d times before the first download, want none", got)
	}

	p.advance()
	p.finish("")
}

// TestProgressReporterOnNothingToReportStaysSilent covers all three events at
// once: no new mail means no start, no progress and no finish.
func TestProgressReporterOnNothingToReportStaysSilent(t *testing.T) {
	var recorded recorder

	p := newProgressReporter(recorded.events(), 0)
	p.advance()
	p.finish("")

	if got := len(recorded.allStarts()); got != 0 {
		t.Errorf("got %d starts, want none", got)
	}
	if got := len(recorded.all()); got != 0 {
		t.Errorf("got %d reports, want none", got)
	}
	if got := len(recorded.allFinishes()); got != 0 {
		t.Errorf("got %d finishes, want none", got)
	}
}

// TestProgressReporterFinishReportsHowFarItGot is the answer to "how does the
// parent know it is over": a sync that gave up halfway still closes, with the
// count it reached and why it stopped.
func TestProgressReporterFinishReportsHowFarItGot(t *testing.T) {
	var recorded recorder

	p := newProgressReporter(recorded.events(), 10)
	p.advance()
	p.advance()
	p.finish("fetch_bodies")

	finishes := recorded.allFinishes()
	if len(finishes) != 1 {
		t.Fatalf("got %d finishes, want 1", len(finishes))
	}
	if want := (finish{done: 2, total: 10, code: "fetch_bodies"}); finishes[0] != want {
		t.Errorf("got %+v, want %+v", finishes[0], want)
	}
}

func TestProgressReporterFinishOnSuccessCarriesNoCode(t *testing.T) {
	var recorded recorder

	p := newProgressReporter(recorded.events(), 2)
	p.advance()
	p.advance()
	p.finish("")

	finishes := recorded.allFinishes()
	if len(finishes) != 1 {
		t.Fatalf("got %d finishes, want 1", len(finishes))
	}
	if want := (finish{done: 2, total: 2}); finishes[0] != want {
		t.Errorf("got %+v, want %+v", finishes[0], want)
	}
}

func TestPercentOf(t *testing.T) {
	for _, tc := range []struct {
		done  int
		total int
		want  int
	}{
		{0, 10, 0},
		{1, 3, 33},
		{2, 3, 66},
		{999, 1000, 99}, // never 100 before the work is done
		{1000, 1000, 100},
		{1, 0, 100}, // no total: nothing left to do
	} {
		if got := percentOf(tc.done, tc.total); got != tc.want {
			t.Errorf("percentOf(%d, %d) = %d, want %d", tc.done, tc.total, got, tc.want)
		}
	}
}
