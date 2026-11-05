package video

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"muse-app/backend/thumbnail"
)

// This file keeps the generated images from outliving the favorites they belong
// to. Removal deletes the thumbnail of the record it removes, but that is not a
// guarantee: a scraped cover that was written between the row being read and the
// row being deleted, a favourite dropped by an older build, a data dir restored
// without its database — all of them leave a file no record points at any more.
//
// The sweep closes that gap the only way it can be closed reliably: by asking
// the database what still exists, on the read path where the whole set is
// already in memory, rather than trying to enumerate every interleaving on the
// write path.
//
// Only files shaped exactly like a generated image are candidates, and the shape
// is what makes this safe: <id>.jpg is the name generator the thumbnail
// extractor and the scraped-cover downloader both use, so anything the user put
// in those folders under another name is never touched.

// ImageSweepResult reports what one sweep did.
type ImageSweepResult struct {
	// Scanned is how many files were looked at across every swept folder.
	Scanned int
	// Removed is how many orphaned images were deleted.
	Removed int
	// Bytes is how much disk space they occupied.
	Bytes int64
}

// sweepImageDirs are the folders whose contents the library owns, relative to
// the data dir.
var sweepImageDirs = []string{thumbnail.RelativeDir, "covers/jav"}

// RemoveOrphanedImages deletes the generated thumbnails and scraped covers that
// no favorite refers to any more, and reports what it freed.
//
// It is safe to call at any time: the sweep only ever deletes a file whose name
// is the id of a record that does not exist, which is a fact about the database
// and not about the disk. A file written a moment ago by a worker whose record
// has since been deleted is exactly what the next sweep is for.
//
// Errors are per-image and non-fatal: one locked file must not stop the sweep or
// fail the removal that triggered it.
func (s *Service) RemoveOrphanedImages() (ImageSweepResult, error) {
	ids, err := s.repo.ListVideoIDs()
	if err != nil {
		return ImageSweepResult{}, err
	}
	live := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		live[id] = struct{}{}
	}
	return s.sweepImages(live), nil
}

// sweepImages walks the folders the library owns and deletes every generated
// image whose id is not in live.
func (s *Service) sweepImages(live map[int64]struct{}) ImageSweepResult {
	var result ImageSweepResult
	for _, rel := range sweepImageDirs {
		dir := filepath.Join(s.dataDir, filepath.FromSlash(rel))
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				s.log.Warn("sweep images: read dir failed", "dir", dir, "err", err)
			}
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			result.Scanned++
			id, ok := generatedImageID(entry.Name())
			if !ok {
				// Not a file this app generated: leave it alone. The folders are
				// on the user's disk, whatever else is in them is not ours.
				continue
			}
			if _, exists := live[id]; exists {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			info, statErr := entry.Info()
			if err := os.Remove(path); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue // removed since the directory was read
				}
				s.log.Warn("sweep images: remove failed", "path", path, "err", err)
				continue
			}
			result.Removed++
			if statErr == nil {
				result.Bytes += info.Size()
			}
			s.log.Debug("swept orphaned image", "path", path)
		}
	}
	if result.Removed > 0 {
		s.log.Info("orphaned images removed", "files", result.Removed, "bytes", result.Bytes)
	}
	return result
}

// generatedImageID reports whether name is one of the app's generated images
// (<id>.jpg) and returns the id it belongs to.
//
// The match is deliberately strict — digits plus ".jpg", nothing else — because
// a false positive deletes a file the app did not create. It also skips the
// downloader's temporary files (".cover-*"), which exist for milliseconds and
// belong to a write still in flight.
func generatedImageID(name string) (int64, bool) {
	base, ok := strings.CutSuffix(name, ".jpg")
	if !ok || base == "" {
		return 0, false
	}
	for _, r := range base {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	id, err := strconv.ParseInt(base, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// sweepImagesAfterRemove runs the sweep off the write path: removal answers the
// user immediately and the scheduled cleanup completes in the background.
//
// Requests coalesce. Removing a batch (查重 keeps one version and drops twenty)
// would otherwise start twenty directory walks, each deleting what the last one
// already did. One sweep at a time, and a request that arrives while one runs
// schedules one more pass so it still sees the newest deletions.
func (s *Service) sweepImagesAfterRemove(id int64) {
	s.sweepReq.mu.Lock()
	if s.sweepReq.running {
		s.sweepReq.again = true
		s.sweepReq.mu.Unlock()
		return
	}
	s.sweepReq.running = true
	s.sweepReq.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("image sweep panicked", "id", id, "panic", r)
			}
		}()
		for {
			result, err := s.RemoveOrphanedImages()
			if err != nil {
				s.log.Warn("sweep images after removal failed", "id", id, "err", err)
			} else if result.Removed > 0 {
				s.log.Info("removal swept orphaned images", "id", id, "files", result.Removed, "bytes", result.Bytes)
			}

			s.sweepReq.mu.Lock()
			if !s.sweepReq.again {
				s.sweepReq.running = false
				s.sweepReq.mu.Unlock()
				return
			}
			s.sweepReq.again = false
			s.sweepReq.mu.Unlock()
		}
	}()
}

// imageSweepRequests coalesces the sweeps scheduled by removals.
type imageSweepRequests struct {
	mu      sync.Mutex
	running bool
	again   bool
}

// sweepIntervalDefault is how often a running app reconciles the image folders.
const sweepIntervalDefault = 24 * time.Hour

// StartImageSweep reconciles the image folders against the database now and then
// every interval, so orphaned images cannot accumulate across restarts. A
// non-positive interval uses sweepIntervalDefault.
//
// The returned func stops the loop. The first sweep runs on its own goroutine:
// startup must not wait on a directory walk, and the answer is never needed for
// the first screen.
func (s *Service) StartImageSweep(interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = sweepIntervalDefault
	}
	done := make(chan struct{})
	var once sync.Once
	sweep := func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("image sweep panicked", "panic", r)
			}
		}()
		result, err := s.RemoveOrphanedImages()
		if err != nil {
			s.log.Warn("sweep images failed", "err", err)
			return
		}
		if result.Scanned > 0 {
			s.log.Info("image folders swept", "scanned", result.Scanned, "removed", result.Removed, "bytes", result.Bytes)
		}
	}
	go func() {
		sweep()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				sweep()
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}
