package windowstate

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Sampler reads and restores the application's main window. Current reports
// false while the window does not exist (before Wails creates it, or once it is
// being destroyed); Apply is only called at startup, before the window is first
// shown.
type Sampler interface {
	Current() (Placement, bool)
	Apply(Placement) error
}

// defaultPollInterval is frequent enough that a drag or resize survives a crash
// but rare enough that saving costs nothing noticeable. The tracker only writes
// when the placement actually changed.
const defaultPollInterval = time.Second

// Tracker polls the window and persists every placement change, so an abrupt
// exit still leaves the last geometry behind. Flush covers the final drag before
// a normal close, which the poll may not have caught yet.
type Tracker struct {
	sampler  Sampler
	save     func(Placement) error
	log      *slog.Logger
	interval time.Duration

	mu      sync.Mutex
	last    Placement
	hasLast bool

	done chan struct{}
}

// NewTracker returns a tracker that persists placements through save.
func NewTracker(sampler Sampler, save func(Placement) error, log *slog.Logger) *Tracker {
	return newTracker(sampler, save, log, defaultPollInterval)
}

func newTracker(sampler Sampler, save func(Placement) error, log *slog.Logger, interval time.Duration) *Tracker {
	return &Tracker{
		sampler:  sampler,
		save:     save,
		log:      log,
		interval: interval,
		done:     make(chan struct{}),
	}
}

// Run polls until ctx is cancelled. Callers should pair it with Wait so no
// settings write outlives the database.
func (t *Tracker) Run(ctx context.Context) {
	defer close(t.done)
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.capture()
		}
	}
}

// Wait blocks until Run returns.
func (t *Tracker) Wait() {
	<-t.done
}

// Flush records the current placement immediately. Call it while the window
// still exists (Wails' OnBeforeClose hook), because a closed window reports no
// placement at all.
func (t *Tracker) Flush() {
	t.capture()
}

func (t *Tracker) capture() {
	t.mu.Lock()
	defer t.mu.Unlock()

	placement, ok := t.sampler.Current()
	if !ok || !placement.Valid() {
		return
	}
	if t.hasLast && t.last == placement {
		return
	}
	if err := t.save(placement); err != nil {
		if t.log != nil {
			t.log.Warn("save window placement failed", "err", err)
		}
		return
	}
	t.last, t.hasLast = placement, true
}
