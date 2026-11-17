package api

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"muse-app/backend/database"
	"muse-app/backend/group"
	"muse-app/backend/jav"
	"muse-app/backend/tasks"
	"muse-app/backend/video"
)

// bgTranslator answers immediately so a facade test can watch the dashboard go
// from running to done without a provider.
type bgTranslator struct{ result string }

func (t bgTranslator) Configured() bool { return true }
func (t bgTranslator) Available() bool  { return true }

func (t bgTranslator) TranslateTitle(context.Context, string) (string, error) {
	return t.result, nil
}

func (t bgTranslator) TranslateTitles(_ context.Context, titles []string) ([]string, []error) {
	translations := make([]string, len(titles))
	errs := make([]error, len(titles))
	for i := range titles {
		translations[i] = t.result
	}
	return translations, errs
}

// newBackgroundLibrary wires a real facade over a migrated database with one
// successfully scraped favorite, which is the smallest library that can show a
// translation task and a scrape row.
func newBackgroundLibrary(t *testing.T) (*Library, *jav.Service, *sql.DB, int64) {
	t.Helper()
	db, err := sql.Open(database.DriverName, "file:"+filepath.Join(t.TempDir(), "app.db"))
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
	lib := New(
		video.NewService(vr, nil, t.TempDir(), discard, nil),
		group.NewService(&memGroupRepo{members: map[int64][]int64{}}, discard, nil),
		discard,
	)
	javRepo := jav.NewSQLiteRepository(db)
	svc := jav.NewService(javRepo, vr, nil, "", nil, discard, 1)
	t.Cleanup(svc.Close)
	lib.SetJavService(svc)

	v := &video.Video{
		Path:      filepath.Join(t.TempDir(), "IPX-633.mp4"),
		FileName:  "IPX-633.mp4",
		FileSize:  7,
		Status:    video.StatusAvailable,
		CreatedAt: 1,
		UpdatedAt: 1,
	}
	if err := vr.Create(v); err != nil {
		t.Fatal(err)
	}
	if err := javRepo.PrepareTask(v.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := javRepo.SaveSuccess(v.ID, jav.Metadata{Code: "IPX-633", Title: "となりの人妻"}, "filename", 1); err != nil {
		t.Fatal(err)
	}
	// The scrape itself succeeded; only the task row is set here because the
	// fixture skips the worker.
	if _, err := db.Exec(`UPDATE jav_scrape_tasks SET status='succeeded', finished_at=1 WHERE video_id=?`, v.ID); err != nil {
		t.Fatal(err)
	}
	return lib, svc, db, v.ID
}

// waitForTask polls the snapshot until the row appears in the wanted state.
func waitForTask(t *testing.T, lib *Library, id string, want func(tasks.Task) bool) tasks.Task {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, task := range lib.BackgroundTasks().Tasks {
			if task.ID == id && want(task) {
				return task
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("task %q did not reach the expected state: %+v", id, lib.BackgroundTasks().Tasks)
	return tasks.Task{}
}

// A finished translation pass is projected with its counts and unit, which is
// what the dashboard draws instead of the log lines users had to read.
func TestBackgroundTasksProjectsTranslationPass(t *testing.T) {
	lib, svc, _, _ := newBackgroundLibrary(t)
	svc.SetTitleTranslator(bgTranslator{result: "邻家的人妻"})
	if _, err := lib.TranslateMissingJavTitles(); err != nil {
		t.Fatalf("translate missing: %v", err)
	}

	row := waitForTask(t, lib, "title-translation", func(task tasks.Task) bool {
		return task.State == tasks.StateDone
	})
	if row.Total != 1 || row.Done != 1 || row.Failed != 0 {
		t.Fatalf("translation row = %+v, want one successful row", row)
	}
	if row.Unit != "条" || row.Kind != tasks.KindBatch {
		t.Fatalf("translation row = %+v, want a 条 batch", row)
	}
	if row.StartedAt == 0 {
		t.Fatalf("translation row = %+v, want a start time", row)
	}
}

// A scrape queue has no batch boundary, so the row reports depth and a detail
// target instead of a percentage.
func TestBackgroundTasksReportsScrapeQueueDepth(t *testing.T) {
	lib, _, db, id := newBackgroundLibrary(t)
	// Put the finished fixture task back in the queue so the row has depth to
	// report.
	if _, err := db.Exec(`UPDATE jav_scrape_tasks SET status='queued', finished_at=0 WHERE video_id=?`, id); err != nil {
		t.Fatal(err)
	}

	row := waitForTask(t, lib, "scrape", func(task tasks.Task) bool {
		return task.State == tasks.StateRunning
	})
	if row.Total != 0 {
		t.Fatalf("scrape row = %+v, want no invented denominator", row)
	}
	if row.Message == "" {
		t.Fatalf("scrape row = %+v, want a queue breakdown", row)
	}
	if row.Detail != "scrapeTasks" {
		t.Fatalf("scrape row = %+v, want the board as its detail view", row)
	}
}

// Without services the dashboard is empty rather than nil: the JSON contract is
// an array, and an empty screen is the honest answer.
func TestBackgroundTasksWithoutServices(t *testing.T) {
	lib := New(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	snapshot := lib.BackgroundTasks()
	if snapshot.Tasks == nil {
		t.Fatal("tasks = nil, want an empty slice")
	}
	if len(snapshot.Tasks) != 0 {
		t.Fatalf("tasks = %+v, want none", snapshot.Tasks)
	}
}
