package watch

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// fsnotifyWatcher aliases the library type so service.go can hold the handle
// while the fsnotify dependency stays local to this file.
type fsnotifyWatcher = fsnotify.Watcher

// Start loads the persisted watch list, registers recursive fsnotify watches,
// schedules the initial import scan and runs the event loop. Call once after
// wiring; safe to call before the UI is up (events emitted early are no-ops).
func (s *Service) Start() {
	roots, err := s.repo.List()
	if err != nil {
		s.log.Error("start: list watched dirs failed", "err", err)
		roots = nil
	}
	for i := range roots {
		s.rememberRoot(&roots[i])
	}
	if w, werr := fsnotify.NewWatcher(); werr == nil {
		s.watcher = w
	} else {
		s.log.Warn("fsnotify unavailable; folder watch falls back to periodic rescan", "err", werr)
	}
	for i := range roots {
		s.watchTree(roots[i].Path)
		s.enqueueScan(&roots[i])
	}
	s.loopWg.Add(1)
	go s.loop()
}

// Stop tears down the watcher and the event loop (used by tests).
func (s *Service) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
	if s.watcher != nil {
		_ = s.watcher.Close()
	}
	s.debounceMu.Lock()
	for key, timer := range s.debounce {
		timer.Stop()
		delete(s.debounce, key)
	}
	s.debounceMu.Unlock()
	s.loopWg.Wait()
}

// loop is the single event loop: it consumes queued scans, fsnotify events,
// watcher errors and the periodic safety-rescan tick. The event/error channels
// are nil when fsnotify is unavailable, and a nil channel never fires in a
// select, so the loop degrades to scans + ticker.
func (s *Service) loop() {
	defer s.loopWg.Done()
	var events chan fsnotify.Event
	var errs chan error
	if s.watcher != nil {
		events = s.watcher.Events
		errs = s.watcher.Errors
	}
	ticker := time.NewTicker(rescanEvery)
	defer ticker.Stop()
	for {
		select {
		case d := <-s.scans:
			s.scanRoot(d)
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			s.handleEvent(ev)
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			s.log.Warn("watcher error", "err", err)
		case <-ticker.C:
			s.ScanAllAsync()
		case <-s.stopCh:
			return
		}
	}
}

// handleEvent reacts to one fsnotify event: keep the recursive registration in
// sync with directory changes, then debounce a rescan of the owning root.
func (s *Service) handleEvent(ev fsnotify.Event) {
	if ev.Op == fsnotify.Chmod {
		return // attribute-only changes never alter the file set
	}
	if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
		// A directory appeared (created or moved in): watch its subtree too so
		// files that land there later are seen.
		s.watchTree(ev.Name)
	} else if ev.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		// Gone: drop the dead handle so a same-named re-create re-arms cleanly.
		s.unwatchPath(ev.Name)
	}
	if root := s.rootFor(ev.Name); root != nil {
		s.scheduleScan(root)
	}
}

// watchTree registers fsnotify handles for dir and every subdirectory.
// Registration is idempotent per path (guarded by the watched set).
func (s *Service) watchTree(dir string) {
	if s.watcher == nil {
		return
	}
	_ = filepath.WalkDir(dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable entry never aborts registration
		}
		if entry.IsDir() {
			if p != dir && skipDir(entry.Name()) {
				return fs.SkipDir
			}
			s.addWatch(p)
		}
		return nil
	})
}

// addWatch registers one directory, storing its real-case path so it can be
// unregistered later (fsnotify matches by the path string given to Add).
func (s *Service) addWatch(dir string) {
	key := rootKey(dir)
	s.watchedMu.Lock()
	if _, ok := s.watched[key]; ok {
		s.watchedMu.Unlock()
		return
	}
	s.watched[key] = dir
	s.watchedMu.Unlock()
	if err := s.watcher.Add(dir); err != nil {
		s.watchedMu.Lock()
		delete(s.watched, key)
		s.watchedMu.Unlock()
		s.log.Warn("watch registration failed", "dir", dir, "err", err)
	}
}

// unwatchPath drops one registration when its directory disappears.
func (s *Service) unwatchPath(path string) {
	key := rootKey(path)
	s.watchedMu.Lock()
	real, ok := s.watched[key]
	if !ok {
		s.watchedMu.Unlock()
		return
	}
	delete(s.watched, key)
	s.watchedMu.Unlock()
	if s.watcher != nil {
		_ = s.watcher.Remove(real)
	}
}

// unwatchTree drops every registration under root (called on Remove so a
// deleted watched folder leaves no orphan handles).
func (s *Service) unwatchTree(root string) {
	key := rootKey(root)
	prefix := key + string(os.PathSeparator)
	s.watchedMu.Lock()
	var reals []string
	for k, real := range s.watched {
		if k == key || strings.HasPrefix(k, prefix) {
			reals = append(reals, real)
			delete(s.watched, k)
		}
	}
	s.watchedMu.Unlock()
	if s.watcher == nil {
		return
	}
	for _, real := range reals {
		_ = s.watcher.Remove(real)
	}
}
