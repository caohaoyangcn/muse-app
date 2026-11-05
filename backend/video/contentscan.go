package video

import (
	"sync"
)

// contentScanChunk is how many files a scan hands to the worker pool at a time.
// Small enough that a shutdown drains it in about a second, large enough to
// keep the workers fed.
const contentScanChunk = 16

// contentScanProgressEvery throttles the progress event during the fingerprint
// phase, where each file costs a couple of megabytes of reading.
const contentScanProgressEvery = 8

// contentExaminedProgressEvery throttles it during the size phase, which only
// stats each file: a smaller stride would emit hundreds of events about work
// that takes no measurable time.
const contentExaminedProgressEvery = 32

// Scan phases. A pass reads every favorite's size first and only then knows
// which files need their content compared, so the two phases keep their own
// counters and the second phase's total does not exist until the first ends.
const (
	scanPhaseExamining = "examining"
	scanPhaseComparing = "comparing"
)

// ContentScanStatus reports the library-wide fingerprint pass (内容重复扫描).
//
// It lives in memory and is not persisted: the pass is resumable from the
// database, because a favorite that still lacks a fingerprint is exactly a
// favorite the next pass picks up.
//
// The counters are per phase, because only the running phase's numbers mean
// anything: a single pair would either be unknown (the work set is not known
// until the size pass ends) or misleading.
type ContentScanStatus struct {
	// Running is true while a pass is in progress.
	Running bool `json:"running"`
	// Phase is "examining" while sizes are read from the disks, "comparing"
	// while fingerprints are computed, and empty when nothing is running.
	Phase string `json:"phase"`
	// Examined and ExamineTotal are the size phase's progress: how many
	// favorites' sizes have been read, out of all the available favorites.
	Examined     int `json:"examined"`
	ExamineTotal int `json:"examineTotal"`
	// Total is how many favorites the comparing phase has to fingerprint, known
	// once examining ends: every available favorite whose stored fingerprint no
	// longer describes its file (including one that never had one). Zero is a
	// complete answer — every fingerprint is current — not a missing one.
	Total int `json:"total"`
	// Done counts the files the comparing phase has finished with (hashed or
	// skipped).
	Done int `json:"done"`
	// Hashed counts freshly computed fingerprints.
	Hashed int `json:"hashed"`
	// Skipped counts files whose stored fingerprint still described them.
	Skipped int `json:"skipped"`
	// Failed counts files that could not be read at all: a missing file, or a
	// drive that is not attached. The panel reports these instead of presenting
	// an unreadable library as duplicate-free.
	Failed int `json:"failed"`
	// StartedAt and FinishedAt are unix milliseconds.
	StartedAt  int64 `json:"startedAt"`
	FinishedAt int64 `json:"finishedAt"`
}

// contentScanState guards the pass status. It is a value on Service so the
// service file needs no locking type of its own.
type contentScanState struct {
	mu     sync.Mutex
	status ContentScanStatus
}

// ScanContentDuplicates starts the library-wide fingerprint pass and returns
// immediately. Calling it while a pass is running returns that pass's status
// rather than starting a second one.
func (s *Service) ScanContentDuplicates() (ContentScanStatus, error) {
	if s.fingerprinter == nil {
		return ContentScanStatus{}, ErrFingerprinterUnavailable
	}
	s.scan.mu.Lock()
	defer s.scan.mu.Unlock()
	if s.scan.status.Running {
		return s.scan.status, nil
	}
	s.scan.status = ContentScanStatus{Running: true, StartedAt: nowMillis()}
	started := s.scan.status
	go s.runContentScan()
	return started, nil
}

// ContentScanStatus returns the current (or last) pass status.
func (s *Service) ContentScanStatus() ContentScanStatus {
	s.scan.mu.Lock()
	defer s.scan.mu.Unlock()
	return s.scan.status
}

// runContentScan is the two-tier pass.
//
// T0 (examining) reads every available favorite's current facts from the disk,
// because the size on a row is only written when the favorite is imported and so
// cannot answer whether the file still is the file it was.
// T1 (comparing) fingerprints every favorite whose stored fingerprint no longer
// describes its file — including one that never had a fingerprint.
//
// Identity is not only a dedup input: move detection recognizes a relocated file
// by its digest, so a favorite that never got one cannot be reconnected after a
// rename. That is why T1 covers the whole unidentified set instead of only the
// files whose size collides with another one: a scan is also how a library that
// was imported before fingerprints existed gains them.
//
// Each phase reports its own progress: the comparing phase's total only exists
// once every file has been examined.
func (s *Service) runContentScan() {
	videos, err := s.repo.List()
	if err != nil {
		s.log.Error("content scan could not list favorites", "err", err)
		s.finishContentScan()
		return
	}

	available := make([]*Video, 0, len(videos))
	for i := range videos {
		if videos[i].Status == StatusAvailable {
			available = append(available, &videos[i])
		}
	}
	// Which disks are attached decides whether an unreadable file is unreadable
	// or merely away, so observe the volumes before judging any of them.
	s.volumes.stamp(videos)
	s.volumes.probe()
	s.beginExamining(len(available))

	candidates := make([]int64, 0, len(available))
	unreadable := 0
	for _, v := range available {
		if info, statErr := s.stat(v.Path); statErr != nil {
			if s.volumes.offline(v.Path) {
				// The disk is not attached. The file is not unreadable, and
				// clearing its fingerprint would forget the bytes that identify
				// it when the drive comes back.
				s.countExamined()
				continue
			}
			// A file nobody can read must never look like a scanned one. The
			// failure is recorded, which also clears any fingerprint it had, so
			// it cannot be grouped as a duplicate on the strength of a verdict
			// about bytes that can no longer be checked.
			if uerr := s.repo.UpdateFingerprintFailure(v.ID, statErr.Error(), nowMillis()); uerr != nil {
				s.log.Warn("record unreadable favorite failed", "id", v.ID, "err", uerr)
			}
			unreadable++
		} else if !fingerprintDescribes(v, info.Size(), info.ModTime().UnixNano()) {
			candidates = append(candidates, v.ID)
		}
		s.countExamined()
	}

	s.beginComparing(len(candidates), unreadable)

	for start := 0; start < len(candidates); start += contentScanChunk {
		end := start + contentScanChunk
		if end > len(candidates) {
			end = len(candidates)
		}
		s.fingerprinter.ProcessBatch(candidates[start:end], s.recordScanResult)
	}
	s.finishContentScan()
}

// beginExamining announces the size phase and how many favorites it covers.
func (s *Service) beginExamining(total int) {
	s.scan.mu.Lock()
	s.scan.status.Phase = scanPhaseExamining
	s.scan.status.Examined = 0
	s.scan.status.ExamineTotal = total
	status := s.scan.status
	s.scan.mu.Unlock()

	s.emitScanProgress(status)
}

// countExamined records one examined favorite, announcing progress in strides
// (the phase stats hundreds of files and none of them takes measurable time).
func (s *Service) countExamined() {
	s.scan.mu.Lock()
	s.scan.status.Examined++
	examined, total := s.scan.status.Examined, s.scan.status.ExamineTotal
	status := s.scan.status
	s.scan.mu.Unlock()

	if examined%contentExaminedProgressEvery == 0 || examined == total {
		s.emitScanProgress(status)
	}
}

// beginComparing moves the pass to the fingerprint work set, which is only
// known now that every size has been read.
func (s *Service) beginComparing(total, unreadable int) {
	s.scan.mu.Lock()
	s.scan.status.Phase = scanPhaseComparing
	s.scan.status.Total = total
	s.scan.status.Done = 0
	s.scan.status.Failed = unreadable
	status := s.scan.status
	s.scan.mu.Unlock()

	s.log.Info("content scan examining finished",
		"examined", status.ExamineTotal,
		"unreadable", unreadable,
		"toCompare", total,
	)
	s.emitScanProgress(status)
}

// emitScanProgress forwards a status snapshot to the frontend.
func (s *Service) emitScanProgress(status ContentScanStatus) {
	if s.emit != nil {
		s.emit("fingerprint:scan:progress", status)
	}
}

// recordScanResult folds one file's outcome into the pass status.
func (s *Service) recordScanResult(out FingerprintOutcome) {
	s.scan.mu.Lock()
	s.scan.status.Done++
	switch out.Status {
	case FingerprintHashed:
		s.scan.status.Hashed++
	case FingerprintSkipped:
		s.scan.status.Skipped++
	case FingerprintFailed:
		s.scan.status.Failed++
	}
	done, total := s.scan.status.Done, s.scan.status.Total
	status := s.scan.status
	s.scan.mu.Unlock()

	if done%contentScanProgressEvery == 0 || done == total {
		s.emitScanProgress(status)
	}
}

// finishContentScan closes out the pass and announces the result. The phase is
// cleared and every counter is left in place, so the panel can summarize what
// the pass did after it stopped running.
func (s *Service) finishContentScan() {
	s.scan.mu.Lock()
	s.scan.status.Running = false
	s.scan.status.Phase = ""
	s.scan.status.FinishedAt = nowMillis()
	status := s.scan.status
	s.scan.mu.Unlock()

	s.log.Info("content scan finished",
		"examined", status.ExamineTotal,
		"unreadable", status.Failed,
		"total", status.Total,
		"hashed", status.Hashed,
		"skipped", status.Skipped,
	)
	s.emitScanProgress(status)
	if s.emit != nil {
		s.emit("fingerprint:scan:done", status)
	}
}
