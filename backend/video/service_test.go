package video

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"muse-app/backend/database"
	"muse-app/backend/thumbnail"
)

// newTestService wires a real SQLite repo with a one-worker processor whose
// ffmpeg tools are empty strings, so background jobs fail harmlessly (and
// quickly) while AddVideo logic is exercised.
func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	dataDir := t.TempDir()
	db, _, err := database.Open(dataDir)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewSQLiteRepository(db)
	processor := NewProcessor(repo, "", "", filepath.Join(dataDir, "thumbnails"), nil, slog.Default(), 1)
	return NewService(repo, processor, dataDir, slog.Default(), nil), dataDir
}

func writeFakeVideo(t *testing.T, dir, name string, size int) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestServiceAddVideo(t *testing.T) {
	svc, dir := newTestService(t)

	p := writeFakeVideo(t, dir, "movie.mp4", 100)
	v, err := svc.AddVideo(p)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if v.Path != p || v.FileName != "movie.mp4" || v.FileSize != 100 || v.Status != StatusAvailable {
		t.Errorf("added video = %+v", v)
	}
	if v.DurationMs != 0 || v.ThumbnailPath != "" {
		t.Errorf("metadata/thumbnail should be empty right after add: %+v", v)
	}
}

func TestServiceAddVideoRejections(t *testing.T) {
	svc, dir := newTestService(t)
	writeFakeVideo(t, dir, "ok.mp4", 10)
	writeFakeVideo(t, dir, "nope.txt", 10)

	if _, err := svc.AddVideo(filepath.Join(dir, "gone.mp4")); err != ErrNotExist {
		t.Errorf("missing file: %v", err)
	}
	if _, err := svc.AddVideo(filepath.Join(dir, "nope.txt")); err != ErrUnsupportedType {
		t.Errorf("txt file: err = %v, want ErrUnsupportedType", err)
	}
	if _, err := svc.AddVideo(dir); err != ErrFolderNotSupported {
		t.Errorf("directory: %v", err)
	}
	if _, err := svc.AddVideo("   "); err != ErrEmptyPath {
		t.Errorf("empty: %v", err)
	}
}

func TestServiceDuplicate(t *testing.T) {
	svc, dir := newTestService(t)
	p := writeFakeVideo(t, dir, "dup.mp4", 10)

	if _, err := svc.AddVideo(p); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVideo(p); err != ErrAlreadyAdded {
		t.Errorf("second add: err = %v, want ErrAlreadyAdded", err)
	}
	list, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 record, got %d", len(list))
	}
}

func TestServiceAddVideosPartialFailure(t *testing.T) {
	svc, dir := newTestService(t)
	p1 := writeFakeVideo(t, dir, "a.mp4", 10)
	p2 := writeFakeVideo(t, dir, "b.txt", 10)

	results := svc.AddVideos([]string{p1, p2, p1})
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if !results[0].Success || results[0].VideoID == 0 {
		t.Errorf("results[0] = %+v", results[0])
	}
	if results[1].Success || results[1].Error == "" {
		t.Errorf("results[1] = %+v", results[1])
	}
	if results[2].Success || results[2].Error == "" {
		t.Errorf("results[2] = %+v", results[2])
	}
}

func TestServiceRemoveKeepsOriginalFile(t *testing.T) {
	svc, dir := newTestService(t)
	p := writeFakeVideo(t, dir, "keepme.mp4", 10)

	v, err := svc.AddVideo(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Remove(v.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("original file must survive removal: %v", err)
	}
	if _, err := svc.Get(v.ID); err != ErrNotFound {
		t.Errorf("record should be gone, err = %v", err)
	}
}

// Removing a favorite has to tell the frontend: a list open in this window, and
// the phone showing the same card, must drop it without waiting for the next
// read. The "already gone" path announces it too, because the caller's copy is
// stale whether this process deleted the row or another one did.
func TestServiceRemoveEmitsRemoval(t *testing.T) {
	svc, dir := newTestService(t)
	p := writeFakeVideo(t, dir, "announce.mp4", 10)

	v, err := svc.AddVideo(p)
	if err != nil {
		t.Fatal(err)
	}

	var announced []int64
	svc.emit = func(name string, data ...any) {
		if name != "video:removed" || len(data) != 1 {
			return
		}
		payload, ok := data[0].(map[string]any)
		if !ok {
			return
		}
		if id, ok := payload["id"].(int64); ok {
			announced = append(announced, id)
		}
	}

	if err := svc.Remove(v.ID); err != nil {
		t.Fatalf("first remove: %v", err)
	}
	if err := svc.Remove(v.ID); err != nil {
		t.Fatalf("removing an already removed favorite must succeed: %v", err)
	}
	if len(announced) != 2 || announced[0] != v.ID || announced[1] != v.ID {
		t.Errorf("removal events = %v, want [%d %d]", announced, v.ID, v.ID)
	}
}

// Removing a favorite whose file is gone is a normal removal: the reference is
// what is being deleted, and the bytes were never the app's to check.
func TestServiceRemoveMissingFile(t *testing.T) {
	svc, dir := newTestService(t)
	p := writeFakeVideo(t, dir, "gone.mp4", 10)

	v, err := svc.AddVideo(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RefreshStatus(v.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Remove(v.ID); err != nil {
		t.Fatalf("remove a favorite whose file is missing: %v", err)
	}
	if _, err := svc.Get(v.ID); err != ErrNotFound {
		t.Errorf("record should be gone, err = %v", err)
	}
}

// A stale card (the record was removed elsewhere) must not come back as an
// error: the user asked for it to be gone, and it already is. Anything that
// still describes it is cleaned up.
func TestServiceRemoveAlreadyGone(t *testing.T) {
	svc, dataDir := newTestService(t)
	p := writeFakeVideo(t, dataDir, "twice.mp4", 10)

	v, err := svc.AddVideo(p)
	if err != nil {
		t.Fatal(err)
	}
	// Give the record a generated thumbnail, then remove it twice.
	thumbDir := filepath.Join(dataDir, thumbnail.RelativeDir)
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	thumbFile := filepath.Join(thumbDir, strconv.FormatInt(v.ID, 10)+".jpg")
	if err := os.WriteFile(thumbFile, []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := svc.Remove(v.ID); err != nil {
		t.Fatalf("first remove: %v", err)
	}
	// The first removal deleted the file; a second one still answers success.
	if _, err := os.Stat(thumbFile); !os.IsNotExist(err) {
		t.Errorf("thumbnail should be gone, stat err = %v", err)
	}
	if err := svc.Remove(v.ID); err != nil {
		t.Errorf("removing an already removed favorite must succeed, got %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("original file must survive both removals: %v", err)
	}
}

// A stale removal of an id that never existed is also a no-op success, and it
// must not leave a thumbnail behind.
func TestServiceRemoveUnknownID(t *testing.T) {
	svc, dataDir := newTestService(t)
	thumbDir := filepath.Join(dataDir, thumbnail.RelativeDir)
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	thumbFile := filepath.Join(thumbDir, "999.jpg")
	if err := os.WriteFile(thumbFile, []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := svc.Remove(999); err != nil {
		t.Fatalf("remove unknown id: %v", err)
	}
	if _, err := os.Stat(thumbFile); !os.IsNotExist(err) {
		t.Errorf("orphan thumbnail should be gone, stat err = %v", err)
	}
}

func TestServiceMissingDetection(t *testing.T) {
	svc, dir := newTestService(t)
	p := writeFakeVideo(t, dir, "doomed.mp4", 10)

	v, _ := svc.AddVideo(p)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}

	// Open on a missing file must fail and flip status.
	if err := svc.Open(v.ID); err == nil {
		t.Error("open on missing file should fail")
	}
	got, _ := svc.Get(v.ID)
	if got.Status != StatusMissing {
		t.Errorf("status = %q, want missing", got.Status)
	}

	// RefreshStatus re-checks and reports the current state.
	got, err := svc.RefreshStatus(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusMissing {
		t.Errorf("refresh status = %q, want missing", got.Status)
	}
}

func TestServiceSearch(t *testing.T) {
	svc, dir := newTestService(t)
	p1 := writeFakeVideo(t, dir, "interstellar.mkv", 10)
	writeFakeVideo(t, dir, "up.mp4", 10)
	if _, err := svc.AddVideo(p1); err != nil {
		t.Fatal(err)
	}

	hits, err := svc.Search("stel")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].FileName != "interstellar.mkv" {
		t.Errorf("search hits = %+v", hits)
	}
}

func TestServiceUpdateInfo(t *testing.T) {
	svc, dir := newTestService(t)
	p := writeFakeVideo(t, dir, "movie.mp4", 10)
	v, err := svc.AddVideo(p)
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.UpdateInfo(v.ID, "  我的电影  ", "备注\n第二行")
	if err != nil {
		t.Fatalf("update info: %v", err)
	}
	if got.Title != "我的电影" || got.Notes != "备注\n第二行" {
		t.Errorf("got title=%q notes=%q", got.Title, got.Notes)
	}

	// The custom title is visible in the full list too (scan path).
	list, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Title != "我的电影" {
		t.Errorf("list title = %q", list[0].Title)
	}

	// file_name stays immutable; an empty title means "show the file name".
	got, err = svc.UpdateInfo(v.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "" || got.FileName != "movie.mp4" {
		t.Errorf("got title=%q fileName=%q", got.Title, got.FileName)
	}

	if _, err := svc.UpdateInfo(999, "x", ""); err != ErrNotFound {
		t.Errorf("missing id: err = %v, want ErrNotFound", err)
	}
}

func TestServiceLikes(t *testing.T) {
	svc, dir := newTestService(t)
	p := writeFakeVideo(t, dir, "movie.mp4", 10)
	v, err := svc.AddVideo(p)
	if err != nil {
		t.Fatal(err)
	}

	// Repeated likes stack one at a time.
	for want := int64(1); want <= 3; want++ {
		got, err := svc.LikeVideo(v.ID)
		if err != nil {
			t.Fatalf("like: %v", err)
		}
		if got.LikeCount != want {
			t.Errorf("like count = %d, want %d", got.LikeCount, want)
		}
	}

	// Clear resets to zero and returns the fresh record.
	got, err := svc.ClearVideoLikes(v.ID)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got.LikeCount != 0 {
		t.Errorf("like count after clear = %d, want 0", got.LikeCount)
	}

	// Clearing a video with no likes is a harmless no-op.
	got, err = svc.ClearVideoLikes(v.ID)
	if err != nil {
		t.Fatalf("clear empty: %v", err)
	}
	if got.LikeCount != 0 {
		t.Errorf("like count = %d, want 0", got.LikeCount)
	}

	// Missing id is reported.
	if _, err := svc.LikeVideo(999); err != ErrNotFound {
		t.Errorf("like missing: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.ClearVideoLikes(999); err != ErrNotFound {
		t.Errorf("clear missing: err = %v, want ErrNotFound", err)
	}
}

func TestServiceResetCover(t *testing.T) {
	svc, dir := newTestService(t)
	p := writeFakeVideo(t, dir, "movie.mp4", 10)
	v, err := svc.AddVideo(p)
	if err != nil {
		t.Fatal(err)
	}

	// Resetting an automatic cover is a no-op.
	got, err := svc.ResetCover(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CustomCoverAt != 0 {
		t.Errorf("reset on auto cover changed state: %+v", got)
	}

	// Marking custom (simulating a prior SetCover) then resetting clears the
	// marker; the thumbnail regeneration itself fails harmlessly because the
	// test processor has no ffmpeg.
	if err := svc.repo.SetCustomCover(v.ID, 12345); err != nil {
		t.Fatal(err)
	}
	got, err = svc.ResetCover(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CustomCoverAt != 0 {
		t.Errorf("custom marker not cleared: %+v", got)
	}

	// SetCover without an ffmpeg binary reports the tool error.
	if _, err := svc.SetCover(v.ID, 1.5); err == nil {
		t.Error("expected SetCover to fail without ffmpeg")
	}
}

func TestServiceBackfillProcessingIncludesMissingThumbnail(t *testing.T) {
	svc, dir := newTestService(t)
	p := writeFakeVideo(t, dir, "movie.mp4", 10)
	v := &Video{
		Path:      p,
		FileName:  filepath.Base(p),
		FileSize:  10,
		CreatedAt: 1,
		UpdatedAt: 1,
		Status:    StatusAvailable,
	}
	if err := svc.repo.Create(v); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.UpdateMetadata(v.ID, 2000, 320, 240, 25, 1000, "h264", "aac"); err != nil {
		t.Fatal(err)
	}

	if got := svc.BackfillProcessing(); got != 1 {
		t.Fatalf("backfill count = %d, want 1", got)
	}
}

func TestServiceAddRemoteVideo(t *testing.T) {
	svc, _ := newTestService(t)
	v, created, err := svc.AddRemoteVideo(7, "/媒体/a.mp4", "a.mp4", 2048)
	if err != nil {
		t.Fatalf("add remote: %v", err)
	}
	if !created || v.SourceID != 7 || v.RemotePath != "/媒体/a.mp4" || v.FileName != "a.mp4" {
		t.Errorf("remote video = %+v created=%v", v, created)
	}
	if v.Path != RemotePathKey(7, "/媒体/a.mp4") || !strings.HasPrefix(v.Path, "cloud://7/") {
		t.Errorf("synthetic path = %q", v.Path)
	}
	// Same path is a duplicate; a case-different WebDAV path is another file.
	if _, _, err := svc.AddRemoteVideo(7, "/媒体/a.mp4", "a.mp4", 2048); err != ErrAlreadyAdded {
		t.Errorf("duplicate err = %v, want ErrAlreadyAdded", err)
	}
	if _, _, err := svc.AddRemoteVideo(7, "/媒体/A.MP4", "A.MP4", 10); err != nil {
		t.Errorf("case-different remote path: %v", err)
	}
	if _, _, err := svc.AddRemoteVideo(7, "relative.mp4", "relative.mp4", 10); err != ErrInvalidRemotePath {
		t.Errorf("relative path err = %v", err)
	}
	if _, _, err := svc.AddRemoteVideo(7, "/媒体/readme.txt", "readme.txt", 10); err != ErrUnsupportedType {
		t.Errorf("unsupported type err = %v", err)
	}

	// A remote favorite marked missing is revived at its recorded remote path.
	if err := svc.repo.UpdateStatus(v.ID, StatusMissing); err != nil {
		t.Fatal(err)
	}
	revived, created, err := svc.AddRemoteVideo(7, "/媒体/a.mp4", "a.mp4", 2048)
	if err != nil {
		t.Fatalf("revive: %v", err)
	}
	if created || revived.ID != v.ID || revived.Status != StatusAvailable {
		t.Errorf("revived = %+v created=%v", revived, created)
	}
}
