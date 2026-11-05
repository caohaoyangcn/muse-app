package video

import (
	"os"
	"sync"
	"testing"
	"time"
)

// TestFileScanStatusDescribesTheStartupPass pins the counters the UI summarizes:
// one pass asks about every attached favorite once, and reports what it found
// and what it did about it. The startup check goes through the same status, so
// 文件检测 can say the library was already checked without running a second pass.
func TestFileScanStatusDescribesTheStartupPass(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	keptPath := writeFakeVideo(t, dir, "kept.mp4", 10)
	gonePath := writeFakeVideo(t, dir, "gone.mp4", 10)
	if _, err := svc.AddVideo(keptPath); err != nil {
		t.Fatalf("add kept: %v", err)
	}
	if _, err := svc.AddVideo(gonePath); err != nil {
		t.Fatalf("add gone: %v", err)
	}
	if err := os.Remove(gonePath); err != nil {
		t.Fatalf("remove: %v", err)
	}

	svc.RefreshAllStatuses()

	status := svc.FileScanStatus()
	if status.Running {
		t.Error("a finished pass must not report itself as running")
	}
	if status.Total != 2 || status.Checked != 2 {
		t.Errorf("pass checked %d/%d, want 2/2 files", status.Checked, status.Total)
	}
	if status.Removed != 1 {
		t.Errorf("removed = %d, want the one confirmed-gone favorite", status.Removed)
	}
	if status.Skipped != 0 || status.Offline != 0 || status.Revived != 0 {
		t.Errorf("unexpected counters: %+v", status)
	}
	if status.StartedAt == 0 || status.FinishedAt == 0 {
		t.Errorf("pass timestamps = %d..%d, want both recorded", status.StartedAt, status.FinishedAt)
	}
	if status.Error != "" {
		t.Errorf("error = %q, want none", status.Error)
	}
}

// TestFileScanStatusLeavesDetachedVolumesOut keeps the two questions apart: a
// file whose disk is unplugged is not "checked and missing", it is not checked
// at all, so it is counted apart from the work set.
func TestFileScanStatusLeavesDetachedVolumesOut(t *testing.T) {
	svc, dir := newTestService(t)
	path := writeFakeVideo(t, dir, "away.mp4", 10)
	if _, err := svc.AddVideo(path); err != nil {
		t.Fatalf("add: %v", err)
	}
	svc.stat = detachedVolumeStub(path)

	svc.RefreshAllStatuses()

	status := svc.FileScanStatus()
	if status.Total != 0 || status.Checked != 0 {
		t.Errorf("pass checked %d/%d files on a detached volume, want none", status.Checked, status.Total)
	}
	if status.Offline != 1 {
		t.Errorf("offline = %d, want the detached favorite counted", status.Offline)
	}
	if status.Removed != 0 {
		t.Errorf("removed = %d, want none: an unplugged drive is not a missing file", status.Removed)
	}
}

// TestFileScanStatusCountsProtectedFavoritesAsSkipped keeps the whole-volume
// guard visible in the result: when every favorite on a volume is gone, records
// are kept, and the pass has to say so instead of looking like a clean sweep.
func TestFileScanStatusCountsProtectedFavoritesAsSkipped(t *testing.T) {
	svc, dir := newTestService(t)
	svc.volumeIdentifiable = func(string) bool { return true }
	for _, name := range []string{"a.mp4", "b.mp4"} {
		path := writeFakeVideo(t, dir, name, 10)
		if _, err := svc.AddVideo(path); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove %s: %v", name, err)
		}
	}

	svc.RefreshAllStatuses()

	status := svc.FileScanStatus()
	if status.Removed != 0 || status.Skipped != 2 {
		t.Errorf("pass removed %d and skipped %d, want 0 removed / 2 protected",
			status.Removed, status.Skipped)
	}
	if status.Checked != 2 || status.Total != 2 {
		t.Errorf("pass checked %d/%d files, want 2/2", status.Checked, status.Total)
	}
}

// TestScanFileStatusesRunsInTheBackground covers the manual entry point: it
// answers immediately (the button cannot wait for thousands of stats) and the
// pass lands in the status that follows it.
func TestScanFileStatusesRunsInTheBackground(t *testing.T) {
	svc, dir := newTestService(t)
	for _, name := range []string{"a.mp4", "b.mp4", "c.mp4"} {
		if _, err := svc.AddVideo(writeFakeVideo(t, dir, name, 10)); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
	}

	started := svc.ScanFileStatuses()
	if !started.Running {
		t.Error("the manual pass must report that it is running")
	}

	status := waitForFileScan(t, svc)
	if status.Running {
		t.Error("the finished pass still reports itself as running")
	}
	if status.Total != 3 || status.Checked != 3 {
		t.Errorf("pass checked %d/%d files, want 3/3", status.Checked, status.Total)
	}
	if status.Removed != 0 || status.Skipped != 0 || status.Error != "" {
		t.Errorf("unexpected result: %+v", status)
	}
}

// TestScanFileStatusesJoinsTheRunningPass is the double-click rule: a second
// press while a pass is in flight joins it instead of asking every disk again
// and splitting the counters between two passes.
func TestScanFileStatusesJoinsTheRunningPass(t *testing.T) {
	svc, dir := newTestService(t)
	files := map[string]bool{}
	for _, name := range []string{"a.mp4", "b.mp4"} {
		path := writeFakeVideo(t, dir, name, 10)
		files[path] = true
		if _, err := svc.AddVideo(path); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
	}

	// Holding the first file's stat open puts the pass deterministically in the
	// middle, so the second call cannot race a pass that already finished.
	entered := make(chan struct{})
	release := make(chan struct{})
	var blockOnce, releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	svc.stat = func(p string) (os.FileInfo, error) {
		if files[p] {
			blockOnce.Do(func() {
				close(entered)
				<-release
			})
		}
		return os.Stat(p)
	}

	first := svc.ScanFileStatuses()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the pass never reached its first file")
	}

	second := svc.ScanFileStatuses()
	if !second.Running {
		t.Error("the joined status must report the pass as running")
	}
	if second.StartedAt != first.StartedAt {
		t.Errorf("second call started a new pass (%d), want the running one (%d)",
			second.StartedAt, first.StartedAt)
	}

	releaseOnce.Do(func() { close(release) })
	status := waitForFileScan(t, svc)
	if status.Total != 2 || status.Checked != 2 {
		t.Errorf("pass checked %d/%d files, want each file once", status.Checked, status.Total)
	}
}

// TestFileScanAnnouncesProgressAndDone is how the desktop follows a pass: the
// plan first (total known, nothing checked yet), then the outcome. The events
// carry the status itself, so the UI never has to poll the counters it draws.
func TestFileScanAnnouncesProgressAndDone(t *testing.T) {
	svc, dir := newTestService(t)
	if _, err := svc.AddVideo(writeFakeVideo(t, dir, "a.mp4", 10)); err != nil {
		t.Fatalf("add: %v", err)
	}

	var progress []FileScanStatus
	var done *FileScanStatus
	svc.emit = func(name string, data ...any) {
		if len(data) != 1 {
			return
		}
		status, ok := data[0].(FileScanStatus)
		if !ok {
			return
		}
		switch name {
		case "video:filescan:progress":
			progress = append(progress, status)
		case "video:filescan:done":
			final := status
			done = &final
		}
	}

	svc.RefreshAllStatuses()

	if len(progress) == 0 {
		t.Fatal("the pass emitted no progress")
	}
	// The first event is the plan: the denominator exists before any stat, which
	// is what lets the UI draw a bar instead of "0/0".
	if plan := progress[0]; plan.Total != 1 || plan.Checked != 0 {
		t.Errorf("plan = checked %d/%d, want 0/1", plan.Checked, plan.Total)
	}
	if done == nil {
		t.Fatal("the pass announced no done event")
	}
	if done.Checked != 1 || done.Total != 1 || done.Running {
		t.Errorf("done = %+v, want the finished 1/1 pass", *done)
	}
}

// TestFileScanWithoutTheLibraryExplainsItself keeps a failed pass from looking
// like a clean library: the reason is reported with the counters, and the pass
// still closes so the UI is not left with a spinner forever.
func TestFileScanWithoutTheLibraryExplainsItself(t *testing.T) {
	svc, _ := newTestService(t)
	svc.repo = &failingListRepo{VideoRepository: svc.repo}

	svc.RefreshAllStatuses()

	status := svc.FileScanStatus()
	if status.Running {
		t.Error("a failed pass must still close")
	}
	if status.Error == "" {
		t.Error("a pass that could not read the library must say so")
	}
	if status.FinishedAt == 0 {
		t.Error("a failed pass must record when it ended")
	}
}

// failingListRepo fails List only, which is how a broken database looks to the
// pass: it cannot even find out which files to ask about.
type failingListRepo struct {
	VideoRepository
}

func (r *failingListRepo) List() ([]Video, error) {
	return nil, os.ErrInvalid
}

// waitForFileScan waits for the background pass to finish and returns its last
// status.
func waitForFileScan(t *testing.T, svc *Service) FileScanStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		status := svc.FileScanStatus()
		if !status.Running {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatal("the file scan did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestFileScanSkipsRemoteFavorites(t *testing.T) {
	svc, _ := newTestService(t)
	v, _, err := svc.AddRemoteVideo(5, "/媒体/a.mp4", "a.mp4", 100)
	if err != nil {
		t.Fatal(err)
	}
	svc.RefreshAllStatuses()
	status := svc.FileScanStatus()
	if status.Remote != 1 || status.Total != 0 || status.Checked != 0 {
		t.Errorf("remote pass = %+v, want remote=1 total=0", status)
	}
	if status.Removed != 0 {
		t.Errorf("remote favorite was removed by the local pass: %+v", status)
	}
	got, err := svc.Get(v.ID)
	if err != nil || got.Status != StatusAvailable {
		t.Errorf("remote status changed: %+v err=%v", got, err)
	}
}
