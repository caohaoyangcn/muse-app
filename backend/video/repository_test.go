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

func TestRepositoryRemoteCRUDAndSearch(t *testing.T) {
	repo := newTestRepo(t)
	local := sampleVideo(`D:\V\local.mp4`)
	if err := repo.Create(local); err != nil {
		t.Fatal(err)
	}
	remote := sampleVideo("cloud://7/abc123")
	remote.FileName = "远程影片.mp4"
	remote.SourceID = 7
	remote.RemotePath = "/媒体/远程影片.mp4"
	if err := repo.Create(remote); err != nil {
		t.Fatalf("create remote: %v", err)
	}

	got, err := repo.GetByRemote(7, "/媒体/远程影片.mp4")
	if err != nil {
		t.Fatalf("get by remote: %v", err)
	}
	if got.ID != remote.ID || got.SourceID != 7 || got.RemotePath != "/媒体/远程影片.mp4" {
		t.Errorf("remote row roundtrip = %+v", got)
	}
	if _, err := repo.GetByRemote(7, "/媒体/REMOTE.mp4"); err != ErrNotFound {
		t.Errorf("case-different remote path: err = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetByRemote(8, "/媒体/远程影片.mp4"); err != ErrNotFound {
		t.Errorf("other connection: err = %v, want ErrNotFound", err)
	}

	// Search matches both the display name and the remote path.
	for _, keyword := range []string{"远程影片", "媒体"} {
		hits, err := repo.Search(keyword)
		if err != nil {
			t.Fatalf("search %q: %v", keyword, err)
		}
		if len(hits) != 1 || hits[0].ID != remote.ID {
			t.Errorf("search %q hits = %+v", keyword, hits)
		}
	}

	// The same (source, path) is a duplicate; a case-different path is not.
	dup := sampleVideo("cloud://7/other")
	dup.SourceID = 7
	dup.RemotePath = "/媒体/远程影片.mp4"
	if err := repo.Create(dup); err == nil {
		t.Error("exact duplicate remote path must fail")
	}
	caseTwin := sampleVideo("cloud://7/def456")
	caseTwin.SourceID = 7
	caseTwin.RemotePath = "/媒体/远程影片.MP4"
	if err := repo.Create(caseTwin); err != nil {
		t.Errorf("case-different remote path must be allowed: %v", err)
	}
}

func TestRepositoryRemoteExcludedFromLocalScans(t *testing.T) {
	repo := newTestRepo(t)

	local := sampleVideo(`D:\V\local.mp4`)
	if err := repo.Create(local); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateFingerprint(local.ID, "samehash", 1, local.FileSize, 99, 100); err != nil {
		t.Fatal(err)
	}

	remote := sampleVideo("cloud://7/remote")
	remote.FileName = "remote.mp4"
	remote.SourceID = 7
	remote.RemotePath = "/Media/remote.mp4"
	if err := repo.Create(remote); err != nil {
		t.Fatal(err)
	}
	// Deliberately give the remote row a fingerprint-shaped hash: even if one
	// leaked in through an older build, every local-only query must ignore it.
	if err := repo.UpdateFingerprint(remote.ID, "samehash", 1, remote.FileSize, 1, 1); err != nil {
		t.Fatal(err)
	}

	need, err := repo.ListNeedingFingerprint(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(need) != 0 {
		t.Errorf("ListNeedingFingerprint included remote/local rows: %v", need)
	}
	peers, err := repo.ListSameSizePeers(remote.FileSize, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range peers {
		if id == remote.ID {
			t.Errorf("ListSameSizePeers included remote id %d", id)
		}
	}
	candidates, err := repo.ListRelocationCandidates(remote.FileSize, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].ID != local.ID {
		t.Errorf("relocation candidates = %+v, want only local id %d", candidates, local.ID)
	}
	fingerprinted, err := repo.ListFingerprinted()
	if err != nil {
		t.Fatal(err)
	}
	if len(fingerprinted) != 1 || fingerprinted[0].ID != local.ID {
		t.Errorf("ListFingerprinted = %+v, want only local id %d", fingerprinted, local.ID)
	}
	byHash, err := repo.ListByContentHash("samehash")
	if err != nil {
		t.Fatal(err)
	}
	if len(byHash) != 1 || byHash[0].ID != local.ID {
		t.Errorf("ListByContentHash = %+v, want only local id %d", byHash, local.ID)
	}
}

// TestRepositoryTrashSoftDelete covers the recycle-bin storage contract:
// trashing hides the row from active reads, keeps it available to the trash
// reads and to path lookups (the unique key still belongs to it), and restoring
// brings it back unchanged.
func TestRepositoryTrashSoftDelete(t *testing.T) {
	repo := newTestRepo(t)
	v := sampleVideo(`D:\Videos\trash.mp4`)
	if err := repo.Create(v); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.Trash(v.ID, TrashReasonUser, 5000); err != nil {
		t.Fatalf("trash: %v", err)
	}
	if _, err := repo.Get(v.ID); err != ErrNotFound {
		t.Errorf("trashed row must be hidden from Get: %v", err)
	}
	all, err := repo.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("active list = %d rows, want 0", len(all))
	}
	found, err := repo.Search("trash")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("active search = %d rows, want 0", len(found))
	}

	any, err := repo.GetAny(v.ID)
	if err != nil {
		t.Fatalf("get any: %v", err)
	}
	if any.DeletedAt != 5000 || any.DeleteReason != TrashReasonUser {
		t.Errorf("trashed row = deletedAt %d reason %q", any.DeletedAt, any.DeleteReason)
	}
	byPath, err := repo.GetByPath(`D:\VIDEOS\TRASH.MP4`)
	if err != nil {
		t.Fatalf("get by path must see the trashed row: %v", err)
	}
	if byPath.ID != v.ID {
		t.Errorf("path lookup id = %d, want %d", byPath.ID, v.ID)
	}

	trash, err := repo.ListTrashed()
	if err != nil {
		t.Fatalf("list trashed: %v", err)
	}
	if len(trash) != 1 || trash[0].ID != v.ID {
		t.Fatalf("trash = %+v", trash)
	}
	// The image sweep needs every id, including trashed ones, or it would
	// delete the cover the trash is still displaying.
	ids, err := repo.ListVideoIDs()
	if err != nil {
		t.Fatalf("list ids: %v", err)
	}
	if len(ids) != 1 || ids[0] != v.ID {
		t.Errorf("all ids = %v, want the trashed row", ids)
	}

	// Trashing twice must not reset the retention clock.
	if err := repo.Trash(v.ID, TrashReasonMissing, 9000); err != nil {
		t.Fatalf("second trash: %v", err)
	}
	any, _ = repo.GetAny(v.ID)
	if any.DeletedAt != 5000 || any.DeleteReason != TrashReasonUser {
		t.Errorf("second trash changed = (%d,%q)", any.DeletedAt, any.DeleteReason)
	}

	if err := repo.Restore(v.ID, 10000); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored, err := repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get after restore: %v", err)
	}
	if restored.DeletedAt != 0 || restored.DeleteReason != "" {
		t.Errorf("restored row = (%d,%q)", restored.DeletedAt, restored.DeleteReason)
	}
}

// TestRepositoryPurgeReleasesPath verifies that a permanent delete frees the
// unique path, which is what lets the user re-add a file they really deleted.
func TestRepositoryPurgeReleasesPath(t *testing.T) {
	repo := newTestRepo(t)
	v := sampleVideo(`D:\Videos\again.mp4`)
	if err := repo.Create(v); err != nil {
		t.Fatal(err)
	}
	if err := repo.Trash(v.ID, TrashReasonUser, 1000); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetAny(v.ID); err != ErrNotFound {
		t.Fatalf("purged row still readable: %v", err)
	}
	if err := repo.Create(sampleVideo(`D:\Videos\again.mp4`)); err != nil {
		t.Errorf("path should be free after purge: %v", err)
	}
}
