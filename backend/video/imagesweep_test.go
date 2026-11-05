package video

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"muse-app/backend/thumbnail"
)

// writeImage creates one generated image in a folder of the data dir.
func writeImage(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func imageName(id int64) string { return strconv.FormatInt(id, 10) + ".jpg" }

// mustRename renames a file, returning the new path.
func mustRename(t *testing.T, from, to string) string {
	t.Helper()
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	return to
}

// The sweep deletes an image whose record is gone and keeps the ones that have a
// record, across both folders the library owns.
func TestRemoveOrphanedImages(t *testing.T) {
	svc, dataDir := newTestService(t)
	keep := writeFakeVideo(t, dataDir, "keep.mp4", 10)
	v, err := svc.AddVideo(keep)
	if err != nil {
		t.Fatal(err)
	}

	thumbDir := filepath.Join(dataDir, thumbnail.RelativeDir)
	coverDir := filepath.Join(dataDir, "covers", "jav")
	liveThumb := writeImage(t, thumbDir, "live.jpg", "x")
	liveCover := writeImage(t, coverDir, "live.jpg", "x")
	// Give the live images the id the record actually got.
	liveThumb = mustRename(t, liveThumb, filepath.Join(thumbDir, imageName(v.ID)))
	liveCover = mustRename(t, liveCover, filepath.Join(coverDir, imageName(v.ID)))
	orphanThumb := writeImage(t, thumbDir, imageName(999), "orphan")
	orphanCover := writeImage(t, coverDir, imageName(998), "orphan")

	result, err := svc.RemoveOrphanedImages()
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if result.Removed != 2 {
		t.Errorf("removed = %d, want 2 (one per folder)", result.Removed)
	}
	if result.Scanned != 4 {
		t.Errorf("scanned = %d, want 4", result.Scanned)
	}
	for _, p := range []string{orphanThumb, orphanCover} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should be gone, stat err = %v", p, err)
		}
	}
	for _, p := range []string{liveThumb, liveCover} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s must survive the sweep: %v", p, err)
		}
	}
	// The favorite itself is untouched by a sweep.
	if _, err := svc.Get(v.ID); err != nil {
		t.Errorf("favorite should still exist: %v", err)
	}
}

// Only files the app names itself are candidates. A user's own image, a
// temporary cover download and a nested folder must all survive.
func TestRemoveOrphanedImagesLeavesForeignFiles(t *testing.T) {
	svc, dataDir := newTestService(t)
	thumbDir := filepath.Join(dataDir, thumbnail.RelativeDir)

	foreign := []string{
		"notes.txt",
		"999.png",
		"999.jpg.bak",
		"abc.jpg",
		"-5.jpg",
		"0.jpg",
		".cover-123456",      // a cover download still in flight
		".cover-jpeg-987654", // the re-encoded temporary
	}
	for _, name := range foreign {
		writeImage(t, thumbDir, name, "keep me")
	}
	// A folder that happens to be named like a generated image is not a file and
	// must never be removed.
	nested := writeImage(t, filepath.Join(thumbDir, "999.jpg"), "inner.jpg", "x")

	result, err := svc.RemoveOrphanedImages()
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if result.Removed != 0 {
		t.Errorf("removed = %d, want 0: no file matches a generated name", result.Removed)
	}
	for _, name := range foreign {
		if _, err := os.Stat(filepath.Join(thumbDir, name)); err != nil {
			t.Errorf("%q must survive the sweep: %v", name, err)
		}
	}
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("nested folder must survive the sweep: %v", err)
	}
}

// A missing folder is not an error: a fresh data dir has nothing to sweep.
func TestRemoveOrphanedImagesMissingFolders(t *testing.T) {
	svc, dataDir := newTestService(t)
	if err := os.RemoveAll(filepath.Join(dataDir, thumbnail.RelativeDir)); err != nil {
		t.Fatal(err)
	}
	result, err := svc.RemoveOrphanedImages()
	if err != nil {
		t.Fatalf("sweep on a fresh data dir: %v", err)
	}
	if result.Removed != 0 || result.Scanned != 0 {
		t.Errorf("result = %+v, want an empty sweep", result)
	}
}

// The prime case: a cover written after its record was deleted (a download that
// landed between the read and the delete) is cleared by the next sweep rather
// than by a periodic pass alone.
func TestRemoveOrphanedImagesClearsCoverWrittenAfterDeletion(t *testing.T) {
	svc, dataDir := newTestService(t)
	p := writeFakeVideo(t, dataDir, "race.mp4", 10)
	v, err := svc.AddVideo(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Remove(v.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	late := writeImage(t, filepath.Join(dataDir, "covers", "jav"), imageName(v.ID), "late cover")

	if _, err := svc.RemoveOrphanedImages(); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if _, err := os.Stat(late); !os.IsNotExist(err) {
		t.Errorf("late cover should be swept, stat err = %v", err)
	}
}

// A record removed by an older build left its image behind with no sweep to
// catch it; the first sweep of a new build clears it.
func TestRemoveOrphanedImagesClearsLegacyLeftovers(t *testing.T) {
	svc, dataDir := newTestService(t)
	leftover := writeImage(t, filepath.Join(dataDir, thumbnail.RelativeDir), "4242.jpg", "legacy")

	result, err := svc.RemoveOrphanedImages()
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if result.Removed != 1 {
		t.Errorf("removed = %d, want 1", result.Removed)
	}
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Errorf("legacy leftover should be gone, stat err = %v", err)
	}
}

// StartImageSweep sweeps immediately and can be stopped, twice, without hanging.
func TestStartImageSweepRunsAndStops(t *testing.T) {
	svc, dataDir := newTestService(t)
	orphan := writeImage(t, filepath.Join(dataDir, thumbnail.RelativeDir), "777.jpg", "orphan")

	stop := svc.StartImageSweep(0)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(orphan); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the first sweep did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	stop() // must be idempotent
}

// A batch removal must not start one directory walk per removal: the sweeps are
// coalesced, and every one of them still ends up seeing the deletions.
func TestRemovalsCoalesceTheirSweeps(t *testing.T) {
	svc, dataDir := newTestService(t)
	thumbDir := filepath.Join(dataDir, thumbnail.RelativeDir)

	var ids []int64
	for i := 0; i < 20; i++ {
		p := writeFakeVideo(t, dataDir, "batch"+strconv.Itoa(i)+".mp4", 10)
		v, err := svc.AddVideo(p)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, v.ID)
		writeImage(t, thumbDir, imageName(v.ID), "cover")
	}
	for _, id := range ids {
		if err := svc.Remove(id); err != nil {
			t.Fatalf("remove %d: %v", id, err)
		}
	}

	// The last scheduled sweep may still be running; poll until the folder is
	// empty of generated images.
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries, err := os.ReadDir(thumbDir)
		if err != nil {
			t.Fatal(err)
		}
		left := 0
		for _, e := range entries {
			if _, ok := generatedImageID(e.Name()); ok {
				left++
			}
		}
		if left == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d orphaned thumbnails left after the sweeps", left)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
