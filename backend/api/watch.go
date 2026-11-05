package api

import (
	"fmt"

	"muse-app/backend/watch"
)

// ---- Folder-watch domain (目录监控) ----
//
// Desktop-only settings surface: the persisted list of auto-monitored
// directories whose video files are imported into the favorites, recursively
// and including files that appear later. The methods live on the shared façade
// so the wiring (and any future HTTP route) reuses one implementation.

// SetWatchService wires the folder-watch domain into the façade. It is optional:
// without it every watch call reports "not configured".
func (l *Library) SetWatchService(svc *watch.Service) { l.watch = svc }

// watchService returns the wired service or a not-configured error.
func (l *Library) watchService() (*watch.Service, error) {
	if l.watch == nil {
		return nil, fmt.Errorf("目录监控未启用")
	}
	return l.watch, nil
}

// ListWatchDirs returns the watched-directory catalog.
func (l *Library) ListWatchDirs() ([]watch.WatchedDir, error) {
	svc, err := l.watchService()
	if err != nil {
		return nil, err
	}
	return svc.List()
}

// AddWatchDir registers a directory for watching and kicks off its initial
// recursive import scan (asynchronous; results arrive via watch:scanned).
func (l *Library) AddWatchDir(path string) (*watch.WatchedDir, error) {
	svc, err := l.watchService()
	if err != nil {
		return nil, err
	}
	return svc.Add(path)
}

// RemoveWatchDir unregisters a watched directory; existing favorites are kept.
func (l *Library) RemoveWatchDir(id int64) error {
	svc, err := l.watchService()
	if err != nil {
		return err
	}
	return svc.Remove(id)
}

// RescanWatchDirs schedules an immediate rescan of every watched root on the
// serialized scan worker. It returns immediately; outcomes arrive via the
// watch:scanned event.
func (l *Library) RescanWatchDirs() {
	if l.watch != nil {
		l.watch.ScanAllAsync()
	}
}

// ScanDirectoryOnce imports the video files under path once, without adding the
// directory to the folder-watch list. It is the 扫描文件夹 entry in the
// add-source dialog; the caller gets the per-file results for the same summary
// and group suggestions as a manual batch add.
func (l *Library) ScanDirectoryOnce(path string) (watch.ScanOnceResult, error) {
	svc, err := l.watchService()
	if err != nil {
		return watch.ScanOnceResult{}, err
	}
	return svc.ScanOnce(path)
}
