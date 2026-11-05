// Package watch contains the folder-watch domain (目录监控): a persisted list
// of directories whose video files are auto-imported into the favorites —
// recursively at add time and again whenever new files appear later. Like every
// domain it only extends the favorite catalog; it never moves or deletes media.
package watch

import (
	"time"

	"muse-app/backend/video"
)

// WatchedDir is one auto-monitored directory.
type WatchedDir struct {
	ID         int64  `json:"id"`
	Path       string `json:"path"`
	CreatedAt  int64  `json:"createdAt"`
	LastScanAt int64  `json:"lastScanAt"` // 0 until the first recursive scan completes
}

// ScanSummary reports the outcome of one scan pass over every watched root.
type ScanSummary struct {
	// Added is the number of favorites created by this pass.
	Added int `json:"added"`
	// Relocated is the number of candidates that did not become new favorites
	// but re-pointed an existing one whose file had moved: the bytes were already
	// in the library under a path that no longer exists. Kept apart from Added so
	// the UI can say "reconnected" instead of "added".
	Relocated int `json:"relocated"`
	// Failed is the number of candidate files that could not be imported.
	Failed int `json:"failed"`
	// Dirs is the number of watched roots included in the pass.
	Dirs int `json:"dirs"`
}

// ScanOnceResult reports one user-picked directory scan (扫描文件夹). It is
// deliberately separate from ScanSummary: this pass is not a watched root, so
// Dirs and last_scan_at have no meaning here. Results is the per-file outcome,
// which lets the UI reuse the same add summary and group suggestions as a
// manual batch add.
type ScanOnceResult struct {
	// Path is the absolute directory the user picked.
	Path string `json:"path"`
	// Added is the number of new favorites created by this pass.
	Added int `json:"added"`
	// Relocated is the number of candidates that re-pointed an existing
	// favorite whose file had moved, instead of creating a duplicate.
	Relocated int `json:"relocated"`
	// Failed is the number of candidates AddVideos rejected.
	Failed int `json:"failed"`
	// Skipped is the number of video files the pass could not read yet (a
	// download client still holds them). They are not favorites and are not
	// retried by this one-off pass.
	Skipped int `json:"skipped"`
	// Results is the per-file outcome, one entry per candidate handed to the
	// library. It is always non-nil so the UI never has to special-case null.
	Results []video.AddVideoResult `json:"results"`
}

// scanDebounce waits for the filesystem to go quiet after a burst of fsnotify
// events (e.g. an in-progress copy) before rescanning the affected root.
const scanDebounce = 1500 * time.Millisecond

// rescanEvery is the periodic safety-net pass. fsnotify covers local disks
// well, but network shares and watcher-buffer overflows can drop events; a
// slow full rescan guarantees new files are picked up eventually.
const rescanEvery = 5 * time.Minute

// importRetry is how long to wait before re-scanning a root that had files the
// app could not read yet (see importable). One retry per scan: a file a download
// client keeps locked for hours is picked up by the periodic scan, not by a
// tight loop.
const importRetry = time.Minute
