package video

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// 精彩时刻 service tests. They drive the real SQLite repository and replace only
// the two ffmpeg seams, so what they pin down is the queue's behaviour around a
// generator: ordering, the counters, cancellation and recovery.

// eventRecorder collects the event names a service emits. The worker emits from
// its own goroutine, so this is the one shared mutable the tests guard.
type eventRecorder struct {
	mu    sync.Mutex
	names []string
}

func (r *eventRecorder) emit(name string, _ ...any) {
	r.mu.Lock()
	r.names = append(r.names, name)
	r.mu.Unlock()
}

func (r *eventRecorder) has(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, got := range r.names {
		if got == name {
			return true
		}
	}
	return false
}

// newHighlightFixture gives a service with one favorite whose file exists and
// whose duration is known, so the time validation has real numbers to check.
func newHighlightFixture(t *testing.T) (*Service, string, *Video) {
	t.Helper()
	svc, dir := newTestService(t)
	t.Cleanup(svc.CloseHighlights)
	p := writeFakeVideo(t, dir, "moment.mp4", 100)
	v, err := svc.AddVideo(p)
	if err != nil {
		t.Fatalf("add video: %v", err)
	}
	if err := svc.repo.UpdateMetadata(v.ID, 120_000, 1920, 1080, 30, 4_000_000, "h264", "aac"); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	v, err = svc.repo.Get(v.ID)
	if err != nil {
		t.Fatalf("re-read video: %v", err)
	}
	return svc, dir, v
}

// writeHighlightFile is the fake generator's output: a file with the given body.
func writeHighlightFile(outPath string, body string) error {
	return os.WriteFile(outPath, []byte(body), 0o644)
}

func waitForHighlight(t *testing.T, svc *Service, videoID, id int64, want string) *Highlight {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		list, err := svc.ListHighlights(videoID)
		if err != nil {
			t.Fatalf("list highlights: %v", err)
		}
		for i := range list {
			if list[i].ID == id && list[i].Status == want {
				return &list[i]
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("highlight %d did not reach %q: %+v", id, want, list)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForHighlightTask(t *testing.T, svc *Service, want func(HighlightTaskStatus) bool) HighlightTaskStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		st := svc.HighlightTaskStatus()
		if want(st) {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("task status did not settle: %+v", st)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAddHighlightValidation(t *testing.T) {
	svc, _, v := newHighlightFixture(t)

	cases := []struct {
		name       string
		start, end int64
		want       string
	}{
		{"negative start", -1, 0, "开始时间无效"},
		{"start at the duration", 120_000, 0, "开始时间超出视频时长"},
		{"end before start", 5_000, 4_000, "结束时间必须晚于开始时间"},
		{"end at start", 5_000, 5_000, "结束时间必须晚于开始时间"},
		{"range under a second", 5_000, 5_500, "时间段至少 1 秒"},
		{"range over ten seconds", 5_000, 16_000, "时间段最长 10 秒（当前 11 秒）"},
		{"end past the duration", 118_000, 121_000, "结束时间超出视频时长"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.AddHighlight(v.ID, tc.start, tc.end)
			if err == nil {
				t.Fatalf("AddHighlight(%d, %d) succeeded, want %q", tc.start, tc.end, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want %q", err, tc.want)
			}
		})
	}

	// Nothing above may have been written: a rejected request is not a row.
	if list, err := svc.ListHighlights(v.ID); err != nil || len(list) != 0 {
		t.Errorf("highlights after rejected requests = %+v (err %v), want none", list, err)
	}

	// The boundaries themselves are valid: a moment at 0 and a ten-second range.
	if _, err := svc.AddHighlight(v.ID, 0, 0); err != nil {
		t.Errorf("moment at 0: %v", err)
	}
	if _, err := svc.AddHighlight(v.ID, 30_000, 40_000); err != nil {
		t.Errorf("ten-second range: %v", err)
	}
	// An unknown duration skips the duration checks instead of guessing.
	short, err := svc.repo.Get(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	short.DurationMs = 0
	if err := ValidateHighlightTimes(500_000, 0, 0); err != nil {
		t.Errorf("unknown duration: %v", err)
	}
}

func TestAddHighlightRejectsUnreadableSource(t *testing.T) {
	svc, dir, v := newHighlightFixture(t)
	if _, err := svc.AddHighlight(v.ID, 1000, 0); err != nil {
		t.Fatalf("add with a present file: %v", err)
	}

	if err := os.Remove(filepath.Join(dir, "moment.mp4")); err != nil {
		t.Fatal(err)
	}
	_, err := svc.AddHighlight(v.ID, 2000, 0)
	if err == nil || !strings.Contains(err.Error(), highlightErrSourceMissing) {
		t.Errorf("err = %v, want %q", err, highlightErrSourceMissing)
	}
}

func TestAddHighlightQueuesAndReturns(t *testing.T) {
	svc, _, v := newHighlightFixture(t)

	// No worker started: the call must still return the queued row rather than
	// wait for anything.
	h, err := svc.AddHighlight(v.ID, 90_000, 0)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if h.ID == 0 || h.Status != HighlightStatusQueued || h.Kind != HighlightKindMoment {
		t.Fatalf("added = %+v", h)
	}
	if !ValidHighlightFileName(filepath.Base(filepath.FromSlash(h.MediaPath))) {
		t.Errorf("media path %q is not a generated-file name", h.MediaPath)
	}

	list, err := svc.ListHighlights(v.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != h.ID {
		t.Fatalf("list = %+v", list)
	}
	// A queued row is already accounted for, so the dashboard shows the work.
	st := svc.HighlightTaskStatus()
	if !st.Active || st.Total != 1 || st.Pending() != 1 || st.StartedAt == 0 {
		t.Errorf("task status = %+v, want one pending row", st)
	}
}

func TestHighlightWorkerGeneratesMoment(t *testing.T) {
	svc, dir, v := newHighlightFixture(t)
	events := &eventRecorder{}
	svc.emit = events.emit

	var (
		mu      sync.Mutex
		atMs    int64
		outSeen string
	)
	svc.highlights.generateMoment = func(_ context.Context, videoPath string, atMsArg int64, outPath string, _ time.Duration) error {
		mu.Lock()
		atMs, outSeen = atMsArg, outPath
		mu.Unlock()
		return writeHighlightFile(outPath, "jpeg bytes")
	}
	svc.StartHighlights()

	h, err := svc.AddHighlight(v.ID, 90_000, 0)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	ready := waitForHighlight(t, svc, v.ID, h.ID, HighlightStatusReady)

	mu.Lock()
	gotAtMs, gotOut := atMs, outSeen
	mu.Unlock()
	if gotAtMs != 90_000 {
		t.Errorf("generator ran at %d ms, want 90000", gotAtMs)
	}
	if filepath.Base(gotOut) != filepath.Base(filepath.FromSlash(ready.MediaPath)) {
		t.Errorf("generator wrote %q, the row recorded %q", gotOut, ready.MediaPath)
	}
	if _, err := os.Stat(gotOut); err != nil {
		t.Errorf("generated file missing: %v", err)
	}
	if ready.FinishedAt == 0 {
		t.Error("a ready row must record when it finished")
	}
	if !events.has("video:highlight:updated") {
		t.Error("a finished highlight must be announced")
	}

	st := waitForHighlightTask(t, svc, func(st HighlightTaskStatus) bool { return !st.Active })
	if st.Total != 1 || st.Done != 1 || st.Failed != 0 || st.Canceled != 0 || st.Pending() != 0 {
		t.Errorf("task status = %+v, want one done row", st)
	}
	if _, err := os.Stat(filepath.Join(dir, HighlightsRelativeDir)); err != nil {
		t.Errorf("highlight folder missing: %v", err)
	}
}

func TestHighlightWorkerFailureKeepsErrorAndCleansUp(t *testing.T) {
	svc, _, v := newHighlightFixture(t)
	svc.highlights.generateMoment = func(_ context.Context, _ string, _ int64, outPath string, _ time.Duration) error {
		// A generator that dies mid-write: the partial file must not survive.
		if err := writeHighlightFile(outPath, "half"); err != nil {
			return err
		}
		return errors.New("boom")
	}
	svc.StartHighlights()

	h, err := svc.AddHighlight(v.ID, 1000, 0)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	failed := waitForHighlight(t, svc, v.ID, h.ID, HighlightStatusFailed)
	if !strings.Contains(failed.Error, "生成精彩时刻失败：boom") {
		t.Errorf("error = %q, want the ffmpeg reason", failed.Error)
	}
	if _, err := os.Stat(svc.highlightPath(failed)); !os.IsNotExist(err) {
		t.Error("a failed attempt must leave no file behind")
	}

	st := waitForHighlightTask(t, svc, func(st HighlightTaskStatus) bool { return !st.Active })
	if st.Failed != 1 || st.Done != 0 {
		t.Errorf("task status = %+v, want one failed row", st)
	}
}

func TestHighlightWorkerReportsUnavailableSources(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		svc, dir, v := newHighlightFixture(t)
		h, err := svc.AddHighlight(v.ID, 1000, 0)
		if err != nil {
			t.Fatalf("add: %v", err)
		}
		// The file disappears between queuing and generating.
		if err := os.Remove(filepath.Join(dir, "moment.mp4")); err != nil {
			t.Fatal(err)
		}
		svc.StartHighlights()
		failed := waitForHighlight(t, svc, v.ID, h.ID, HighlightStatusFailed)
		if failed.Error != highlightErrSourceMissing {
			t.Errorf("error = %q, want %q", failed.Error, highlightErrSourceMissing)
		}
	})

	t.Run("favorite in the recycle bin", func(t *testing.T) {
		svc, _, v := newHighlightFixture(t)
		h, err := svc.AddHighlight(v.ID, 1000, 0)
		if err != nil {
			t.Fatalf("add: %v", err)
		}
		if err := svc.repo.Trash(v.ID, TrashReasonUser, nowMillis()); err != nil {
			t.Fatalf("trash video: %v", err)
		}
		svc.StartHighlights()
		failed := waitForHighlight(t, svc, v.ID, h.ID, HighlightStatusFailed)
		if failed.Error != highlightErrTrashed {
			t.Errorf("error = %q, want %q", failed.Error, highlightErrTrashed)
		}
	})
}

func TestRetryFailedHighlight(t *testing.T) {
	svc, _, v := newHighlightFixture(t)
	var (
		mu    sync.Mutex
		calls int
	)
	svc.highlights.generateMoment = func(_ context.Context, _ string, _ int64, outPath string, _ time.Duration) error {
		mu.Lock()
		calls++
		attempt := calls
		mu.Unlock()
		if attempt == 1 {
			return errors.New("第一次失败")
		}
		return writeHighlightFile(outPath, "jpeg")
	}
	svc.StartHighlights()

	h, err := svc.AddHighlight(v.ID, 1000, 0)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	waitForHighlight(t, svc, v.ID, h.ID, HighlightStatusFailed)

	// A ready row has nothing to retry; only a failure is user-retryable.
	if err := svc.RetryHighlight(v.ID, h.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	ready := waitForHighlight(t, svc, v.ID, h.ID, HighlightStatusReady)
	if ready.Error != "" {
		t.Errorf("error after retry = %q, want it cleared", ready.Error)
	}
	if !strings.HasSuffix(ready.MediaPath, ".jpg") {
		t.Errorf("media path = %q, want the same file", ready.MediaPath)
	}
	if err := svc.RetryHighlight(v.ID, h.ID); err == nil {
		t.Error("retrying a ready highlight should be refused")
	}
}

func TestDeleteHighlightAccountsForPendingRows(t *testing.T) {
	svc, _, v := newHighlightFixture(t)

	// No worker: the row stays queued.
	h, err := svc.AddHighlight(v.ID, 1000, 0)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := svc.DeleteHighlight(v.ID, h.ID); err != nil {
		t.Fatalf("delete queued: %v", err)
	}
	list, err := svc.ListHighlights(v.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("highlights after delete = %+v, want none", list)
	}
	st := svc.HighlightTaskStatus()
	if st.Total != 1 || st.Done != 1 || st.Canceled != 1 || st.Failed != 0 || st.Active {
		t.Errorf("task status = %+v, want the cancellation counted once", st)
	}

	// Deleting again is not an error: the card was already stale.
	if err := svc.DeleteHighlight(v.ID, h.ID); err != nil {
		t.Errorf("repeated delete: %v", err)
	}
}

func TestDeleteReadyHighlightRemovesFile(t *testing.T) {
	svc, _, v := newHighlightFixture(t)
	svc.highlights.generateMoment = func(_ context.Context, _ string, _ int64, outPath string, _ time.Duration) error {
		return writeHighlightFile(outPath, "jpeg")
	}
	svc.StartHighlights()

	h, err := svc.AddHighlight(v.ID, 1000, 0)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	ready := waitForHighlight(t, svc, v.ID, h.ID, HighlightStatusReady)
	if _, err := os.Stat(svc.highlightPath(ready)); err != nil {
		t.Fatalf("generated file missing: %v", err)
	}

	if err := svc.DeleteHighlight(v.ID, h.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(svc.highlightPath(ready)); !os.IsNotExist(err) {
		t.Error("deleting a highlight must delete its generated file")
	}
	st := svc.HighlightTaskStatus()
	// A ready row is a result, not work in flight: deleting it does not change
	// the round's counters.
	if st.Done != 1 || st.Canceled != 0 || st.Failed != 0 {
		t.Errorf("task status = %+v, want the finished round unchanged", st)
	}
}

// TestDeleteRunningHighlightDiscardsResult is the race the worker's rows-affected
// check exists for: the user removes a row while ffmpeg is already writing it.
func TestDeleteRunningHighlightDiscardsResult(t *testing.T) {
	svc, _, v := newHighlightFixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	svc.highlights.generateMoment = func(ctx context.Context, _ string, _ int64, outPath string, _ time.Duration) error {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		return writeHighlightFile(outPath, "jpeg")
	}
	svc.StartHighlights()

	h, err := svc.AddHighlight(v.ID, 1000, 0)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	<-started
	if err := svc.DeleteHighlight(v.ID, h.ID); err != nil {
		t.Fatalf("delete running: %v", err)
	}
	close(release)

	// The worker finishes, finds no row, and drops the file. Wait for it to go
	// back to idle before asking about the counters.
	st := waitForHighlightTask(t, svc, func(st HighlightTaskStatus) bool {
		return st.Current == "" && !st.Active
	})
	if st.Total != 1 || st.Done != 1 || st.Canceled != 1 || st.Failed != 0 {
		t.Errorf("task status = %+v, want exactly one cancellation", st)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(svc.highlightPath(h)); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the result of a deleted row must not be kept")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestHighlightsSurviveRestart covers the interruption rules: a quit leaves the
// row running, and the next start re-queues it with one interruption recorded.
func TestHighlightsSurviveRestart(t *testing.T) {
	svc, dir, v := newHighlightFixture(t)
	started := make(chan struct{})
	svc.highlights.generateMoment = func(ctx context.Context, _ string, _ int64, _ string, _ time.Duration) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	svc.StartHighlights()

	h, err := svc.AddHighlight(v.ID, 1000, 0)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	<-started
	svc.CloseHighlights()

	list, err := svc.ListHighlights(v.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Status != HighlightStatusRunning {
		t.Fatalf("after quit = %+v, want the row left running", list)
	}
	if list[0].Error != "" {
		t.Errorf("a quit must not report a failure: %q", list[0].Error)
	}

	// A second process over the same database: recovery re-queues the row and
	// the generator finally succeeds.
	restarted := NewService(svc.repo, svc.processor, dir, svc.log, nil)
	t.Cleanup(restarted.CloseHighlights)
	restarted.highlights.generateMoment = func(_ context.Context, _ string, _ int64, outPath string, _ time.Duration) error {
		return writeHighlightFile(outPath, "jpeg")
	}
	restarted.StartHighlights()

	ready := waitForHighlight(t, restarted, v.ID, h.ID, HighlightStatusReady)
	if ready.Interruptions != 1 {
		t.Errorf("interruptions = %d, want 1", ready.Interruptions)
	}
}

// TestHighlightRecoveryGivesUpAfterRepeatedInterruptions: a file that takes the
// app down every time must stop being retried automatically.
func TestHighlightRecoveryGivesUpAfterRepeatedInterruptions(t *testing.T) {
	svc, dir, v := newHighlightFixture(t)
	h := &Highlight{
		VideoID:       v.ID,
		Kind:          HighlightKindMoment,
		StartMs:       1000,
		MediaPath:     highlightMediaPath(v.ID, 1, HighlightKindMoment),
		Status:        HighlightStatusRunning,
		Interruptions: 2,
		CreatedAt:     nowMillis(),
		UpdatedAt:     nowMillis(),
	}
	if err := svc.repo.CreateHighlight(h); err != nil {
		t.Fatalf("create: %v", err)
	}

	restarted := NewService(svc.repo, svc.processor, dir, svc.log, nil)
	t.Cleanup(restarted.CloseHighlights)
	restarted.StartHighlights()

	failed := waitForHighlight(t, restarted, v.ID, h.ID, HighlightStatusFailed)
	if failed.Error != "生成被多次中断" {
		t.Errorf("error = %q, want the interruption message", failed.Error)
	}
	if failed.Interruptions != 3 {
		t.Errorf("interruptions = %d, want 3", failed.Interruptions)
	}
	// The failed row is not part of the round: nothing was queued for it.
	if st := restarted.HighlightTaskStatus(); st.Active {
		t.Errorf("task status = %+v, want an idle round", st)
	}
}

// The frame rate is a product decision the user feels rather than a
// technical one, so it is pinned here. The GIF delay is stored in hundredths of
// a second: the default must divide 100 (or every frame gets rounded), and it
// must stay at or under 50 fps, because a delay below 2 cs is clamped to 10 cs
// by browsers and the loop would play slower than a lower frame rate would.
func TestHighlightGIFFrameRatesAreSmooth(t *testing.T) {
	if highlightGIFDefaultFPS < 20 {
		t.Errorf("default %d fps is too choppy; 20 or more is the floor", highlightGIFDefaultFPS)
	}
	if highlightGIFDefaultFPS > 50 {
		t.Errorf("default %d fps cannot be expressed in GIF centiseconds; "+
			"frames under 2 cs are clamped to 100 ms", highlightGIFDefaultFPS)
	}
	if 100%highlightGIFDefaultFPS != 0 {
		t.Errorf("default %d fps does not divide 100 cs; frame delays would be rounded",
			highlightGIFDefaultFPS)
	}
	if highlightGIFFallbackFPS >= highlightGIFDefaultFPS {
		t.Errorf("fallback %d fps must be below the default %d fps",
			highlightGIFFallbackFPS, highlightGIFDefaultFPS)
	}
	if highlightGIFFallbackFPS < 12 {
		t.Errorf("fallback %d fps is a slideshow; the size retry should not go that low",
			highlightGIFFallbackFPS)
	}
}

// TestHighlightGIFRetriesAtLowerFrameRate pins the 8 MB ceiling: the first file
// over the line is regenerated once at the fallback rate, at the same path.
func TestHighlightGIFRetriesAtLowerFrameRate(t *testing.T) {
	t.Run("second attempt is smaller", func(t *testing.T) {
		svc, _, v := newHighlightFixture(t)
		var (
			mu   sync.Mutex
			fps  []int
			body []int64
		)
		svc.highlights.generateGIF = func(_ context.Context, _ string, _ int64, _ int64, fpsArg int, outPath string, _ time.Duration) error {
			mu.Lock()
			fps = append(fps, fpsArg)
			attempt := len(fps)
			mu.Unlock()
			if err := writeHighlightFile(outPath, "gif"); err != nil {
				return err
			}
			size := int64(highlightGIFMaxBytes + 1)
			if attempt == 2 {
				size = 1024
			}
			if err := os.Truncate(outPath, size); err != nil {
				return err
			}
			mu.Lock()
			body = append(body, size)
			mu.Unlock()
			return nil
		}
		svc.StartHighlights()

		h, err := svc.AddHighlight(v.ID, 1000, 6000)
		if err != nil {
			t.Fatalf("add: %v", err)
		}
		waitForHighlight(t, svc, v.ID, h.ID, HighlightStatusReady)

		mu.Lock()
		defer mu.Unlock()
		if len(fps) != 2 || fps[0] != highlightGIFDefaultFPS || fps[1] != highlightGIFFallbackFPS {
			t.Errorf("frame rates = %v, want [10 8]", fps)
		}
		if len(body) != 2 || body[0] <= highlightGIFMaxBytes || body[1] > highlightGIFMaxBytes {
			t.Errorf("sizes = %v", body)
		}
	})

	t.Run("still too big is accepted", func(t *testing.T) {
		svc, _, v := newHighlightFixture(t)
		var (
			mu  sync.Mutex
			fps []int
		)
		svc.highlights.generateGIF = func(_ context.Context, _ string, _ int64, _ int64, fpsArg int, outPath string, _ time.Duration) error {
			mu.Lock()
			fps = append(fps, fpsArg)
			mu.Unlock()
			if err := writeHighlightFile(outPath, "gif"); err != nil {
				return err
			}
			return os.Truncate(outPath, int64(highlightGIFMaxBytes+1))
		}
		svc.StartHighlights()

		h, err := svc.AddHighlight(v.ID, 1000, 6000)
		if err != nil {
			t.Fatalf("add: %v", err)
		}
		ready := waitForHighlight(t, svc, v.ID, h.ID, HighlightStatusReady)
		if _, err := os.Stat(svc.highlightPath(ready)); err != nil {
			t.Errorf("the accepted file must exist: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(fps) != 2 {
			t.Errorf("attempts = %v, want exactly two (no third try)", fps)
		}
	})
}

// TestHighlightWithMissingFavoriteFailsCleanly covers the orphan an interrupted
// permanent delete can leave: the row exists, the favorite does not. It must end
// as a failure rather than stay counted as in flight forever.
func TestHighlightWithMissingFavoriteFailsCleanly(t *testing.T) {
	svc, _, v := newHighlightFixture(t)
	h := &Highlight{
		VideoID:   v.ID + 100,
		Kind:      HighlightKindMoment,
		StartMs:   1000,
		MediaPath: highlightMediaPath(v.ID+100, 1, HighlightKindMoment),
		Status:    HighlightStatusQueued,
		CreatedAt: nowMillis(),
		UpdatedAt: nowMillis(),
	}
	if err := svc.repo.CreateHighlight(h); err != nil {
		t.Fatalf("create: %v", err)
	}
	svc.highlights.generateMoment = func(_ context.Context, _ string, _ int64, _ string, _ time.Duration) error {
		t.Error("the generator must not run for a favorite that does not exist")
		return nil
	}
	svc.StartHighlights()

	failed := waitForHighlight(t, svc, h.VideoID, h.ID, HighlightStatusFailed)
	if failed.Error != highlightErrDeletedVideo {
		t.Errorf("error = %q, want %q", failed.Error, highlightErrDeletedVideo)
	}
	st := waitForHighlightTask(t, svc, func(st HighlightTaskStatus) bool { return !st.Active })
	if st.Failed != 1 || st.Current != "" {
		t.Errorf("task status = %+v, want one failure and no current item", st)
	}
}

// TestRemoveVideoAccountsForItsPendingHighlights keeps the round counter honest
// when a favorite is permanently deleted with work still queued.
func TestRemoveVideoAccountsForItsPendingHighlights(t *testing.T) {
	svc, _, v := newHighlightFixture(t)
	if _, err := svc.AddHighlight(v.ID, 1000, 0); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := svc.AddHighlight(v.ID, 2000, 0); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := svc.Remove(v.ID); err != nil {
		t.Fatalf("remove video: %v", err)
	}

	st := svc.HighlightTaskStatus()
	if st.Total != 2 || st.Done != 2 || st.Canceled != 2 || st.Active {
		t.Errorf("task status = %+v, want both rows cancelled", st)
	}
}
