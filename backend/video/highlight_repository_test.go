package video

import (
	"testing"
)

// The highlight repository is exercised through its transitions rather than
// through the fields it writes: the worker's whole safety story is "a conditional
// UPDATE reports whether the row is still mine", so that is what these tests pin
// down.

func createHighlight(t *testing.T, repo VideoRepository, videoID int64, kind string, startMs, endMs int64) *Highlight {
	t.Helper()
	now := nowMillis()
	h := &Highlight{
		VideoID:   videoID,
		Kind:      kind,
		StartMs:   startMs,
		EndMs:     endMs,
		MediaPath: highlightMediaPath(videoID, 1, kind),
		Status:    HighlightStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateHighlight(h); err != nil {
		t.Fatalf("create highlight: %v", err)
	}
	return h
}

func TestHighlightRepositoryCRUD(t *testing.T) {
	repo := newTestRepo(t)
	v := sampleVideo(`D:\Videos\a.mp4`)
	if err := repo.Create(v); err != nil {
		t.Fatalf("create video: %v", err)
	}

	moment := createHighlight(t, repo, v.ID, HighlightKindMoment, 90_000, 0)
	if moment.ID == 0 {
		t.Fatal("expected the highlight id to be set")
	}
	if !ValidHighlightFileName(moment.MediaPath[len(HighlightsRelativeDir)+1:]) {
		t.Errorf("media path %q does not match the generated-file shape", moment.MediaPath)
	}

	got, err := repo.GetHighlight(moment.ID)
	if err != nil {
		t.Fatalf("get highlight: %v", err)
	}
	if got.VideoID != v.ID || got.Kind != HighlightKindMoment || got.StartMs != 90_000 || got.Status != HighlightStatusQueued {
		t.Errorf("got %+v", got)
	}

	// A row is only reachable through its own favorite.
	if _, err := repo.GetVideoHighlight(v.ID+1, moment.ID); err != ErrNotFound {
		t.Errorf("cross-video read: err = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetVideoHighlight(v.ID, moment.ID); err != nil {
		t.Errorf("own-video read: %v", err)
	}

	// Instants sort by timeline, not by insertion order.
	if err := repo.CreateHighlight(&Highlight{
		VideoID: v.ID, Kind: HighlightKindRange, StartMs: 30_000, EndMs: 35_000,
		MediaPath: highlightMediaPath(v.ID, 2, HighlightKindRange),
		Status:    HighlightStatusQueued, CreatedAt: 1, UpdatedAt: 1,
	}); err != nil {
		t.Fatalf("create earlier highlight: %v", err)
	}
	list, err := repo.ListHighlights(v.ID)
	if err != nil {
		t.Fatalf("list highlights: %v", err)
	}
	if len(list) != 2 || list[0].StartMs != 30_000 || list[1].StartMs != 90_000 {
		t.Fatalf("list order = %+v, want the timeline order", list)
	}
	if other, err := repo.ListHighlights(v.ID + 1); err != nil || len(other) != 0 || other == nil {
		t.Errorf("other favorite = %v (err %v), want a non-nil empty list", other, err)
	}

	paths, err := repo.ListHighlightMediaPaths()
	if err != nil {
		t.Fatalf("list media paths: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("media paths = %v, want both rows", paths)
	}
}

// TestHighlightRepositoryClaimAndFinish covers the two transitions the worker
// depends on, including the "the row moved under me" answers.
func TestHighlightRepositoryClaimAndFinish(t *testing.T) {
	repo := newTestRepo(t)
	v := sampleVideo(`D:\Videos\a.mp4`)
	if err := repo.Create(v); err != nil {
		t.Fatalf("create video: %v", err)
	}
	h := createHighlight(t, repo, v.ID, HighlightKindMoment, 1000, 0)

	if ok, err := repo.ClaimHighlight(h.ID); err != nil || !ok {
		t.Fatalf("first claim = %v, %v, want true", ok, err)
	}
	if ok, err := repo.ClaimHighlight(h.ID); err != nil || ok {
		t.Errorf("second claim = %v, %v, want false", ok, err)
	}

	if ok, err := repo.FinishHighlight(h.ID, HighlightStatusReady, "", 777); err != nil || !ok {
		t.Fatalf("finish = %v, %v, want true", ok, err)
	}
	got, err := repo.GetHighlight(h.ID)
	if err != nil {
		t.Fatalf("get after finish: %v", err)
	}
	if got.Status != HighlightStatusReady || got.FinishedAt != 777 {
		t.Errorf("after finish = %+v", got)
	}

	// Finishing a deleted row reports false, which is the worker's signal to
	// discard the file it just produced.
	if err := repo.DeleteHighlight(v.ID, h.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if ok, err := repo.FinishHighlight(h.ID, HighlightStatusReady, "", 1); err != nil || ok {
		t.Errorf("finish after delete = %v, %v, want false", ok, err)
	}
}

func TestHighlightRepositoryRetry(t *testing.T) {
	repo := newTestRepo(t)
	v := sampleVideo(`D:\Videos\a.mp4`)
	if err := repo.Create(v); err != nil {
		t.Fatalf("create video: %v", err)
	}
	h := createHighlight(t, repo, v.ID, HighlightKindRange, 0, 5000)

	// A queued row has nothing to retry.
	if ok, err := repo.RetryHighlight(v.ID, h.ID); err != nil || ok {
		t.Errorf("retry queued = %v, %v, want false", ok, err)
	}

	if ok, err := repo.ClaimHighlight(h.ID); err != nil || !ok {
		t.Fatalf("claim: %v, %v", ok, err)
	}
	if ok, err := repo.FinishHighlight(h.ID, HighlightStatusFailed, "源文件缺失", 5); err != nil || !ok {
		t.Fatalf("finish failed: %v, %v", ok, err)
	}
	// Retrying through the wrong favorite must not touch the row.
	if ok, err := repo.RetryHighlight(v.ID+1, h.ID); err != nil || ok {
		t.Errorf("retry cross-video = %v, %v, want false", ok, err)
	}
	if ok, err := repo.RetryHighlight(v.ID, h.ID); err != nil || !ok {
		t.Fatalf("retry = %v, %v, want true", ok, err)
	}
	got, err := repo.GetHighlight(h.ID)
	if err != nil {
		t.Fatalf("get after retry: %v", err)
	}
	if got.Status != HighlightStatusQueued || got.Error != "" || got.FinishedAt != 0 {
		t.Errorf("after retry = %+v, want a clean queued row", got)
	}
}

// TestHighlightRepositoryRecover covers the restart rules: a queued row is
// re-queued, a running row is bumped, and the third interruption gives up.
func TestHighlightRepositoryRecover(t *testing.T) {
	repo := newTestRepo(t)
	v := sampleVideo(`D:\Videos\a.mp4`)
	if err := repo.Create(v); err != nil {
		t.Fatalf("create video: %v", err)
	}

	queued := createHighlight(t, repo, v.ID, HighlightKindMoment, 1000, 0)
	interrupted := createHighlight(t, repo, v.ID, HighlightKindMoment, 2000, 0)
	if ok, err := repo.ClaimHighlight(interrupted.ID); err != nil || !ok {
		t.Fatalf("claim: %v, %v", ok, err)
	}

	requeue, failed, err := repo.RecoverHighlights(3)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if failed != 0 {
		t.Errorf("failed = %d, want 0 on the first interruption", failed)
	}
	if len(requeue) != 2 {
		t.Fatalf("requeue = %v, want both rows", requeue)
	}
	got, err := repo.GetHighlight(interrupted.ID)
	if err != nil {
		t.Fatalf("get recovered: %v", err)
	}
	if got.Status != HighlightStatusQueued || got.Interruptions != 1 {
		t.Errorf("recovered row = %+v, want queued with one interruption", got)
	}

	// Interrupt it again, twice: the third time gives up.
	for i := 2; i <= 3; i++ {
		if ok, err := repo.ClaimHighlight(interrupted.ID); err != nil || !ok {
			t.Fatalf("re-claim %d: %v, %v", i, ok, err)
		}
		if _, _, err := repo.RecoverHighlights(3); err != nil {
			t.Fatalf("recover %d: %v", i, err)
		}
	}
	got, err = repo.GetHighlight(interrupted.ID)
	if err != nil {
		t.Fatalf("get after third interruption: %v", err)
	}
	if got.Status != HighlightStatusFailed || got.Error != "生成被多次中断" || got.Interruptions != 3 || got.FinishedAt == 0 {
		t.Errorf("after three interruptions = %+v", got)
	}
	// The row that was only ever queued is untouched.
	if ok, err := repo.ClaimHighlight(queued.ID); err != nil || !ok {
		t.Errorf("queued row was consumed: %v, %v", ok, err)
	}
}

// TestHighlightRepositoryDeleteCascadesWithVideo pins the permanent-delete rule:
// removing a favorite removes its highlights in the same transaction.
func TestHighlightRepositoryDeleteCascadesWithVideo(t *testing.T) {
	repo := newTestRepo(t)
	v := sampleVideo(`D:\Videos\a.mp4`)
	if err := repo.Create(v); err != nil {
		t.Fatalf("create video: %v", err)
	}
	keep := sampleVideo(`D:\Videos\b.mp4`)
	if err := repo.Create(keep); err != nil {
		t.Fatalf("create second video: %v", err)
	}
	mine := createHighlight(t, repo, v.ID, HighlightKindMoment, 1000, 0)
	other := createHighlight(t, repo, keep.ID, HighlightKindMoment, 1000, 0)

	if err := repo.Delete(v.ID); err != nil {
		t.Fatalf("delete video: %v", err)
	}
	if _, err := repo.GetHighlight(mine.ID); err != ErrNotFound {
		t.Errorf("highlight of a deleted favorite = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetHighlight(other.ID); err != nil {
		t.Errorf("other favorite's highlight was deleted: %v", err)
	}
}
