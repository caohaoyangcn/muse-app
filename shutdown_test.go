package main

import (
	"bytes"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestShutdownManagerRunsCleanupOnceInOrder(t *testing.T) {
	var calls []string
	m := newShutdownManager(nil,
		shutdownStep{"watch", func() { calls = append(calls, "watch") }},
		shutdownStep{"worker", func() { calls = append(calls, "worker") }},
		shutdownStep{"database", func() { calls = append(calls, "database") }},
	)

	m.Close()
	m.Close()

	if want := []string{"watch", "worker", "database"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("cleanup calls = %v, want %v", calls, want)
	}
}

// TestExitWatchdogForcesExitWhileStepBlocks models the reported bug: the
// window is gone, but a shutdown step is waiting on background work and the
// process would otherwise stay in the task manager indefinitely.
func TestExitWatchdogForcesExitWhileStepBlocks(t *testing.T) {
	var logged bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logged, nil))
	block := make(chan struct{})
	defer close(block)

	m := newShutdownManager(log, shutdownStep{"jav scraper", func() { <-block }})
	go m.Close()
	waitForCurrentStep(t, m, "jav scraper")

	exited := make(chan int, 1)
	armExitWatchdog(30*time.Millisecond, log, m.CurrentStep, func(code int) {
		exited <- code
	})

	select {
	case code := <-exited:
		if code != 0 {
			t.Fatalf("forced exit code = %d, want 0", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not force an exit for a blocked shutdown step")
	}

	if got := logged.String(); !strings.Contains(got, "jav scraper") {
		t.Fatalf("watchdog log does not name the blocked step: %s", got)
	}
}

func waitForCurrentStep(t *testing.T, m *shutdownManager, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if m.CurrentStep() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("shutdown step %q never became current", want)
}
