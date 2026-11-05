package watch

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"muse-app/backend/video"
)

// LibrarySource is the slice of the favorites library the watcher needs:
// list existing paths (to diff each scan against) and add newly discovered
// files. AutoTagVideos applies the automatic group rules (file-name based plus
// the caller's resolution grouping) to the success results. api.Library
// satisfies it, so the façade stays the only writer and auto-imports behave
// exactly like manual adds (dedup, background thumbnail + metadata scrape) —
// including their group rules.
type LibrarySource interface {
	ListVideos() ([]video.Video, error)
	AddVideos(paths []string) []video.AddVideoResult
	AutoTagVideos(results []video.AddVideoResult)
}

// Service manages the watched-directory catalog and the auto-import loop.
// Create it with New, wire it into api.Library, then call Start once.
type Service struct {
	repo Repository
	lib  LibrarySource
	emit func(name string, data ...any)
	log  *slog.Logger

	// watcher is the fsnotify handle; nil when the platform backend is
	// unavailable, in which case only the periodic rescan keeps things fresh.
	watcher *fsnotifyWatcher
	// watched holds every directory currently registered with fsnotify, keyed
	// by cleaned lower-case path with the real-case path as value, so
	// re-registration never double-adds a handle and unregistration can name
	// the exact path that was registered.
	watchedMu sync.Mutex
	watched   map[string]string
	// roots maps root keys (cleaned lower-case path) to their rows so fsnotify
	// events can be routed to the root whose scan must be refreshed.
	rootsMu sync.Mutex
	roots   map[string]*WatchedDir
	// scans serializes scan passes: one worker consumes root rows, so two
	// passes never race the favorites dedup check with the same candidate.
	scans chan *WatchedDir
	// debounce holds one timer per root; bursts of fsnotify events collapse
	// into a single scan once the filesystem goes quiet.
	debounceMu sync.Mutex
	debounce   map[string]*time.Timer
	// canImport is the readability gate: a file another process still holds
	// exclusively is not importable yet (see importable). A seam so tests can
	// describe a locked file without one.
	canImport func(path string) bool
	// retrying remembers which roots already have a delayed rescan pending, so
	// a root with locked files is looked at once more and no more.
	retryMu  sync.Mutex
	retrying map[string]bool

	stopCh   chan struct{}
	stopOnce sync.Once
	loopWg   sync.WaitGroup
}

// New builds the folder-watch service over its persistence and the library.
func New(repo Repository, lib LibrarySource, emit func(name string, data ...any), log *slog.Logger) *Service {
	return &Service{
		repo:      repo,
		lib:       lib,
		emit:      emit,
		log:       log,
		watched:   map[string]string{},
		roots:     map[string]*WatchedDir{},
		scans:     make(chan *WatchedDir, 64),
		debounce:  map[string]*time.Timer{},
		canImport: importable,
		retrying:  map[string]bool{},
		stopCh:    make(chan struct{}),
	}
}

// List returns the watched-directory catalog in insertion order.
func (s *Service) List() ([]WatchedDir, error) {
	return s.repo.List()
}

// Add registers a directory for folder watching. The directory is scanned
// immediately (asynchronously) so its existing videos land in the favorites,
// and every video file that appears later is picked up by the watch loop.
func (s *Service) Add(path string) (*WatchedDir, error) {
	abs, err := normalizeDir(path)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.GetByPath(abs); err == nil {
		return nil, ErrAlreadyWatched
	} else if err != ErrNotFound {
		return nil, err
	}
	d := &WatchedDir{Path: abs, CreatedAt: time.Now().UnixMilli()}
	if err := s.repo.Create(d); err != nil {
		return nil, err
	}
	s.rememberRoot(d)
	s.watchTree(abs)
	s.enqueueScan(d)
	s.log.Info("watch dir added", "id", d.ID, "path", d.Path)
	s.emitChanged()
	return d, nil
}

// Remove unregisters a watched directory. Existing favorites are untouched —
// removing the watch only stops future auto-imports (收藏 ≠ 文件).
func (s *Service) Remove(id int64) error {
	d, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	s.forgetRoot(d.Path)
	s.unwatchTree(d.Path)
	s.clearDebounce(d.Path)
	s.log.Info("watch dir removed", "id", id, "path", d.Path)
	s.emitChanged()
	return nil
}

// ScanAll rescans every watched root synchronously and returns the summary.
// It is the test/diagnostic form; the UI uses ScanAllAsync, which routes
// through the serialized scan worker. Do not call it while Start's worker is
// consuming scans.
func (s *Service) ScanAll() ScanSummary {
	roots, err := s.repo.List()
	if err != nil {
		s.log.Error("scan all: list watched dirs failed", "err", err)
		return ScanSummary{}
	}
	summary := ScanSummary{Dirs: len(roots)}
	for i := range roots {
		r := s.scanRoot(&roots[i])
		summary.Added += r.Added
		summary.Relocated += r.Relocated
		summary.Failed += r.Failed
	}
	return summary
}

// ScanAllAsync schedules a rescan of every watched root on the serialized scan
// worker (used by the periodic safety net and the manual "立即扫描" button).
func (s *Service) ScanAllAsync() {
	roots, err := s.repo.List()
	if err != nil {
		s.log.Error("scan all: list watched dirs failed", "err", err)
		return
	}
	for i := range roots {
		s.enqueueScan(&roots[i])
	}
}

// collectCandidates walks root and returns the supported video files that are
// not already favorites. The second return value counts files that exist but
// are still locked by another process; the scan retries those on a later pass.
func (s *Service) collectCandidates(root string, existing map[string]struct{}) ([]string, int) {
	var candidates []string
	unreadable := 0
	walkErr := filepath.WalkDir(root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			s.log.Warn("scan: entry unavailable", "path", p, "err", err)
			return nil // one unreadable entry never aborts the walk
		}
		if entry.IsDir() {
			if p != root && skipDir(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !video.IsSupportedVideoFile(entry.Name()) {
			return nil
		}
		if _, seen := existing[strings.ToLower(p)]; seen {
			return nil
		}
		if !s.canImport(p) {
			// The file exists but another program still holds it exclusively (a
			// download client writing or verifying it). Importing it now would
			// record a reference the fingerprint pass cannot identify, and the
			// next rename of that file would become a second favorite for the
			// same bytes.
			unreadable++
			s.log.Debug("scan: file not readable yet", "path", p)
			return nil
		}
		candidates = append(candidates, p)
		return nil
	})
	if walkErr != nil {
		s.log.Warn("scan: walk failed", "root", root, "err", walkErr)
	}
	return candidates, unreadable
}

// scanRoot walks one watched root, diffs the supported video files against the
// favorites and imports the missing ones through the shared façade. It is
// idempotent: already-added paths produce no side effects.
func (s *Service) scanRoot(d *WatchedDir) ScanSummary {
	summary := ScanSummary{Dirs: 1}
	// The root may have been removed while its scan sat queued; skip silently.
	if _, err := s.repo.Get(d.ID); err != nil {
		return summary
	}
	existing, err := s.existingPaths()
	if err != nil {
		s.log.Error("watch scan: list videos failed", "root", d.Path, "err", err)
		return summary
	}

	candidates, unreadable := s.collectCandidates(d.Path, existing)
	if unreadable > 0 {
		s.scheduleImportRetry(d)
	}

	if len(candidates) > 0 {
		results := s.lib.AddVideos(candidates)
		// Auto-imported files get the same automatic group rules as manual adds
		// (e.g. "马赛克破解" for restored files); no dialog — the rules apply
		// silently, like the processor's resolution-based 4K grouping.
		s.lib.AutoTagVideos(results)
		for _, r := range results {
			switch {
			case !r.Success:
				summary.Failed++
				s.log.Warn("watch scan: add failed", "path", r.Path, "err", r.Error)
			case r.Relocated:
				// The file was already in the library under a path it has left;
				// the scan reconnected the favorite instead of duplicating it.
				summary.Relocated++
			default:
				summary.Added++
			}
		}
	}
	if err := s.repo.MarkScanned(d.ID, time.Now().UnixMilli()); err != nil {
		s.log.Warn("watch scan: mark scanned failed", "id", d.ID, "err", err)
	}
	if summary.Added > 0 || summary.Relocated > 0 || summary.Failed > 0 {
		s.log.Info("watch scan finished", "root", d.Path,
			"added", summary.Added, "relocated", summary.Relocated, "failed", summary.Failed)
	}
	s.emitScanned(d, summary)
	return summary
}

// existingPaths builds the case-insensitive set of every favorited path so a
// scan can diff the tree in memory instead of querying per file.
func (s *Service) existingPaths() (map[string]struct{}, error) {
	videos, err := s.lib.ListVideos()
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(videos))
	for _, v := range videos {
		set[strings.ToLower(v.Path)] = struct{}{}
	}
	return set, nil
}

// skipDir reports whether a directory should never be descended into: hidden
// folders and Windows system locations that cannot contain real media.
func skipDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "$recycle.bin", "system volume information":
		return true
	}
	return false
}

// normalizeDir validates a watched-directory candidate and returns its
// canonical form (absolute + cleaned, like the video domain's path rules).
func normalizeDir(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", ErrEmptyPath
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(filepath.Clean(abs))
	if err != nil {
		return "", ErrNotExist
	}
	if !info.IsDir() {
		return "", ErrNotDir
	}
	return filepath.Clean(abs), nil
}

// rootKey is the canonical map key for a directory path.
func rootKey(path string) string {
	return strings.ToLower(filepath.Clean(path))
}

func (s *Service) rememberRoot(d *WatchedDir) {
	s.rootsMu.Lock()
	s.roots[rootKey(d.Path)] = d
	s.rootsMu.Unlock()
}

func (s *Service) forgetRoot(path string) {
	s.rootsMu.Lock()
	delete(s.roots, rootKey(path))
	s.rootsMu.Unlock()
}

// rootFor finds the watched root that contains path (exact root match wins,
// then the deepest containing root so nested roots route to themselves).
func (s *Service) rootFor(path string) *WatchedDir {
	key := rootKey(path)
	sep := string(os.PathSeparator)
	s.rootsMu.Lock()
	defer s.rootsMu.Unlock()
	if d, ok := s.roots[key]; ok {
		return d
	}
	var best *WatchedDir
	bestLen := -1
	for k, d := range s.roots {
		if strings.HasPrefix(key, k+sep) && len(k) > bestLen {
			best, bestLen = d, len(k)
		}
	}
	return best
}

// enqueueScan schedules a root scan on the serialized worker; it never blocks
// the caller (a dropped scan is recovered by the periodic rescan).
func (s *Service) enqueueScan(d *WatchedDir) {
	select {
	case s.scans <- d:
	default:
		s.log.Warn("watch scan queue full, dropping scan", "root", d.Path)
	}
}

// scheduleScan debounces a root's scan: fsnotify delivers bursts of events for
// one user action (copy, move), and only the last one needs to trigger work.
func (s *Service) scheduleScan(d *WatchedDir) {
	key := rootKey(d.Path)
	s.debounceMu.Lock()
	defer s.debounceMu.Unlock()
	if t, ok := s.debounce[key]; ok {
		t.Reset(scanDebounce)
		return
	}
	s.debounce[key] = time.AfterFunc(scanDebounce, func() {
		s.debounceMu.Lock()
		delete(s.debounce, key)
		s.debounceMu.Unlock()
		s.enqueueScan(d)
	})
}

// importable reports whether a file can be opened for reading right now.
//
// The watcher sees a file the moment it appears, which is often while the
// program writing it still holds it exclusively (a download client). Importing
// such a file records a reference the fingerprint pass cannot identify, so when
// the file is later renamed the library cannot recognize it and ends up with two
// favorites for the same bytes. Waiting until the file can be read costs a
// retry, not the import.
func importable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	return f.Close() == nil
}

// scheduleImportRetry looks at a root once more after importRetry when its scan
// found files that were not readable yet. The delayed scan is not a rejection:
// it is the second look that imports a file whose writer has let go. A root
// already waiting for that second look does not schedule another one.
func (s *Service) scheduleImportRetry(d *WatchedDir) {
	key := rootKey(d.Path)
	s.retryMu.Lock()
	if s.retrying[key] {
		s.retryMu.Unlock()
		return
	}
	s.retrying[key] = true
	s.retryMu.Unlock()

	time.AfterFunc(importRetry, func() {
		s.retryMu.Lock()
		delete(s.retrying, key)
		s.retryMu.Unlock()
		select {
		case <-s.stopCh:
			return // the service is closing; no scan is wanted
		default:
		}
		s.enqueueScan(d)
	})
}

func (s *Service) clearDebounce(path string) {
	key := rootKey(path)
	s.debounceMu.Lock()
	if t, ok := s.debounce[key]; ok {
		t.Stop()
		delete(s.debounce, key)
	}
	s.debounceMu.Unlock()
}

// emitChanged notifies the frontend that the watched-dir catalog changed
// (added or removed), so the settings list can refresh.
func (s *Service) emitChanged() {
	if s.emit != nil {
		s.emit("watch:changed", map[string]any{})
	}
}

// emitScanned reports one finished scan pass. The frontend reloads the grid
// and toasts when the pass imported, reconnected or failed anything.
func (s *Service) emitScanned(d *WatchedDir, summary ScanSummary) {
	if s.emit != nil {
		s.emit("watch:scanned", map[string]any{
			"path":      d.Path,
			"added":     summary.Added,
			"relocated": summary.Relocated,
			"failed":    summary.Failed,
		})
	}
}
