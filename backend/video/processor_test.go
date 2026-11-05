package video

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestProcessorRetriesAfterThumbnailFailure(t *testing.T) {
	repo := newTestRepo(t)
	videoPath := filepath.Join(t.TempDir(), "movie.mp4")
	if err := os.WriteFile(videoPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := sampleVideo(videoPath)
	if err := repo.Create(v); err != nil {
		t.Fatal(err)
	}

	thumbDir := t.TempDir()
	attempts := 0
	p := &Processor{
		ctx:      context.Background(),
		repo:     repo,
		thumbDir: thumbDir,
		log:      discardLogger(),
		sleep:    func(time.Duration) {},
		generateThumbnail: func(_ context.Context, _, _ string, _ int64, dir string, id int64) (string, error) {
			attempts++
			if attempts == 1 {
				return "", errors.New("ffmpeg: Permission denied")
			}
			name := strconv.FormatInt(id, 10) + ".jpg"
			if err := os.WriteFile(filepath.Join(dir, name), []byte("jpeg"), 0o644); err != nil {
				return "", err
			}
			return "thumbnails/" + name, nil
		},
	}
	p.processThumbnail(v, 2000)

	got, err := repo.Get(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("thumbnail attempts = %d, want 2", attempts)
	}
	if got.ThumbnailPath != "thumbnails/"+strconv.FormatInt(v.ID, 10)+".jpg" {
		t.Fatalf("thumbnail path = %q, want generated thumbnail", got.ThumbnailPath)
	}
	if _, err := os.Stat(filepath.Join(thumbDir, filepath.Base(got.ThumbnailPath))); err != nil {
		t.Fatalf("generated thumbnail missing: %v", err)
	}
}

// TestProcessorCloseCancelsInFlightThumbnail covers the shutdown path that
// used to leave Muse.exe alive: a probe/ffmpeg waiting on a bad file is now
// canceled through the processor context, so Close returns instead of waiting
// for the extraction and its retries.
func TestProcessorCloseCancelsInFlightThumbnail(t *testing.T) {
	repo := newTestRepo(t)
	v := createTestVideo(t, repo)

	started := make(chan struct{})
	canceled := make(chan struct{})
	p := newRunningProcessor(t, repo, func(ctx context.Context, _, _ string, _ int64, _ string, _ int64) (string, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return "", ctx.Err()
	})
	p.Enqueue(v.ID)
	<-started

	closed := make(chan struct{})
	go func() {
		p.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close waited for the in-flight thumbnail instead of canceling it")
	}
	select {
	case <-canceled:
	default:
		t.Fatal("in-flight thumbnail did not observe the shutdown cancellation")
	}
}

// TestProcessorCloseDoesNotDrainQueuedThumbnails: only the job already running
// may start; every queued id is abandoned and re-enqueued by the startup
// backfill. Draining a large queue was what kept the process alive after its
// window had already disappeared.
func TestProcessorCloseDoesNotDrainQueuedThumbnails(t *testing.T) {
	repo := newTestRepo(t)
	v := createTestVideo(t, repo)

	var calls atomic.Int32
	first := make(chan struct{})
	p := newRunningProcessor(t, repo, func(ctx context.Context, _, _ string, _ int64, _ string, _ int64) (string, error) {
		if calls.Add(1) == 1 {
			close(first)
		}
		<-ctx.Done()
		return "", ctx.Err()
	})
	p.Enqueue(v.ID)
	<-first
	for i := 0; i < 20; i++ {
		p.Enqueue(v.ID)
	}
	p.Close()

	if got := calls.Load(); got != 1 {
		t.Fatalf("thumbnail extractions = %d, want 1 (queued jobs must not drain)", got)
	}
}

func newRunningProcessor(t *testing.T, repo VideoRepository, generate func(context.Context, string, string, int64, string, int64) (string, error)) *Processor {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	p := &Processor{
		jobs:              make(chan int64, 512),
		ctx:               ctx,
		cancel:            cancel,
		repo:              repo,
		thumbDir:          t.TempDir(),
		log:               discardLogger(),
		sleep:             func(time.Duration) {},
		generateThumbnail: generate,
	}
	p.wg.Add(1)
	go p.worker()
	return p
}

func createTestVideo(t *testing.T, repo VideoRepository) *Video {
	t.Helper()
	videoPath := filepath.Join(t.TempDir(), "movie.mp4")
	if err := os.WriteFile(videoPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := sampleVideo(videoPath)
	if err := repo.Create(v); err != nil {
		t.Fatal(err)
	}
	return v
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
