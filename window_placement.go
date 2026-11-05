package main

import (
	"context"
	"log/slog"
	"sync"

	"muse-app/backend/settings"
	"muse-app/backend/windowstate"
)

// windowPlacement restores the geometry Muse closed with and remembers the next
// one. It sits between the Wails lifecycle callbacks (which run on different
// goroutines) and the settings store, and owns the background sampler.
type windowPlacement struct {
	store *settings.Store
	log   *slog.Logger

	mu      sync.Mutex
	tracker *windowstate.Tracker
	cancel  context.CancelFunc
}

func newWindowPlacement(store *settings.Store, log *slog.Logger) *windowPlacement {
	return &windowPlacement{store: store, log: log}
}

// saved reads the placement persisted by a previous run. A read error is logged
// and treated as "no saved placement" so a settings problem never blocks startup.
func (w *windowPlacement) saved() (windowstate.Placement, bool) {
	placement, ok, err := w.store.WindowPlacement()
	if err != nil {
		w.log.Warn("read window placement failed", "err", err)
		return windowstate.Placement{}, false
	}
	return placement, ok
}

// start applies the saved placement (when restore is true) and begins recording
// changes. Call it at the very beginning of OnStartup: the window has been
// created but Wails has not shown it yet, so the default centered geometry never
// reaches the screen.
func (w *windowPlacement) start(ctx context.Context, placement windowstate.Placement, restore bool) {
	sampler := windowstate.NewSampler(ctx, minWindowWidth, minWindowHeight)
	if restore {
		if err := sampler.Apply(placement); err != nil {
			w.log.Warn("restore window placement failed", "err", err)
		}
	}

	tracker := windowstate.NewTracker(sampler, w.store.SetWindowPlacement, w.log)
	trackerCtx, cancel := context.WithCancel(context.Background())
	w.mu.Lock()
	w.tracker, w.cancel = tracker, cancel
	w.mu.Unlock()
	go tracker.Run(trackerCtx)
}

// flush writes the final geometry. Wails calls OnBeforeClose while the window
// still exists, so this is the last chance to notice a resize the poll missed.
func (w *windowPlacement) flush() {
	w.mu.Lock()
	tracker := w.tracker
	w.mu.Unlock()
	if tracker != nil {
		tracker.Flush()
	}
}

// stop ends the poller and waits for it, so the last settings write cannot race
// the shutdown of the database.
func (w *windowPlacement) stop() {
	w.mu.Lock()
	tracker, cancel := w.tracker, w.cancel
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if tracker != nil {
		tracker.Wait()
	}
}
