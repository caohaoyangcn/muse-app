package video

import (
	"sort"
	"sync"
	"time"
)

// 文件检测: re-answer "is this file still there?" for every favorite, on demand.
//
// The same pass runs at startup (RefreshAllStatuses). Until now that was the only
// trigger, so a file deleted while the app was running stayed a usable card until
// the next launch: the volume poll only reconciles a disk that came back, and
// Open only checks the one file it was asked about. "Restart the app and find
// out" is not a trigger, which is what the manual entry point removes.
//
// The manual pass is the startup pass, not a second implementation of it: it
// stamps and probes the volumes, then reconciles every attached volume as a whole
// (reconcileVolume), so the trusted-volume guard and the auto-removal rules
// cannot drift between the two triggers.
//
// The status lives in memory, like the content scan's. A pass is an observation
// of the current disk, not a checkpoint: repeating it costs one stat per file and
// it starts from the database every time, so there is nothing worth persisting.

// fileScanProgressEvery throttles the progress event. A pass only stats each
// file, which is cheap, so emitting per file would flood the frontend with
// hundreds of events about work it cannot see.
const fileScanProgressEvery = 16

// FileScanStatus reports the current or last 文件检测 pass.
type FileScanStatus struct {
	// Running is true while a pass is in progress.
	Running bool `json:"running"`
	// Total is how many favorites the pass will check: every favorite whose
	// volume is attached when the pass starts. Favorites on a detached volume
	// are counted by Offline instead.
	Total int `json:"total"`
	// Checked is how many files have been asked about so far.
	Checked int `json:"checked"`
	// Removed counts favorites whose file was confirmed gone and whose record
	// was moved to the recycle bin (never the source video). The name is kept
	// for API compatibility; the destination is the trash, so a restore is
	// possible until the retention expires.
	Removed int `json:"removed"`
	// Skipped counts favorites confirmed gone but deliberately kept: the whole
	// volume looked wrong (a reused drive letter, or a mount that is not ready
	// yet), so nothing on it is deleted.
	Skipped int `json:"skipped"`
	// Revived counts favorites whose file was found again and whose status went
	// back to available.
	Revived int `json:"revived"`
	// Offline counts favorites left alone because their volume is not attached.
	// A drive that is away is not a missing file, so those are not "checked".
	Offline int `json:"offline"`
	// Remote counts WebDAV favorites skipped by the local file-existence pass.
	// Their source has its own health check; stat-ing a cloud:// key would be a
	// guaranteed false missing.
	Remote int `json:"remote"`
	// Error is why a pass could not run at all (the library could not be read),
	// empty otherwise. A pass that ran leaves it empty even if it freed nothing.
	Error string `json:"error"`
	// StartedAt and FinishedAt are unix milliseconds.
	StartedAt  int64 `json:"startedAt"`
	FinishedAt int64 `json:"finishedAt"`
}

// fileScanState guards the pass status. Its Running flag is what serializes
// passes: reconcileMu already serializes the disk work per volume, but two full
// passes would still double the stats and split the counters between them.
type fileScanState struct {
	mu     sync.Mutex
	status FileScanStatus
}

// ScanFileStatuses starts a 文件检测 pass and returns immediately. Calling it
// while a pass is running returns that pass's status instead of starting a
// second one: a button pressed twice is still one pass.
func (s *Service) ScanFileStatuses() FileScanStatus {
	status, mine := s.beginFileScan()
	if mine {
		go s.executeFileScan()
	}
	return status
}

// FileScanStatus returns the current (or last) pass status.
func (s *Service) FileScanStatus() FileScanStatus {
	s.fileScan.mu.Lock()
	defer s.fileScan.mu.Unlock()
	return s.fileScan.status
}

// RefreshAllStatuses re-checks every favorite at startup: it observes the
// volumes first (a favorite whose drive is not attached is left untouched),
// then reconciles each attached volume. A file that is gone while its volume is
// present is moved to the recycle bin because the reference no longer describes
// anything on the disk. Files that are only unreadable stay visible as
// status = missing.
//
// It runs synchronously (app.startup already calls it from its own goroutine)
// and records its outcome in the same status ScanFileStatuses reports, so 文件检测
// can say the library was already checked at startup without a second pass.
func (s *Service) RefreshAllStatuses() {
	if _, mine := s.beginFileScan(); !mine {
		return
	}
	s.executeFileScan()
}

// beginFileScan claims the pass. It reports whether the caller owns the work;
// when it does not, a pass is already running and the returned status describes
// it (the caller joins it rather than starting another).
func (s *Service) beginFileScan() (FileScanStatus, bool) {
	s.fileScan.mu.Lock()
	defer s.fileScan.mu.Unlock()
	if s.fileScan.status.Running {
		return s.fileScan.status, false
	}
	s.fileScan.status = FileScanStatus{Running: true, StartedAt: nowMillis()}
	return s.fileScan.status, true
}

// executeFileScan is the pass itself. It owns the status until finishFileScan
// closes it, so every exit path must go through finishFileScan.
func (s *Service) executeFileScan() {
	videos, err := s.repo.List()
	if err != nil {
		s.log.Error("file scan could not list favorites", "err", err)
		s.finishFileScan(0, 0, 0, err)
		return
	}
	s.volumes.stamp(videos)
	s.volumes.probe()

	// A favorite on a detached volume is not checked: the drive being away is
	// not a missing file. Everything else is the work the progress bar counts.
	roots := make(map[string]string, len(videos))
	total, offline, remote := 0, 0, 0
	for i := range videos {
		if videos[i].IsRemote() {
			remote++
			continue
		}
		root := VolumeRoot(videos[i].Path)
		roots[volumeKey(root)] = root
		if s.volumes.offline(root) {
			offline++
			continue
		}
		total++
	}
	ordered := make([]string, 0, len(roots))
	for _, root := range roots {
		ordered = append(ordered, root)
	}
	// A stable order keeps the pass (and its progress) independent of map
	// iteration, exactly like the volume poll.
	sort.Strings(ordered)
	s.planFileScan(total, offline, remote)

	revived, removed, skipped := 0, 0, 0
	for _, root := range ordered {
		// The drive may have been unplugged between the probe and this pass.
		if s.volumes.offline(root) {
			continue
		}
		r, d, k := s.reconcileVolume(root, true, s.countChecked)
		revived += r
		removed += d
		skipped += k
	}
	s.finishFileScan(revived, removed, skipped, nil)
}

// planFileScan publishes the work set: how many files will be asked about, and
// how many favorites are left alone on detached volumes.
func (s *Service) planFileScan(total, offline, remote int) {
	s.fileScan.mu.Lock()
	s.fileScan.status.Total = total
	s.fileScan.status.Offline = offline
	s.fileScan.status.Remote = remote
	status := s.fileScan.status
	s.fileScan.mu.Unlock()

	s.emitFileScanProgress(status)
}

// countChecked records one file whose existence is now known, announcing
// progress in strides.
func (s *Service) countChecked() {
	s.fileScan.mu.Lock()
	s.fileScan.status.Checked++
	checked, total := s.fileScan.status.Checked, s.fileScan.status.Total
	status := s.fileScan.status
	s.fileScan.mu.Unlock()

	if checked%fileScanProgressEvery == 0 || checked == total {
		s.emitFileScanProgress(status)
	}
}

// finishFileScan closes the pass and announces the result. Every counter stays
// in place, so the UI can summarize what the last pass did long after it ended.
func (s *Service) finishFileScan(revived, removed, skipped int, runErr error) {
	s.fileScan.mu.Lock()
	s.fileScan.status.Running = false
	s.fileScan.status.FinishedAt = nowMillis()
	s.fileScan.status.Revived = revived
	s.fileScan.status.Removed = removed
	s.fileScan.status.Skipped = skipped
	if runErr != nil {
		s.fileScan.status.Error = runErr.Error()
	}
	status := s.fileScan.status
	s.fileScan.mu.Unlock()

	s.log.Info("file scan finished",
		"checked", status.Checked,
		"total", status.Total,
		"offline", status.Offline,
		"remote", status.Remote,
		"revived", status.Revived,
		"removed", status.Removed,
		"skipped", status.Skipped,
		"finishedAt", time.UnixMilli(status.FinishedAt).Format(time.RFC3339),
	)
	s.emitFileScanProgress(status)
	if s.emit != nil {
		s.emit("video:filescan:done", status)
	}
}

// emitFileScanProgress forwards a status snapshot to the frontend.
func (s *Service) emitFileScanProgress(status FileScanStatus) {
	if s.emit != nil {
		s.emit("video:filescan:progress", status)
	}
}
