package api

import (
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"muse-app/backend/database"
	"muse-app/backend/tasks"
	"muse-app/backend/video"
)

// The highlight half of memVideoRepo. It keeps the same "the repository is dumb,
// the service decides" split as the SQLite implementation, including the false
// answers the generator's conditional updates rely on.

func (r *memVideoRepo) CreateHighlight(h *video.Highlight) error {
	r.nextHighlightID++
	h.ID = r.nextHighlightID
	r.highlights = append(r.highlights, *h)
	return nil
}

func (r *memVideoRepo) GetHighlight(id int64) (*video.Highlight, error) {
	for i := range r.highlights {
		if r.highlights[i].ID == id {
			return &r.highlights[i], nil
		}
	}
	return nil, video.ErrNotFound
}

func (r *memVideoRepo) GetVideoHighlight(videoID, id int64) (*video.Highlight, error) {
	h, err := r.GetHighlight(id)
	if err != nil {
		return nil, err
	}
	if h.VideoID != videoID {
		return nil, video.ErrNotFound
	}
	return h, nil
}

func (r *memVideoRepo) ListHighlights(videoID int64) ([]video.Highlight, error) {
	out := make([]video.Highlight, 0)
	for _, h := range r.highlights {
		if h.VideoID == videoID {
			out = append(out, h)
		}
	}
	return out, nil
}

func (r *memVideoRepo) ListQueuedHighlightIDs() ([]int64, error) {
	out := make([]int64, 0)
	for _, h := range r.highlights {
		if h.Status == video.HighlightStatusQueued {
			out = append(out, h.ID)
		}
	}
	return out, nil
}

func (r *memVideoRepo) ListHighlightMediaPaths() ([]string, error) {
	out := make([]string, 0, len(r.highlights))
	for _, h := range r.highlights {
		if h.MediaPath != "" {
			out = append(out, h.MediaPath)
		}
	}
	return out, nil
}

func (r *memVideoRepo) ClaimHighlight(id int64) (bool, error) {
	h, err := r.GetHighlight(id)
	if err != nil || h.Status != video.HighlightStatusQueued {
		return false, nil
	}
	h.Status = video.HighlightStatusRunning
	return true, nil
}

func (r *memVideoRepo) FinishHighlight(id int64, status, errMsg string, finishedAt int64) (bool, error) {
	h, err := r.GetHighlight(id)
	if err != nil {
		return false, nil
	}
	h.Status, h.Error, h.FinishedAt = status, errMsg, finishedAt
	return true, nil
}

func (r *memVideoRepo) RetryHighlight(videoID, id int64) (bool, error) {
	h, err := r.GetVideoHighlight(videoID, id)
	if err != nil || h.Status != video.HighlightStatusFailed {
		return false, nil
	}
	h.Status, h.Error, h.FinishedAt = video.HighlightStatusQueued, "", 0
	return true, nil
}

func (r *memVideoRepo) RecoverHighlights(maxInterruptions int) ([]int64, int, error) {
	failed := 0
	for i := range r.highlights {
		h := &r.highlights[i]
		if h.Status != video.HighlightStatusRunning {
			continue
		}
		h.Interruptions++
		if h.Interruptions >= maxInterruptions {
			h.Status = video.HighlightStatusFailed
			h.Error = "生成被多次中断"
			failed++
			continue
		}
		h.Status = video.HighlightStatusQueued
	}
	ids, err := r.ListQueuedHighlightIDs()
	return ids, failed, err
}

func (r *memVideoRepo) DeleteHighlight(videoID, id int64) error {
	out := r.highlights[:0]
	for _, h := range r.highlights {
		if h.ID != id || h.VideoID != videoID {
			out = append(out, h)
		}
	}
	r.highlights = out
	return nil
}

// newHighlightLibrary wires a real façade over a migrated database with one
// favorite whose file exists, so the highlight service reaches its generator.
// The processor has no ffmpeg, which makes every generation fail fast: that is
// the cheapest way to reach a finished round in a test.
func newHighlightLibrary(t *testing.T) (*Library, *video.Service, int64) {
	t.Helper()
	dataDir := t.TempDir()
	db, err := sql.Open(database.DriverName, "file:"+filepath.Join(dataDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}

	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	vr := video.NewSQLiteRepository(db)
	processor := video.NewProcessor(vr, "", "", filepath.Join(dataDir, "thumbnails"), nil, discard, 1)
	svc := video.NewService(vr, processor, dataDir, discard, nil)
	lib := New(svc, discard)

	p := filepath.Join(dataDir, "moment.mp4")
	if err := os.WriteFile(p, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := &video.Video{
		Path: p, FileName: "moment.mp4", FileSize: 5, DurationMs: 60_000,
		Status: video.StatusAvailable, CreatedAt: 1, UpdatedAt: 1,
	}
	if err := vr.Create(v); err != nil {
		t.Fatal(err)
	}
	return lib, svc, v.ID
}

// TestBackgroundTasksProjectsHighlightRound covers the dashboard projection: a
// round with work in flight is one running row, and the finished round keeps its
// counts and a summary instead of disappearing.
func TestBackgroundTasksProjectsHighlightRound(t *testing.T) {
	lib, svc, videoID := newHighlightLibrary(t)
	t.Cleanup(svc.CloseHighlights)

	// Queued but not started: the row must already describe the waiting work.
	if _, err := lib.AddVideoHighlight(videoID, 1000, 0); err != nil {
		t.Fatalf("add highlight: %v", err)
	}
	row := findTaskRow(t, lib, "highlight-moments")
	if row.Title != "精彩时刻生成" || row.Unit != "条" || row.Kind != tasks.KindBatch {
		t.Errorf("row = %+v", row)
	}
	if row.State != tasks.StateRunning || row.Total != 1 || row.Done != 0 {
		t.Errorf("pending row = %+v, want one running row", row)
	}
	if row.Message != "排队 1" {
		t.Errorf("message = %q, want the queue depth", row.Message)
	}

	// Let the worker run it. There is no ffmpeg in this fixture, so the round
	// ends in one failure; what matters is the projection of a finished round.
	svc.StartHighlights()
	deadline := time.Now().Add(3 * time.Second)
	for {
		row = findTaskRow(t, lib, "highlight-moments")
		if row.State == tasks.StateDone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("round did not finish: %+v", row)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if row.Total != 1 || row.Failed != 1 || row.Done != 0 {
		t.Errorf("finished row = %+v, want one failed generation", row)
	}
	if row.Message != "完成 0 · 失败 1" {
		t.Errorf("message = %q", row.Message)
	}
	if row.StartedAt == 0 {
		t.Error("a finished round must keep its start time")
	}
}

// TestHighlightFacadePassThrough keeps the four façade methods honest: the read
// returns what the service stored, validation happens once, and the two writes
// reach the service.
func TestHighlightFacadePassThrough(t *testing.T) {
	lib, svc, videoID := newHighlightLibrary(t)
	t.Cleanup(svc.CloseHighlights)

	added, err := lib.AddVideoHighlight(videoID, 90_000, 0)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	list, err := lib.ListVideoHighlights(videoID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != added.ID || list[0].Kind != video.HighlightKindMoment {
		t.Fatalf("list = %+v", list)
	}

	// An impossible range is refused by the same rules the desktop applies.
	if _, err := lib.AddVideoHighlight(videoID, 5000, 4000); err == nil {
		t.Error("an impossible range must be refused")
	}
	if list, _ := lib.ListVideoHighlights(videoID); len(list) != 1 {
		t.Errorf("a refused add stored a row: %+v", list)
	}

	// Unknown ids: delete is idempotent, retry reports the miss.
	if err := lib.DeleteVideoHighlight(videoID, 999); err != nil {
		t.Errorf("delete unknown: %v", err)
	}
	if err := lib.RetryVideoHighlight(videoID, 999); err == nil {
		t.Error("retrying a highlight that does not exist should report it")
	}

	if err := lib.DeleteVideoHighlight(videoID, added.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if list, _ := lib.ListVideoHighlights(videoID); len(list) != 0 {
		t.Errorf("highlights after delete = %+v", list)
	}
}

// findTaskRow returns one dashboard row, failing the test when it is absent.
func findTaskRow(t *testing.T, lib *Library, id string) tasks.Task {
	t.Helper()
	for _, row := range lib.BackgroundTasks().Tasks {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("后台任务看板缺少 %q 行: %+v", id, lib.BackgroundTasks().Tasks)
	return tasks.Task{}
}
