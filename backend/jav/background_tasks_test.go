package jav

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"muse-app/backend/video"
)

// waitForTitleStatus polls the dashboard projection until it satisfies want.
// Progress is updated by the workers, so every assertion here waits for the
// event instead of sleeping a fixed amount.
func waitForTitleStatus(t *testing.T, s *Service, want func(TitleTranslationStatus) bool) TitleTranslationStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := s.TitleTranslationStatus()
		if want(st) {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("title status did not reach the expected state: %+v", s.TitleTranslationStatus())
	return TitleTranslationStatus{}
}

// A bulk pass must describe itself: the total is known when the work is
// accepted, and the finished counts survive the end of the pass so the
// dashboard can show the last result without persisting anything.
func TestTitleTranslationStatusTracksOnePass(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{result: "邻家的人妻"}
	s.SetTitleTranslator(translator)

	// Scraped while translation is paused: the row exists but is untranslated.
	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	if st := s.TitleTranslationStatus(); st.Active || st.Total != 0 {
		t.Fatalf("status before a pass = %+v, want idle", st)
	}

	translator.configured.Store(true)
	queued, err := s.TranslateMissingTitles()
	if err != nil {
		t.Fatalf("translate missing: %v", err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1", queued)
	}
	waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZhStatus == TitleZhSucceeded })

	st := waitForTitleStatus(t, s, func(st TitleTranslationStatus) bool { return !st.Active })
	if st.Total != 1 || st.Done != 1 || st.Failed != 0 {
		t.Fatalf("status = %+v, want one successful row", st)
	}
	if st.StartedAt == 0 || st.UpdatedAt < st.StartedAt {
		t.Fatalf("timestamps = %d / %d, want a start and a later update", st.StartedAt, st.UpdatedAt)
	}
}

// A failed provider call must be counted as a failure, not silently dropped.
func TestTitleTranslationStatusCountsFailures(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{err: errors.New("LLM 鉴权失败（401）")}
	s.SetTitleTranslator(translator)

	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	translator.configured.Store(true)
	if _, err := s.TranslateMissingTitles(); err != nil {
		t.Fatalf("translate missing: %v", err)
	}
	waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZhStatus == TitleZhFailed })

	st := waitForTitleStatus(t, s, func(st TitleTranslationStatus) bool { return !st.Active })
	if st.Total != 1 || st.Done != 0 || st.Failed != 1 {
		t.Fatalf("status = %+v, want one failed row", st)
	}
}

// A manual translation started after the last pass ended is a new pass: the
// counters restart, so "上次：1/1" never silently grows into "2/2".
func TestTitleTranslationStatusStartsANewPassAfterIdle(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{result: "邻家的人妻"}
	s.SetTitleTranslator(translator)

	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	translator.configured.Store(true)
	if _, err := s.TranslateMissingTitles(); err != nil {
		t.Fatalf("translate missing: %v", err)
	}
	waitForTitleStatus(t, s, func(st TitleTranslationStatus) bool { return !st.Active && st.Done == 1 })

	if err := s.TranslateTitle(id); err != nil {
		t.Fatalf("translate title: %v", err)
	}
	st := waitForTitleStatus(t, s, func(st TitleTranslationStatus) bool { return !st.Active && st.Done == 1 })
	if st.Total != 1 {
		t.Fatalf("status = %+v, want the new pass to count one row", st)
	}
}

// A row whose video record disappeared is skipped by the worker; the pass must
// still close its books (Done+Failed == Total) instead of reporting progress
// that never completes.
func TestTitleTranslationStatusCountsUnreadableRows(t *testing.T) {
	s, _, _, _ := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{result: "不会用到的译名"}
	translator.configured.Store(true)
	s.SetTitleTranslator(translator)

	if !s.enqueueTitleTranslation(99999, false) {
		t.Fatal("a missing record should still be accepted by the queue")
	}
	st := waitForTitleStatus(t, s, func(st TitleTranslationStatus) bool { return !st.Active })
	if st.Total != 1 || st.Done != 0 || st.Failed != 1 {
		t.Fatalf("status = %+v, want the skipped row counted as failed", st)
	}
}

// The scrape row reports queue depth, not a percentage: the dashboard reads the
// cheap aggregate and must ignore trashed favorites just like the board.
func TestScrapeTaskCountsGroupsByState(t *testing.T) {
	s, db, vr, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	repo := NewSQLiteRepository(db)
	if err := repo.PrepareTask(id, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE jav_scrape_tasks SET status='running', finished_at=100 WHERE video_id=?`, id); err != nil {
		t.Fatal(err)
	}

	other := &video.Video{Path: filepath.Join(t.TempDir(), "ABW-180.mp4"), FileName: "ABW-180.mp4", FileSize: 7, Status: video.StatusAvailable, CreatedAt: 1, UpdatedAt: 1}
	if err := vr.Create(other); err != nil {
		t.Fatal(err)
	}
	if err := repo.PrepareTask(other.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE jav_scrape_tasks SET status='succeeded', finished_at=200 WHERE video_id=?`, other.ID); err != nil {
		t.Fatal(err)
	}

	trashed := &video.Video{Path: filepath.Join(t.TempDir(), "TRA-1.mp4"), FileName: "TRA-1.mp4", FileSize: 7, Status: video.StatusAvailable, CreatedAt: 1, UpdatedAt: 1}
	if err := vr.Create(trashed); err != nil {
		t.Fatal(err)
	}
	if err := repo.PrepareTask(trashed.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE videos SET deleted_at=1 WHERE id=?`, trashed.ID); err != nil {
		t.Fatal(err)
	}

	counts, err := s.ScrapeTaskCounts()
	if err != nil {
		t.Fatalf("scrape task counts: %v", err)
	}
	if counts.Running != 1 || counts.Succeeded != 1 || counts.Queued != 0 || counts.Failed != 0 {
		t.Fatalf("counts = %+v, want one running and one succeeded task", counts)
	}
	if counts.LastFinishedAt != 200 {
		t.Fatalf("last finished at = %d, want 200", counts.LastFinishedAt)
	}
}
