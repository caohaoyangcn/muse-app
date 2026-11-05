package video

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrashMovesFavoriteAndEmits(t *testing.T) {
	svc, dir := newTestService(t)
	path := writeFakeVideo(t, dir, "remove-me.mp4", 32)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	svc.emit = func(name string, data ...any) {
		events = append(events, name)
	}

	if err := svc.Trash(v.ID, TrashReasonUser); err != nil {
		t.Fatalf("trash: %v", err)
	}
	if _, err := svc.Get(v.ID); err != ErrNotFound {
		t.Errorf("trashed favorite visible in Get: %v", err)
	}
	list, err := svc.ListTrash()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != v.ID || list[0].DeleteReason != TrashReasonUser {
		t.Fatalf("trash = %+v", list)
	}
	if len(events) != 1 || events[0] != "video:trashed" {
		t.Errorf("events = %v, want one video:trashed", events)
	}
	// The source file is untouched by a removal.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("source file was touched: %v", err)
	}

	// Trashing twice is idempotent and does not announce again.
	if err := svc.Trash(v.ID, TrashReasonUser); err != nil {
		t.Fatalf("second trash: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("second trash emitted again: %v", events)
	}
	// A stale card whose record is already gone still succeeds, so the client
	// can drop it.
	if err := svc.Trash(999, TrashReasonUser); err != nil {
		t.Errorf("stale trash: %v", err)
	}
}

func TestRestoreTrashedFavoriteIsTheSameRecord(t *testing.T) {
	svc, dir := newTestService(t)
	path := writeFakeVideo(t, dir, "restore-me.mp4", 64)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.UpdateInfo(v.ID, "我的标题", "备注"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LikeVideo(v.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Trash(v.ID, TrashReasonUser); err != nil {
		t.Fatal(err)
	}

	var restored bool
	svc.emit = func(name string, data ...any) {
		if name == "video:restored" {
			restored = true
		}
	}
	if err := svc.Restore(v.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := svc.Get(v.ID)
	if err != nil {
		t.Fatalf("get restored: %v", err)
	}
	if got.Title != "我的标题" || got.Notes != "备注" || got.LikeCount != 1 {
		t.Errorf("user-owned data lost: %+v", got)
	}
	if got.Status != StatusAvailable {
		t.Errorf("status = %q, want available while the file exists", got.Status)
	}
	if !restored {
		t.Error("restore did not announce video:restored")
	}
}

func TestRestoreKeepsMissingStatusWhenTheFileIsGone(t *testing.T) {
	svc, dir := newTestService(t)
	path := writeFakeVideo(t, dir, "gone.mp4", 64)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := svc.Trash(v.ID, TrashReasonMissing); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore(v.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := svc.Get(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusMissing {
		t.Errorf("status = %q, want missing", got.Status)
	}
}

func TestAddSamePathRestoresTrashedFavorite(t *testing.T) {
	svc, dir := newTestService(t)
	path := writeFakeVideo(t, dir, "re-add.mp4", 64)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.UpdateInfo(v.ID, "保留标题", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.Trash(v.ID, TrashReasonUser); err != nil {
		t.Fatal(err)
	}

	again, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if again.ID != v.ID {
		t.Errorf("re-add created a new favorite %d, want original %d", again.ID, v.ID)
	}
	if again.Title != "保留标题" {
		t.Errorf("original metadata not restored: %+v", again)
	}
	if again.DeletedAt != 0 {
		t.Errorf("restored row still trashed: %+v", again)
	}
}

func TestMissingCleanupEntersTrash(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	path := writeFakeVideo(t, dir, "vanished.mp4", 32)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	svc.RefreshAllStatuses()

	trash, err := svc.ListTrash()
	if err != nil {
		t.Fatal(err)
	}
	if len(trash) != 1 || trash[0].ID != v.ID || trash[0].DeleteReason != TrashReasonMissing {
		t.Fatalf("trash after missing cleanup = %+v", trash)
	}
}

func TestMovedFileRelinksMissingTrash(t *testing.T) {
	svc, dir := newTestService(t)
	content := pattern(21, 8192)
	old := seedMissingFavorite(t, svc, writeVideoAt(t, filepath.Join(dir, "old"), "movie.mp4", content))
	if err := svc.Trash(old.ID, TrashReasonMissing); err != nil {
		t.Fatalf("trash missing favorite: %v", err)
	}
	moved := writeVideoAt(t, filepath.Join(dir, "new"), "movie.mp4", content)
	got, err := svc.AddVideo(moved)
	if err != nil {
		t.Fatalf("add moved file: %v", err)
	}
	if got.ID != old.ID {
		t.Errorf("id = %d, want trashed favorite %d", got.ID, old.ID)
	}
	if got.DeletedAt != 0 {
		t.Errorf("relinked favorite still trashed: %+v", got)
	}
	if n := favoriteCount(t, svc); n != 1 {
		t.Errorf("favorites = %d, want 1", n)
	}
}

func TestPurgeTrashedFavoriteRemovesOnlyMuseData(t *testing.T) {
	svc, dir := newTestService(t)
	path := writeFakeVideo(t, dir, "purge.mp4", 32)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatal(err)
	}
	thumb := filepath.Join(dir, "thumbnails", "1.jpg")
	if err := os.MkdirAll(filepath.Dir(thumb), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(thumb, []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.UpdateThumbnail(v.ID, "thumbnails/1.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Trash(v.ID, TrashReasonUser); err != nil {
		t.Fatal(err)
	}
	if err := svc.Remove(v.ID); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if _, err := svc.repo.GetAny(v.ID); err != ErrNotFound {
		t.Errorf("purged row still present: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("source video must survive a purge: %v", err)
	}
	if _, err := os.Stat(thumb); !os.IsNotExist(err) {
		t.Errorf("generated thumbnail must be removed, stat err = %v", err)
	}
}
