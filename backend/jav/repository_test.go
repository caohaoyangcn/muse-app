package jav

import (
	"path/filepath"
	"testing"

	"muse-app/backend/database"
	"muse-app/backend/video"
)

// ListCodes feeds the shared video read model, so it must return exactly the
// saved scrape results: a code is present only after a successful scrape and
// never for a video the user chose to skip.
func TestListCodesReturnsAuthoritativeResults(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewSQLiteRepository(db)
	videos := video.NewSQLiteRepository(db)
	create := func(name string) int64 {
		t.Helper()
		v := &video.Video{
			Path:      filepath.Join(t.TempDir(), name),
			FileName:  name,
			FileSize:  1,
			CreatedAt: 1,
			UpdatedAt: 1,
			Status:    video.StatusAvailable,
		}
		if err := videos.Create(v); err != nil {
			t.Fatalf("create favorite %q: %v", name, err)
		}
		if err := repo.PrepareTask(v.ID, false); err != nil {
			t.Fatalf("prepare task %q: %v", name, err)
		}
		return v.ID
	}

	scraped := create("a.mp4")
	if err := repo.SaveSuccess(scraped, Metadata{Code: "  IPX-633  "}, "filename", 2); err != nil {
		t.Fatalf("save success: %v", err)
	}
	unscraped := create("b.mp4")
	skipped := create("c.mp4")
	if err := repo.SaveSuccess(skipped, Metadata{Code: "ABW-180"}, "filename", 2); err != nil {
		t.Fatalf("save skipped code: %v", err)
	}
	if err := repo.SetOverride(skipped, OverrideSkip, ""); err != nil {
		t.Fatalf("skip video: %v", err)
	}

	codes, err := repo.ListCodes()
	if err != nil {
		t.Fatalf("list codes: %v", err)
	}
	if got := codes[scraped]; got != "IPX-633" {
		t.Errorf("scraped code = %q, want trimmed IPX-633", got)
	}
	if _, ok := codes[unscraped]; ok {
		t.Error("a video with no scrape result must not carry a code")
	}
	if _, ok := codes[skipped]; ok {
		t.Error("a skipped video must not carry a code")
	}
}

// TestTrashedVideosAreHiddenFromJavReads pins the recycle-bin boundary for the
// scrape domain: a trashed favorite keeps its metadata for a restore, but must
// not appear in the task board, duplicate worklist, actor index or retry scan.
func TestTrashedVideosAreHiddenFromJavReads(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewSQLiteRepository(db)
	videos := video.NewSQLiteRepository(db)
	create := func(name string) *video.Video {
		t.Helper()
		v := &video.Video{
			Path:      filepath.Join(t.TempDir(), name),
			FileName:  name,
			FileSize:  1,
			CreatedAt: 1,
			UpdatedAt: 1,
			Status:    video.StatusAvailable,
		}
		if err := videos.Create(v); err != nil {
			t.Fatalf("create favorite %q: %v", name, err)
		}
		if err := repo.PrepareTask(v.ID, false); err != nil {
			t.Fatalf("prepare task %q: %v", name, err)
		}
		if err := repo.SaveSuccess(v.ID, Metadata{Code: "ABC-123", Actors: []string{"演员甲"}}, "filename", 2); err != nil {
			t.Fatalf("save success %q: %v", name, err)
		}
		return v
	}
	active := create("active.mp4")
	trashed := create("trashed.mp4")
	if err := videos.Trash(trashed.ID, video.TrashReasonUser, 1000); err != nil {
		t.Fatalf("trash: %v", err)
	}

	codes, err := repo.ListCodes()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := codes[trashed.ID]; ok {
		t.Error("trashed favorite still contributes a code")
	}
	if _, ok := codes[active.ID]; !ok {
		t.Error("active favorite lost its code")
	}

	overviews, err := repo.ListTaskOverviews()
	if err != nil {
		t.Fatal(err)
	}
	if len(overviews) != 1 || overviews[0].Video.ID != active.ID {
		t.Errorf("task overviews = %+v, want only the active video", overviews)
	}

	duplicates, err := repo.ListDuplicateCandidates()
	if err != nil {
		t.Fatal(err)
	}
	if len(duplicates) != 1 || duplicates[0].Video.ID != active.ID {
		t.Errorf("duplicate candidates = %+v, want only the active video", duplicates)
	}

	refs, err := repo.ListActorFilmRefs()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].VideoID != active.ID {
		t.Errorf("actor refs = %+v, want only the active video", refs)
	}

	ids, err := repo.VideoIDsByActorNames([]string{"演员甲"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != active.ID {
		t.Errorf("actor videos = %v, want [%d]", ids, active.ID)
	}

	pending, err := repo.RecoverPending(1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range pending {
		if task.VideoID == trashed.ID {
			t.Errorf("trashed favorite is still in the retry scan: %+v", task)
		}
	}
}
