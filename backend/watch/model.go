// Package watch contains the folder-watch domain (目录监控): a persisted list
// of directories whose video files are auto-imported into the favorites —
// recursively at add time and again whenever new files appear later. Like every
// domain it only extends the favorite catalog; it never moves or deletes media.
package watch

import "time"

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
