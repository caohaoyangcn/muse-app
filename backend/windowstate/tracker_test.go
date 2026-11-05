package windowstate

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeSampler is safe for concurrent use: the poller reads it while the test
// moves the simulated window.
type fakeSampler struct {
	mu        sync.Mutex
	placement Placement
	ok        bool
}

func (f *fakeSampler) Current() (Placement, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.placement, f.ok
}

func (f *fakeSampler) Apply(Placement) error { return nil }

func (f *fakeSampler) set(p Placement) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.placement = p
	f.ok = true
}

// TestTrackerSavesChangesOnce verifies the poller persists a placement, does
// not rewrite an unchanged one every tick, and records a later resize.
func TestTrackerSavesChangesOnce(t *testing.T) {
	sampler := &fakeSampler{
		placement: Placement{Left: 10, Top: 20, Width: 1100, Height: 720},
		ok:        true,
	}
	saved := make(chan Placement, 8)
	tracker := newTracker(sampler, func(p Placement) error {
		saved <- p
		return nil
	}, nil, time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	go tracker.Run(ctx)

	first := waitPlacement(t, saved)
	if first != (Placement{Left: 10, Top: 20, Width: 1100, Height: 720}) {
		t.Fatalf("first save = %+v", first)
	}
	assertNoPlacement(t, saved, 20*time.Millisecond)

	sampler.set(Placement{Left: 10, Top: 20, Width: 1300, Height: 900})
	second := waitPlacement(t, saved)
	if second.Width != 1300 || second.Height != 900 {
		t.Fatalf("second save = %+v", second)
	}

	cancel()
	tracker.Wait()
}

// TestTrackerFlushSavesCurrentOnce verifies Flush records the current geometry
// immediately (the final drag before closing) and stays quiet afterwards.
func TestTrackerFlushSavesCurrentOnce(t *testing.T) {
	sampler := &fakeSampler{
		placement: Placement{Left: 300, Top: 200, Width: 1000, Height: 700},
		ok:        true,
	}
	saves := 0
	tracker := newTracker(sampler, func(Placement) error {
		saves++
		return nil
	}, nil, time.Hour)

	tracker.Flush()
	tracker.Flush()
	if saves != 1 {
		t.Errorf("save calls = %d, want 1", saves)
	}
}

// TestTrackerSkipsMissingAndInvalidWindows verifies a destroyed window or a
// nonsensical sample never reaches the settings store.
func TestTrackerSkipsMissingAndInvalidWindows(t *testing.T) {
	sampler := &fakeSampler{}
	saves := 0
	tracker := newTracker(sampler, func(Placement) error {
		saves++
		return nil
	}, nil, time.Hour)
	tracker.Flush()

	sampler.set(Placement{Width: 0, Height: 0})
	tracker.Flush()
	if saves != 0 {
		t.Errorf("save calls = %d, want 0", saves)
	}
}

// TestTrackerKeepsWorkingAfterSaveError verifies one failed write does not stop
// the tracker from trying again once the store recovers.
func TestTrackerKeepsWorkingAfterSaveError(t *testing.T) {
	sampler := &fakeSampler{
		placement: Placement{Left: 1, Top: 2, Width: 900, Height: 650},
		ok:        true,
	}
	attempts := 0
	tracker := newTracker(sampler, func(Placement) error {
		attempts++
		if attempts == 1 {
			return errors.New("database is busy")
		}
		return nil
	}, nil, time.Hour)

	tracker.Flush()
	tracker.Flush()
	if attempts != 2 {
		t.Errorf("save attempts = %d, want 2", attempts)
	}
	if !tracker.hasLast {
		t.Error("a successful retry must be recorded")
	}
}

func waitPlacement(t *testing.T, ch <-chan Placement) Placement {
	t.Helper()
	select {
	case p := <-ch:
		return p
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a saved placement")
		return Placement{}
	}
}

func assertNoPlacement(t *testing.T, ch <-chan Placement, wait time.Duration) {
	t.Helper()
	select {
	case p := <-ch:
		t.Fatalf("unexpected save %+v", p)
	case <-time.After(wait):
	}
}
