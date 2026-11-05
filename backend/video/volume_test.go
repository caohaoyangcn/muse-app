package video

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestVolumeRoot pins the mount point behind a recorded path: the volume the path
// names when it names one, and the filesystem root otherwise. Both the relocation
// filter and the detached-volume check stat this, and off Windows the root is the
// only thing there is to stat, which is why a path without a volume must not
// yield "nothing to test".
func TestVolumeRoot(t *testing.T) {
	if got, want := VolumeRoot(filepath.Join("/", "Videos", "a.mp4")), string(os.PathSeparator); got != want {
		t.Errorf("VolumeRoot of a path with no volume = %q, want %q", got, want)
	}
	if volume := filepath.VolumeName(os.TempDir()); volume != "" {
		want := volume + string(os.PathSeparator)
		if got := VolumeRoot(filepath.Join(os.TempDir(), "a.mp4")); got != want {
			t.Errorf("VolumeRoot = %q, want %q", got, want)
		}
	}
}

// TestVolumeStampNeverTouchesTheDisk is the cost rule: a read only lifts the
// cached answer onto the record, so listing the library cannot stall on an
// unresponsive drive or share.
func TestVolumeStampNeverTouchesTheDisk(t *testing.T) {
	probes := 0
	state := newVolumeState(func(string) (os.FileInfo, error) {
		probes++
		return fakeFileInfo{}, nil
	})
	path := filepath.Join(t.TempDir(), "a.mp4")

	videos := []Video{{Path: path}}
	state.stamp(videos)
	if probes != 0 {
		t.Errorf("stamping read the disk %d times, want 0", probes)
	}
	if videos[0].VolumeRoot != VolumeRoot(path) {
		t.Errorf("VolumeRoot = %q, want %q", videos[0].VolumeRoot, VolumeRoot(path))
	}
	if videos[0].VolumeOffline {
		t.Error("an unprobed volume must not be reported as detached")
	}
}

// TestVolumeProbeReportsOnlyTransitions pins what the UI is told: the first
// observation is the starting point (not an event), and only a real change of
// state is a change.
func TestVolumeProbeReportsOnlyTransitions(t *testing.T) {
	online := true
	state := newVolumeState(func(string) (os.FileInfo, error) {
		if online {
			return fakeFileInfo{}, nil
		}
		return nil, os.ErrNotExist
	})
	path := filepath.Join(t.TempDir(), "a.mp4")
	state.stamp([]Video{{Path: path}})

	first := state.probe()
	if len(first) != 1 || !first[0].Online || first[0].Changed {
		t.Fatalf("first probe = %+v, want one attached volume, not a change", first)
	}

	online = false
	detached := state.probe()
	if len(detached) != 1 || detached[0].Online || !detached[0].Changed {
		t.Fatalf("probe after unplugging = %+v, want one detached volume, a change", detached)
	}
	if !state.offline(path) {
		t.Error("the volume must be known as detached after the probe")
	}
	if repeated := state.probe(); len(repeated) != 1 || repeated[0].Changed {
		t.Errorf("a repeated probe = %+v, want no change", repeated)
	}

	online = true
	back := state.probe()
	if len(back) != 1 || !back[0].Online || !back[0].Changed {
		t.Fatalf("probe after reattaching = %+v, want one attached volume, a change", back)
	}
}

// TestVolumeRecordLearnsFromOneFile keeps a single-file answer from disagreeing
// with what the grid shows: opening one video tells the cache the drive is back.
func TestVolumeRecordLearnsFromOneFile(t *testing.T) {
	state := newVolumeState(func(string) (os.FileInfo, error) { return fakeFileInfo{}, nil })
	path := filepath.Join(t.TempDir(), "a.mp4")

	state.record(path, false)
	if !state.offline(path) {
		t.Fatal("recorded observation was not kept")
	}
	state.record(path, true)
	if state.offline(path) {
		t.Error("a reattached volume must stop being reported as detached")
	}
}

// TestRefreshAllStatusesLeavesADetachedVolumeAlone is the whole point: a file
// whose disk is not plugged in is not a missing file, so startup must not stamp
// it as one.
func TestRefreshAllStatusesLeavesADetachedVolumeAlone(t *testing.T) {
	svc, dir := newTestService(t)
	path := writeFakeVideo(t, dir, "away.mp4", 10)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	svc.stat = detachedVolumeStub(path)

	svc.RefreshAllStatuses()

	got, err := svc.Get(v.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusAvailable {
		t.Errorf("status = %q, want %q: the drive is away, the file is not missing", got.Status, StatusAvailable)
	}
	if !got.VolumeOffline || got.VolumeRoot == "" {
		t.Errorf("volume = %+v, want the detached volume marked with its root", got)
	}
}

// TestOpenOnADetachedVolumeReportsTheDisk keeps a failed play from rewriting the
// library: the user is told which disk to plug in, and the favorite stays.
func TestOpenOnADetachedVolumeReportsTheDisk(t *testing.T) {
	svc, dir := newTestService(t)
	path := writeFakeVideo(t, dir, "away.mp4", 10)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	svc.stat = detachedVolumeStub(path)

	err = svc.Open(v.ID)
	if err == nil {
		t.Fatal("opening a file on an absent volume must fail")
	}
	if !strings.Contains(err.Error(), "磁盘未接入") {
		t.Errorf("error = %q, want it to name the detached disk", err)
	}
	got, err := svc.repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusAvailable {
		t.Errorf("status = %q, want it untouched", got.Status)
	}
	if !svc.volumes.offline(path) {
		t.Error("the direct observation must teach the cache the disk is away")
	}
}

// TestRefreshVolumesRevivesFavoritesWhenTheDiskReturns covers the other half:
// plugging the drive back in clears the warning and the missing mark without a
// restart.
func TestRefreshVolumesRevivesFavoritesWhenTheDiskReturns(t *testing.T) {
	svc, dir := newTestService(t)
	path := writeFakeVideo(t, dir, "away.mp4", 10)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	online := false
	svc.stat = func(p string) (os.FileInfo, error) {
		if !online && (strings.EqualFold(p, path) || strings.EqualFold(p, VolumeRoot(path))) {
			return nil, os.ErrNotExist
		}
		return os.Stat(p)
	}
	// The row was marked missing before the drive went away (or by an earlier
	// startup); the probe alone must not mark an unavailable file missing.
	if err := svc.repo.UpdateStatus(v.ID, StatusMissing); err != nil {
		t.Fatalf("status: %v", err)
	}
	if _, err := svc.List(); err != nil { // registers the volume
		t.Fatalf("list: %v", err)
	}
	if changes := svc.RefreshVolumes(); len(changes) != 1 || changes[0].Online || changes[0].Changed {
		t.Fatalf("first probe = %+v, want one detached volume", changes)
	}

	online = true
	changes := svc.RefreshVolumes()
	if len(changes) != 1 || !changes[0].Online || !changes[0].Changed {
		t.Fatalf("probe after reattaching = %+v, want one attached volume, a change", changes)
	}
	got, err := svc.repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusAvailable {
		t.Errorf("status = %q, want the favorite reachable again", got.Status)
	}
}

// TestWatchVolumesReportsDetachment is the live path: the poll notices a drive
// going away and emits one event naming it.
func TestWatchVolumesReportsDetachment(t *testing.T) {
	svc, dir := newTestService(t)
	path := writeFakeVideo(t, dir, "away.mp4", 10)
	if _, err := svc.AddVideo(path); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := svc.List(); err != nil { // registers the volume
		t.Fatalf("list: %v", err)
	}

	events := make(chan VolumeChange, 4)
	svc.emit = func(name string, data ...any) {
		if name != "video:volume" || len(data) != 1 {
			return
		}
		payload, ok := data[0].(map[string]any)
		if !ok {
			return
		}
		root, _ := payload["root"].(string)
		online, _ := payload["online"].(bool)
		events <- VolumeChange{Root: root, Online: online}
	}

	// The poll runs on its own goroutine, so the fake disk's state must be
	// synchronized: a plain bool here is a data race under -race.
	var online atomic.Bool
	online.Store(true)
	svc.stat = func(p string) (os.FileInfo, error) {
		if !online.Load() && (strings.EqualFold(p, path) || strings.EqualFold(p, VolumeRoot(path))) {
			return nil, os.ErrNotExist
		}
		return os.Stat(p)
	}
	stop := svc.WatchVolumes(5 * time.Millisecond)
	defer stop()

	// Let the first probe establish that everything is attached, then unplug.
	time.Sleep(20 * time.Millisecond)
	online.Store(false)

	select {
	case change := <-events:
		if change.Online || change.Root != VolumeRoot(path) {
			t.Errorf("event = %+v, want the volume reported detached", change)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the poll never reported the detached volume")
	}
}

// TestRefreshAllStatusesRemovesAMissingFavoriteOnAnAttachedVolume is the new
// contract: a file confirmed gone while its volume is present is not left as a
// broken card — the reference is cleaned up (record + generated assets only).
func TestRefreshAllStatusesRemovesAMissingFavoriteOnAnAttachedVolume(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	path := writeFakeVideo(t, dir, "gone.mp4", 10)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	svc.RefreshAllStatuses()

	if _, err := svc.repo.Get(v.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing favorite still recorded: err = %v, want ErrNotFound", err)
	}
}

// TestRefreshAllStatusesRemovesOnlyTheMissingFavorite keeps a volume trusted
// while at least one favorite is still present: the gone files are cleaned up,
// the present ones stay.
func TestRefreshAllStatusesRemovesOnlyTheMissingFavorite(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	keptPath := writeFakeVideo(t, dir, "kept.mp4", 10)
	gonePath := writeFakeVideo(t, dir, "gone.mp4", 10)
	kept, err := svc.AddVideo(keptPath)
	if err != nil {
		t.Fatalf("add kept: %v", err)
	}
	gone, err := svc.AddVideo(gonePath)
	if err != nil {
		t.Fatalf("add gone: %v", err)
	}
	if err := os.Remove(gonePath); err != nil {
		t.Fatalf("remove: %v", err)
	}

	svc.RefreshAllStatuses()

	if _, err := svc.repo.Get(gone.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing favorite still recorded: err = %v, want ErrNotFound", err)
	}
	still, err := svc.repo.Get(kept.ID)
	if err != nil {
		t.Fatalf("present favorite must stay: %v", err)
	}
	if still.Status != StatusAvailable {
		t.Errorf("status = %q, want %q", still.Status, StatusAvailable)
	}
}

// TestRefreshAllStatusesKeepsAnUnreadableFile pins the difference between "gone"
// and "cannot read": a permission or I/O failure must never destroy a favorite.
func TestRefreshAllStatusesKeepsAnUnreadableFile(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	path := writeFakeVideo(t, dir, "locked.mp4", 10)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	svc.stat = func(p string) (os.FileInfo, error) {
		if strings.EqualFold(p, path) {
			return nil, &os.PathError{Op: "stat", Path: p, Err: os.ErrPermission}
		}
		return os.Stat(p)
	}

	svc.RefreshAllStatuses()

	got, err := svc.repo.Get(v.ID)
	if err != nil {
		t.Fatalf("unreadable favorite must stay: %v", err)
	}
	if got.Status != StatusMissing {
		t.Errorf("status = %q, want %q: unreadable is visible, not deleted", got.Status, StatusMissing)
	}
}

// TestRefreshAllStatusesRefusesToRemoveEveryFavoriteOnAVolume is the guard
// against a reused drive letter (or a mount that is not ready yet): when none
// of a volume's favorites is present, the records stay visible for the user to
// decide instead of being wiped.
func TestRefreshAllStatusesRefusesToRemoveEveryFavoriteOnAVolume(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	firstPath := writeFakeVideo(t, dir, "a.mp4", 10)
	secondPath := writeFakeVideo(t, dir, "b.mp4", 10)
	first, err := svc.AddVideo(firstPath)
	if err != nil {
		t.Fatalf("add first: %v", err)
	}
	second, err := svc.AddVideo(secondPath)
	if err != nil {
		t.Fatalf("add second: %v", err)
	}
	if err := os.Remove(firstPath); err != nil {
		t.Fatalf("remove first: %v", err)
	}
	if err := os.Remove(secondPath); err != nil {
		t.Fatalf("remove second: %v", err)
	}

	var swept map[string]any
	svc.emit = func(name string, data ...any) {
		if name == "video:missing:swept" && len(data) == 1 {
			swept, _ = data[0].(map[string]any)
		}
	}
	svc.RefreshAllStatuses()

	for _, id := range []int64{first.ID, second.ID} {
		got, err := svc.repo.Get(id)
		if err != nil {
			t.Fatalf("favorite %d must be kept: %v", id, err)
		}
		if got.Status != StatusMissing {
			t.Errorf("favorite %d status = %q, want %q", id, got.Status, StatusMissing)
		}
	}
	if swept == nil {
		t.Fatal("the sweep result must be announced so the user knows why nothing was removed")
	}
	if swept["removed"] != 0 || swept["skipped"] != 2 {
		t.Errorf("sweep = %+v, want removed=0 skipped=2", swept)
	}
}

// TestRefreshAllStatusesLeavesMissingRecordsWithoutAnIdentifiableVolume covers
// Unix: every path lives on "/", so an unplugged external drive looks exactly
// like a deleted file. Destructive cleanup must not run there.
func TestRefreshAllStatusesLeavesMissingRecordsWithoutAnIdentifiableVolume(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return false }
	path := writeFakeVideo(t, dir, "gone.mp4", 10)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	svc.RefreshAllStatuses()

	got, err := svc.repo.Get(v.ID)
	if err != nil {
		t.Fatalf("record must be kept when the volume cannot be identified: %v", err)
	}
	if got.Status != StatusMissing {
		t.Errorf("status = %q, want %q", got.Status, StatusMissing)
	}
}

// TestRefreshVolumesRemovesFavoritesThatWentMissingWhileDetached is the live
// path the user asked for: the drive comes back, and the files deleted while it
// was away must disappear from the library without a restart.
func TestRefreshVolumesRemovesFavoritesThatWentMissingWhileDetached(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	path := writeFakeVideo(t, dir, "gone.mp4", 10)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := svc.List(); err != nil { // registers the volume
		t.Fatalf("list: %v", err)
	}

	online := true
	svc.stat = func(p string) (os.FileInfo, error) {
		if !online && (strings.EqualFold(p, path) || strings.EqualFold(p, VolumeRoot(path))) {
			return nil, os.ErrNotExist
		}
		return os.Stat(p)
	}

	// Establish the attached state, then detach.
	if changes := svc.RefreshVolumes(); len(changes) != 1 || !changes[0].Online || changes[0].Changed {
		t.Fatalf("first probe = %+v, want one attached volume, not a change", changes)
	}
	online = false
	if changes := svc.RefreshVolumes(); len(changes) != 1 || changes[0].Online || !changes[0].Changed {
		t.Fatalf("probe after unplugging = %+v, want one detached volume, a change", changes)
	}

	// The user deleted the file while the drive was away.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove file: %v", err)
	}

	online = true
	changes := svc.RefreshVolumes()
	if len(changes) != 1 || !changes[0].Online || !changes[0].Changed {
		t.Fatalf("probe after reattaching = %+v, want one attached volume, a change", changes)
	}
	if _, err := svc.repo.Get(v.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("favorite missing on the returning drive still recorded: err = %v, want ErrNotFound", err)
	}
}

// TestAutoRemovalUsesTheOrchestratedRemover keeps the cleanup on the façade's
// one removal path (groups + scraped metadata), not a bare repository delete.
func TestAutoRemovalUsesTheOrchestratedRemover(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	path := writeFakeVideo(t, dir, "gone.mp4", 10)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	var called []int64
	svc.SetMissingRemover(func(id int64) error {
		called = append(called, id)
		return svc.Remove(id)
	})
	svc.RefreshAllStatuses()

	if len(called) != 1 || called[0] != v.ID {
		t.Errorf("remover called with %v, want [%d]", called, v.ID)
	}
	if _, err := svc.repo.Get(v.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("record = %v, want ErrNotFound", err)
	}
}

// TestAutoRemovalKeepsTheRecordWhenRemovalFails makes a transient database
// failure visible and retryable instead of silently forgetting the row.
func TestAutoRemovalKeepsTheRecordWhenRemovalFails(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	path := writeFakeVideo(t, dir, "gone.mp4", 10)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	svc.SetMissingRemover(func(int64) error { return errors.New("database busy") })

	svc.RefreshAllStatuses()

	got, err := svc.repo.Get(v.ID)
	if err != nil {
		t.Fatalf("record must stay for a retry: %v", err)
	}
	if got.Status != StatusMissing {
		t.Errorf("status = %q, want %q", got.Status, StatusMissing)
	}
}

// TestAutoRemovalAnnouncesSweepResults gives the desktop one summary event per
// volume, so the user is told that a cleanup happened (and that files were not
// deleted).
func TestAutoRemovalAnnouncesSweepResults(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	path := writeFakeVideo(t, dir, "gone.mp4", 10)
	if _, err := svc.AddVideo(path); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	var swept map[string]any
	svc.emit = func(name string, data ...any) {
		if name == "video:missing:swept" && len(data) == 1 {
			swept, _ = data[0].(map[string]any)
		}
	}
	svc.RefreshAllStatuses()

	if swept == nil {
		t.Fatal("no sweep event emitted")
	}
	if swept["root"] != VolumeRoot(path) || swept["removed"] != 1 || swept["skipped"] != 0 {
		t.Errorf("sweep = %+v, want root=%q removed=1 skipped=0", swept, VolumeRoot(path))
	}
}

// TestReconcileMissingRemovesGoneFilesAndKeepsTheRest is the read-path entry
// point the 番号 duplicate worklist uses: only the version confirmed gone is
// removed, and a healthy sibling on the same volume trusts the disk.
func TestReconcileMissingRemovesGoneFilesAndKeepsTheRest(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	keptPath := writeFakeVideo(t, dir, "kept.mp4", 10)
	gonePath := writeFakeVideo(t, dir, "gone.mp4", 10)
	kept, err := svc.AddVideo(keptPath)
	if err != nil {
		t.Fatalf("add kept: %v", err)
	}
	gone, err := svc.AddVideo(gonePath)
	if err != nil {
		t.Fatalf("add gone: %v", err)
	}
	if err := os.Remove(gonePath); err != nil {
		t.Fatalf("remove: %v", err)
	}

	var swept map[string]any
	svc.emit = func(name string, data ...any) {
		if name == "video:missing:swept" && len(data) == 1 {
			swept, _ = data[0].(map[string]any)
		}
	}
	removed, err := svc.ReconcileMissing([]int64{kept.ID, gone.ID})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(removed) != 1 || !removed[gone.ID] {
		t.Errorf("removed = %v, want only %d", removed, gone.ID)
	}
	if _, err := svc.repo.Get(gone.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("gone record = %v, want ErrNotFound", err)
	}
	if _, err := svc.repo.Get(kept.ID); err != nil {
		t.Errorf("kept record must stay: %v", err)
	}
	if swept == nil || swept["removed"] != 1 || swept["skipped"] != 0 {
		t.Errorf("sweep = %+v, want removed=1 skipped=0", swept)
	}
}

// TestReconcileMissingKeepsARecordOnADetachedVolume keeps the read path from
// deleting a favorite whose disk is simply unplugged; the panel shows
// "磁盘未接入" instead.
func TestReconcileMissingKeepsARecordOnADetachedVolume(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	path := writeFakeVideo(t, dir, "away.mp4", 10)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	svc.stat = detachedVolumeStub(path)

	removed, err := svc.ReconcileMissing([]int64{v.ID})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want none for an unplugged drive", removed)
	}
	got, err := svc.repo.Get(v.ID)
	if err != nil {
		t.Fatalf("record must stay: %v", err)
	}
	if got.Status != StatusAvailable {
		t.Errorf("status = %q, want %q: a detached drive is not a missing file", got.Status, StatusAvailable)
	}
}

// TestReconcileMissingKeepsAnUnreadableFile keeps the read path from treating
// a permission failure as a deletion.
func TestReconcileMissingKeepsAnUnreadableFile(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	path := writeFakeVideo(t, dir, "locked.mp4", 10)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	svc.stat = func(p string) (os.FileInfo, error) {
		if strings.EqualFold(p, path) {
			return nil, &os.PathError{Op: "stat", Path: p, Err: os.ErrPermission}
		}
		return os.Stat(p)
	}

	removed, err := svc.ReconcileMissing([]int64{v.ID})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want none for an unreadable file", removed)
	}
	got, err := svc.repo.Get(v.ID)
	if err != nil {
		t.Fatalf("record must stay: %v", err)
	}
	if got.Status != StatusMissing {
		t.Errorf("status = %q, want %q", got.Status, StatusMissing)
	}
}

// TestReconcileMissingRefusesWhenEveryFavoriteOnTheVolumeIsGone applies the
// same reused-drive-letter guard as startup to the read path.
func TestReconcileMissingRefusesWhenEveryFavoriteOnTheVolumeIsGone(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	firstPath := writeFakeVideo(t, dir, "a.mp4", 10)
	secondPath := writeFakeVideo(t, dir, "b.mp4", 10)
	first, err := svc.AddVideo(firstPath)
	if err != nil {
		t.Fatalf("add first: %v", err)
	}
	second, err := svc.AddVideo(secondPath)
	if err != nil {
		t.Fatalf("add second: %v", err)
	}
	if err := os.Remove(firstPath); err != nil {
		t.Fatalf("remove first: %v", err)
	}
	if err := os.Remove(secondPath); err != nil {
		t.Fatalf("remove second: %v", err)
	}

	var swept map[string]any
	svc.emit = func(name string, data ...any) {
		if name == "video:missing:swept" && len(data) == 1 {
			swept, _ = data[0].(map[string]any)
		}
	}
	removed, err := svc.ReconcileMissing([]int64{first.ID, second.ID})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want none when the whole volume is missing", removed)
	}
	if swept == nil || swept["removed"] != 0 || swept["skipped"] != 2 {
		t.Errorf("sweep = %+v, want removed=0 skipped=2", swept)
	}
}

// TestReconcileMissingLeavesRecordsWithoutAnIdentifiableVolume mirrors the
// startup guard on the read path.
func TestReconcileMissingLeavesRecordsWithoutAnIdentifiableVolume(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return false }
	path := writeFakeVideo(t, dir, "gone.mp4", 10)
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	removed, err := svc.ReconcileMissing([]int64{v.ID})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want none without an identifiable volume", removed)
	}
	got, err := svc.repo.Get(v.ID)
	if err != nil {
		t.Fatalf("record must stay: %v", err)
	}
	if got.Status != StatusMissing {
		t.Errorf("status = %q, want %q", got.Status, StatusMissing)
	}
}

// detachedVolumeStub makes one file and its volume root unreachable while every
// other path still answers, which is what an unplugged drive looks like.
func detachedVolumeStub(path string) func(string) (os.FileInfo, error) {
	return func(p string) (os.FileInfo, error) {
		if strings.EqualFold(p, path) || strings.EqualFold(p, VolumeRoot(path)) {
			return nil, os.ErrNotExist
		}
		return os.Stat(p)
	}
}
