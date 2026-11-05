package video

import (
	pathpkg "path"
	"strings"
	"testing"

	"muse-app/backend/database"
)

func newTestRepo(t *testing.T) VideoRepository {
	t.Helper()
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewSQLiteRepository(db)
}

func sampleVideo(path string) *Video {
	return &Video{
		Path:      path,
		FileName:  pathpkg.Base(strings.ReplaceAll(path, `\`, "/")),
		FileSize:  1234,
		CreatedAt: 1000,
		UpdatedAt: 1000,
		Status:    StatusAvailable,
	}
}

func TestRepositoryCRUD(t *testing.T) {
	repo := newTestRepo(t)

	v := sampleVideo(`D:\Videos\a.mp4`)
	if err := repo.Create(v); err != nil {
		t.Fatalf("create: %v", err)
	}
	if v.ID == 0 {
		t.Fatal("expected id to be set")
	}

	got, err := repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.FileName != "a.mp4" || got.Status != StatusAvailable {
		t.Errorf("got %+v", got)
	}

	if _, err := repo.Get(999); err != ErrNotFound {
		t.Errorf("get missing: err = %v, want ErrNotFound", err)
	}

	if err := repo.UpdateMetadata(v.ID, 7423420, 3840, 2160, 23.976, 8_000_000, "hevc", "aac"); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	if err := repo.UpdateThumbnail(v.ID, "thumbnails/1.jpg"); err != nil {
		t.Fatalf("update thumbnail: %v", err)
	}
	if err := repo.UpdateLastOpened(v.ID, 2000); err != nil {
		t.Fatalf("update last opened: %v", err)
	}
	if err := repo.UpdateStatus(v.ID, StatusMissing); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, _ = repo.Get(v.ID)
	if got.DurationMs != 7423420 || got.Width != 3840 || got.Height != 2160 || got.FrameRate != 23.976 || got.Bitrate != 8_000_000 {
		t.Errorf("metadata not persisted: %+v", got)
	}
	if got.ThumbnailPath != "thumbnails/1.jpg" || got.LastOpenedAt != 2000 || got.Status != StatusMissing {
		t.Errorf("updates not persisted: %+v", got)
	}

	if err := repo.Delete(v.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(v.ID); err != ErrNotFound {
		t.Errorf("after delete: err = %v, want ErrNotFound", err)
	}
}

func TestRepositoryDuplicatePath(t *testing.T) {
	repo := newTestRepo(t)
	if err := repo.Create(sampleVideo(`D:\Videos\a.mp4`)); err != nil {
		t.Fatal(err)
	}
	// COLLATE NOCASE: same path in different case must violate the unique key.
	if err := repo.Create(sampleVideo(`D:\VIDEOS\A.MP4`)); err == nil {
		t.Error("expected unique constraint violation for case-insensitive duplicate")
	}
}

func TestRepositoryGetByPath(t *testing.T) {
	repo := newTestRepo(t)
	if err := repo.Create(sampleVideo(`D:\Videos\a.mp4`)); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByPath(`D:\VIDEOS\A.mp4`)
	if err != nil {
		t.Fatalf("get by path (nocase): %v", err)
	}
	if got.Path != `D:\Videos\a.mp4` {
		t.Errorf("path = %q", got.Path)
	}
	if _, err := repo.GetByPath(`D:\Videos\other.mp4`); err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRepositoryListOrderAndSearch(t *testing.T) {
	repo := newTestRepo(t)
	for _, p := range []string{`D:\V\first.mp4`, `D:\V\second.mp4`, `D:\Other\interstellar.mkv`} {
		if err := repo.Create(sampleVideo(p)); err != nil {
			t.Fatal(err)
		}
	}

	all, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("list = %d videos, want 3", len(all))
	}
	// Equal created_at: tie-break by id DESC keeps insertion-reversed order stable.
	if all[0].FileName != "interstellar.mkv" || all[2].FileName != "first.mp4" {
		t.Errorf("unexpected order: %s, %s, %s", all[0].FileName, all[1].FileName, all[2].FileName)
	}

	hits, err := repo.Search("interstellar")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].FileName != "interstellar.mkv" {
		t.Errorf("search hits = %+v", hits)
	}

	// Search must not treat % or _ as wildcards.
	hits, err = repo.Search("a_b")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("escaped search should match nothing, got %+v", hits)
	}
}

func TestRepositoryListNeedingMetadata(t *testing.T) {
	repo := newTestRepo(t)

	// No metadata at all -> backfill candidate.
	a := sampleVideo(`D:\V\a.mp4`)
	if err := repo.Create(a); err != nil {
		t.Fatal(err)
	}
	// Has duration + frame rate -> already backfilled, not a candidate.
	b := sampleVideo(`D:\V\b.mp4`)
	if err := repo.Create(b); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(b.ID, 5000, 1920, 1080, 23.976, 4_000_000, "h264", "aac"); err != nil {
		t.Fatal(err)
	}

	ids, err := repo.ListNeedingMetadata()
	if err != nil {
		t.Fatalf("list needing metadata: %v", err)
	}
	if len(ids) != 1 || ids[0] != a.ID {
		t.Errorf("backfill targets = %v, want only id %d", ids, a.ID)
	}
}

func TestRepositoryLikes(t *testing.T) {
	repo := newTestRepo(t)
	v := sampleVideo(`D:\V\liked.mp4`)
	if err := repo.Create(v); err != nil {
		t.Fatal(err)
	}

	// A fresh video starts at zero likes.
	got, err := repo.Get(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LikeCount != 0 {
		t.Errorf("initial like count = %d, want 0", got.LikeCount)
	}

	// Repeated likes stack: each call adds one.
	for want := int64(1); want <= 3; want++ {
		count, err := repo.IncrementLikes(v.ID)
		if err != nil {
			t.Fatalf("increment: %v", err)
		}
		if count != want {
			t.Errorf("count = %d, want %d", count, want)
		}
	}
	got, _ = repo.Get(v.ID)
	if got.LikeCount != 3 {
		t.Errorf("persisted like count = %d, want 3", got.LikeCount)
	}

	// Clear resets to zero.
	if err := repo.ClearLikes(v.ID); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, _ = repo.Get(v.ID)
	if got.LikeCount != 0 {
		t.Errorf("like count after clear = %d, want 0", got.LikeCount)
	}

	// Unknown ids report ErrNotFound.
	if _, err := repo.IncrementLikes(999); err != ErrNotFound {
		t.Errorf("increment missing: err = %v, want ErrNotFound", err)
	}
	if err := repo.ClearLikes(999); err != ErrNotFound {
		t.Errorf("clear missing: err = %v, want ErrNotFound", err)
	}
}
