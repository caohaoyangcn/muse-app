package main

import (
	"log/slog"
	"sync"
	"time"
)

// slowShutdownStep is the threshold above which a cleanup step is worth
// naming in the log: normal shutdown is milliseconds, so a step this slow is
// already waiting on a disk, an ffmpeg child or a network peer.
const slowShutdownStep = 500 * time.Millisecond

// shutdownStep is one cleanup action with a name for the log. The name is what
// the exit watchdog reports when cleanup is still stuck on it.
type shutdownStep struct {
	name string
	run  func()
}

// shutdownManager runs the application cleanup once, in order. It owns the
// "which step are we in" state so a shutdown that blocks can be attributed
// instead of guessed at.
type shutdownManager struct {
	once  sync.Once
	steps []shutdownStep
	log   *slog.Logger

	mu      sync.Mutex
	current string
}

func newShutdownManager(log *slog.Logger, steps ...shutdownStep) *shutdownManager {
	if log == nil {
		log = slog.Default()
	}
	return &shutdownManager{steps: steps, log: log}
}

// Close runs every step once, in order. It is synchronous: normal shutdown is
// expected to finish. The exit watchdog (armExitWatchdog) bounds the
// pathological case where a step waits on the filesystem or a network peer.
func (m *shutdownManager) Close() {
	m.once.Do(func() {
		for _, step := range m.steps {
			if step.run == nil {
				continue
			}
			m.setCurrent(step.name)
			started := time.Now()
			step.run()
			elapsed := time.Since(started)
			m.setCurrent("")
			if elapsed >= slowShutdownStep {
				m.log.Warn("shutdown step was slow", "step", step.name, "elapsed", elapsed)
			}
		}
	})
}

// CurrentStep names the step currently running, or "" when cleanup is between
// steps or already finished. Safe to call from another goroutine.
func (m *shutdownManager) CurrentStep() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

func (m *shutdownManager) setCurrent(name string) {
	m.mu.Lock()
	m.current = name
	m.mu.Unlock()
}

// armExitWatchdog guarantees the process ends after the window is closed, even
// when a cleanup step never returns. Closing the window must always end Muse:
// queued thumbnails, fingerprints and scrapes are persisted (or re-derivable)
// and resume on the next launch, so an invisible process waiting for them is
// never the right trade. exit is injected so the timeout is testable without
// terminating the test binary.
func armExitWatchdog(after time.Duration, log *slog.Logger, current func() string, exit func(int)) {
	time.AfterFunc(after, func() {
		step := ""
		if current != nil {
			step = current()
		}
		if log != nil {
			log.Error("shutdown deadline exceeded; forcing exit", "deadline", after, "step", step)
		}
		if exit != nil {
			exit(0)
		}
	})
}
