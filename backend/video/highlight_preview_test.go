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

// Preview tests. The point of a preview is that the dialog shows the file the
// save will keep, so these pin the two halves of that promise: rendering records
// nothing, and saving adopts the rendered bytes instead of encoding again.

// previewFixture installs a fake generator that writes a small file and records
// the paths it was asked to render.
type previewFixture struct {
	svc   *Service
	dir   string
	video *Video
	mu    sync.Mutex
	calls []string
}

func newPreviewFixture(t *testing.T) *previewFixture {
	t.Helper()
	svc, dir, v := newHighlightFixture(t)
	f := &previewFixture{svc: svc, dir: dir, video: v}
	svc.highlights.generateMoment = func(_ context.Context, _ string, _ int64, outPath string, _ time.Duration) error {
		f.mu.Lock()
		f.calls = append(f.calls, outPath)
		f.mu.Unlock()
		return writeHighlightFile(outPath, "jpeg")
	}
	svc.highlights.generateClip = func(_ context.Context, _ string, _ int64, _ int64, _ int, outPath string, _ time.Duration) error {
		f.mu.Lock()
		f.calls = append(f.calls, outPath)
		f.mu.Unlock()
		return writeHighlightFile(outPath, "mp4")
	}
	return f
}

func (f *previewFixture) renders() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func TestPreviewHighlightRendersWithoutRecording(t *testing.T) {
	f := newPreviewFixture(t)

	preview, err := f.svc.PreviewHighlight(f.video.ID, 30_000, 35_000)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.Kind != HighlightKindRange || preview.StartMs != 30_000 || preview.EndMs != 35_000 {
		t.Errorf("preview = %+v, want the request echoed back", preview)
	}
	if !strings.HasPrefix(preview.Path, HighlightsPreviewDir+"/") {
		t.Errorf("path = %q, want it under %s/", preview.Path, HighlightsPreviewDir)
	}
	if !ValidHighlightPreviewFileName(filepath.Base(preview.Path)) {
		t.Errorf("path = %q is not a preview name", preview.Path)
	}
	if preview.FPS != highlightClipFPS {
		t.Errorf("fps = %d, want the clip rate %d", preview.FPS, highlightClipFPS)
	}
	if !strings.HasSuffix(preview.Path, ".mp4") {
		t.Errorf("path = %q, want an MP4 the browser can play in a <video>", preview.Path)
	}
	if preview.Bytes <= 0 {
		t.Errorf("bytes = %d, want the rendered size", preview.Bytes)
	}

	// The rendered file exists and nothing was recorded.
	if _, err := os.Stat(filepath.Join(f.dir, filepath.FromSlash(preview.Path))); err != nil {
		t.Fatalf("preview file missing: %v", err)
	}
	if list, err := f.svc.ListHighlights(f.video.ID); err != nil || len(list) != 0 {
		t.Errorf("highlights after preview = %+v (err %v), want none", list, err)
	}
	if st := f.svc.HighlightTaskStatus(); st.Total != 0 || st.Active {
		t.Errorf("task status = %+v, want an untouched round", st)
	}
}

func TestPreviewHighlightMomentIsAStill(t *testing.T) {
	f := newPreviewFixture(t)
	preview, err := f.svc.PreviewHighlight(f.video.ID, 90_000, 0)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.Kind != HighlightKindMoment || preview.FPS != 0 {
		t.Errorf("preview = %+v, want a still with no frame rate", preview)
	}
	if !strings.HasSuffix(preview.Path, ".jpg") {
		t.Errorf("path = %q, want a JPEG", preview.Path)
	}
}

// A clip preview is rendered once, at the clip frame rate: there is no size
// fallback any more, so the rate a preview reports is the rate the saved file has.
func TestPreviewHighlightRendersOneClipAtTheClipRate(t *testing.T) {
	f := newPreviewFixture(t)
	var (
		mu    sync.Mutex
		rates []int
	)
	f.svc.highlights.generateClip = func(_ context.Context, _ string, _ int64, _ int64, fps int, outPath string, _ time.Duration) error {
		mu.Lock()
		rates = append(rates, fps)
		mu.Unlock()
		return writeHighlightFile(outPath, "mp4")
	}

	preview, err := f.svc.PreviewHighlight(f.video.ID, 1000, 21_000)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(rates) != 1 || rates[0] != highlightClipFPS {
		t.Errorf("renders = %v, want exactly one at %d fps", rates, highlightClipFPS)
	}
	if preview.FPS != highlightClipFPS {
		t.Errorf("fps = %d, want %d", preview.FPS, highlightClipFPS)
	}
}

func TestPreviewHighlightRejectsBadInput(t *testing.T) {
	f := newPreviewFixture(t)

	if _, err := f.svc.PreviewHighlight(f.video.ID, 5000, 4000); err == nil {
		t.Error("an impossible range must be refused")
	}
	if len(f.renders()) != 0 {
		t.Errorf("a refused preview still ran ffmpeg: %v", f.renders())
	}

	if err := os.Remove(filepath.Join(f.dir, "moment.mp4")); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.PreviewHighlight(f.video.ID, 1000, 0)
	if err == nil || !strings.Contains(err.Error(), highlightErrSourceMissing) {
		t.Errorf("err = %v, want %q", err, highlightErrSourceMissing)
	}
}

func TestPreviewHighlightReportsGeneratorFailure(t *testing.T) {
	f := newPreviewFixture(t)
	f.svc.highlights.generateClip = func(_ context.Context, _ string, _ int64, _ int64, _ int, outPath string, _ time.Duration) error {
		if err := writeHighlightFile(outPath, "half"); err != nil {
			return err
		}
		return errors.New("boom")
	}

	_, err := f.svc.PreviewHighlight(f.video.ID, 1000, 6000)
	if err == nil || !strings.Contains(err.Error(), "生成预览失败：boom") {
		t.Fatalf("err = %v, want the generator reason", err)
	}
	// A failed preview must not leave a half-written file for the dialog to show.
	entries, readErr := os.ReadDir(filepath.Join(f.dir, HighlightsPreviewDir))
	if readErr == nil {
		for _, entry := range entries {
			if ValidHighlightPreviewFileName(entry.Name()) {
				t.Errorf("failed preview left %q behind", entry.Name())
			}
		}
	}
}

func TestAddHighlightFromPreviewAdoptsTheFile(t *testing.T) {
	f := newPreviewFixture(t)
	preview, err := f.svc.PreviewHighlight(f.video.ID, 30_000, 35_000)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	previewPath := filepath.Join(f.dir, filepath.FromSlash(preview.Path))
	renders := len(f.renders())

	h, err := f.svc.AddHighlightFromPreview(f.video.ID, 30_000, 35_000, preview.Path)
	if err != nil {
		t.Fatalf("save from preview: %v", err)
	}
	if h.Status != HighlightStatusReady || h.Kind != HighlightKindRange ||
		h.StartMs != 30_000 || h.EndMs != 35_000 || h.FinishedAt == 0 {
		t.Errorf("saved = %+v, want a ready row with the confirmed times", h)
	}
	if !ValidHighlightFileName(filepath.Base(filepath.FromSlash(h.MediaPath))) {
		t.Errorf("media path = %q, want a generated-file name", h.MediaPath)
	}
	// The bytes were adopted, not re-encoded: one render, and the file moved.
	if len(f.renders()) != renders {
		t.Errorf("save re-ran the generator (%d -> %d renders)", renders, len(f.renders()))
	}
	if _, err := os.Stat(previewPath); !os.IsNotExist(err) {
		t.Error("the preview file should have moved out of preview/")
	}
	if _, err := os.Stat(f.svc.highlightPath(h)); err != nil {
		t.Errorf("saved file missing: %v", err)
	}
	if list, err := f.svc.ListHighlights(f.video.ID); err != nil || len(list) != 1 {
		t.Errorf("highlights = %+v (err %v), want the saved row", list, err)
	}
	// Nothing went through the queue, so no round is active.
	if st := f.svc.HighlightTaskStatus(); st.Active {
		t.Errorf("task status = %+v, want no round for an adopted preview", st)
	}
}

func TestAddHighlightFromPreviewRejectsStaleInput(t *testing.T) {
	f := newPreviewFixture(t)
	preview, err := f.svc.PreviewHighlight(f.video.ID, 30_000, 35_000)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	cases := []struct {
		name    string
		start   int64
		end     int64
		path    string
		wantErr error
	}{
		{"other range", 30_000, 36_000, preview.Path, errHighlightPreviewStale},
		{"other start", 31_000, 35_000, preview.Path, errHighlightPreviewStale},
		{"moment instead of range", 30_000, 0, preview.Path, errHighlightPreviewStale},
		{"unknown file", 30_000, 35_000, HighlightsPreviewDir + "/p-1-2-3-4-5.mp4", errHighlightPreviewStale},
		{"traversal", 30_000, 35_000, "../../etc/passwd", errHighlightPreviewStale},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.svc.AddHighlightFromPreview(f.video.ID, tc.start, tc.end, tc.path); !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
			if list, _ := f.svc.ListHighlights(f.video.ID); len(list) != 0 {
				t.Errorf("a refused save recorded %+v", list)
			}
		})
	}
}

func TestDiscardHighlightPreview(t *testing.T) {
	f := newPreviewFixture(t)
	preview, err := f.svc.PreviewHighlight(f.video.ID, 1000, 6000)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	path := filepath.Join(f.dir, filepath.FromSlash(preview.Path))

	if err := f.svc.DiscardHighlightPreview(preview.Path); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("discard must delete the preview file")
	}
	// Idempotent, and a path outside the preview folder is a no-op rather than a
	// delete of something arbitrary.
	if err := f.svc.DiscardHighlightPreview(preview.Path); err != nil {
		t.Errorf("second discard: %v", err)
	}
	keep := writeFakeVideo(t, f.dir, "keep.mp4", 10)
	if err := f.svc.DiscardHighlightPreview("../../keep.mp4"); err != nil {
		t.Errorf("foreign path: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("discard must never touch a file outside the preview folder")
	}
}

func TestStartHighlightsPurgesStalePreviews(t *testing.T) {
	f := newPreviewFixture(t)
	preview, err := f.svc.PreviewHighlight(f.video.ID, 1000, 6000)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	stalePath := filepath.Join(f.dir, filepath.FromSlash(preview.Path))
	foreign := writeImage(t, filepath.Join(f.dir, HighlightsPreviewDir), "notes.txt", "keep me")

	// A preview only means something to the dialog that rendered it.
	f.svc.StartHighlights()
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Error("a preview from a previous process must be cleaned up at startup")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Error("files the app did not name must survive the purge")
	}
}

// The orphan sweep owns thumbnails, scraped covers and highlights/ — never the
// preview folder, whose files have no record by design.
func TestImageSweepLeavesPreviewsAlone(t *testing.T) {
	f := newPreviewFixture(t)
	preview, err := f.svc.PreviewHighlight(f.video.ID, 1000, 6000)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	path := filepath.Join(f.dir, filepath.FromSlash(preview.Path))

	if _, err := f.svc.RemoveOrphanedImages(); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the sweep deleted an open preview: %v", err)
	}
}
