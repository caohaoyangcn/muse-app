package video

import (
	"log/slog"
	"os"
	"sync"
)

// fingerprintQueueSize bounds the ad-hoc queue. A full queue drops the job
// instead of blocking the caller: imports and scans re-derive what still needs
// hashing from the database, so a dropped id is picked up by the next scan.
const fingerprintQueueSize = 512

// FingerprintStatus is what processing one favorite did.
type FingerprintStatus int

const (
	// FingerprintSkipped means the stored fingerprint still described the file,
	// so nothing was read.
	FingerprintSkipped FingerprintStatus = iota
	// FingerprintHashed means a fingerprint was computed and stored.
	FingerprintHashed
	// FingerprintFailed means the file could not be read. The failure is
	// recorded on the row, which is how the panel reports unreadable files
	// instead of quietly treating them as duplicates-free.
	FingerprintFailed
)

// FingerprintOutcome is the result of processing one favorite.
type FingerprintOutcome struct {
	ID     int64
	Status FingerprintStatus
	Digest string
	// Message is the failure reason, empty unless Status is FingerprintFailed.
	Message string
}

// FullHashOutcome is the result of the full-file confirmation of one favorite.
type FullHashOutcome struct {
	ID     int64
	Digest string
	Size   int64
	Mtime  int64
	// Message is the failure reason; empty on success.
	Message string
}

// Succeeded reports whether a digest was produced.
func (o FullHashOutcome) Succeeded() bool { return o.Message == "" && o.Digest != "" }

// Fingerprinter computes and refreshes content fingerprints in the background.
//
// It is a bounded worker pool with the same shape as the thumbnail Processor,
// because the two jobs differ in kind but not in structure: a queue fed by
// imports, drained by workers, closed by draining. The pool is deliberately
// small — a pass reads two megabytes per file, so the disks are the limit, not
// the CPU.
//
// The same pool owns the full-file confirmations, on a queue of its own with a
// single worker: a confirmation reads a whole multi-gigabyte file, so the point
// is to keep it off the fingerprint queue and to keep several of them from
// competing for the same disks at once.
type Fingerprinter struct {
	repo VideoRepository
	emit func(name string, data ...any)
	log  *slog.Logger
	// compute, fullHash and stat are seams: tests replace them to count reads
	// and to drive sizes without touching a disk.
	compute  func(path string) (FingerprintResult, error)
	fullHash func(path string) (FingerprintResult, error)
	stat     func(path string) (os.FileInfo, error)
	now      func() int64
	// volumeOffline is optional: it tells a file whose disk is not attached
	// apart from a file that is unreadable. Only the former must keep its
	// digest, so the identity survives a drive being unplugged. Wired by
	// Service.SetFingerprinter.
	volumeOffline func(path string) bool

	jobs      chan fingerprintJob
	confirms  chan confirmJob
	wg        sync.WaitGroup
	closeOnce sync.Once
	jobsMu    sync.RWMutex
	closed    bool
}

// fingerprintJob is one queued id plus, for a scan, the callback that reports
// its outcome back to the scan that submitted it.
type fingerprintJob struct {
	id   int64
	done func(FingerprintOutcome)
}

// confirmJob is one queued full-file read.
type confirmJob struct {
	id   int64
	done func(FullHashOutcome)
}

// NewFingerprinter starts a fingerprint worker pool plus the single confirmation
// worker.
func NewFingerprinter(repo VideoRepository, emit func(name string, data ...any), log *slog.Logger, workers int) *Fingerprinter {
	if workers < 1 {
		workers = 1
	}
	f := &Fingerprinter{
		repo:     repo,
		emit:     emit,
		log:      log,
		compute:  Fingerprint,
		fullHash: FullHash,
		stat:     os.Stat,
		now:      nowMillis,
		jobs:     make(chan fingerprintJob, fingerprintQueueSize),
		confirms: make(chan confirmJob, fingerprintQueueSize),
	}
	for i := 0; i < workers; i++ {
		f.wg.Add(1)
		go f.worker()
	}
	f.wg.Add(1)
	go f.confirmWorker()
	return f
}

// Enqueue schedules one favorite for fingerprinting. It never blocks the
// caller.
func (f *Fingerprinter) Enqueue(id int64) {
	f.jobsMu.RLock()
	defer f.jobsMu.RUnlock()
	if f.closed {
		return
	}
	select {
	case f.jobs <- fingerprintJob{id: id}:
	default:
		f.log.Warn("fingerprint queue full, dropping job", "id", id)
	}
}

// Close stops accepting jobs and drains the ones already queued. Safe to call
// more than once and safe to call while another goroutine is enqueuing.
func (f *Fingerprinter) Close() {
	f.closeOnce.Do(func() {
		f.jobsMu.Lock()
		f.closed = true
		close(f.jobs)
		close(f.confirms)
		f.jobsMu.Unlock()
		f.wg.Wait()
	})
}

// ProcessBatch fingerprints every id through the pool and blocks until the
// batch is done, reporting each outcome as it lands. It stops early when the
// pool is closing, so an interrupted scan does not hold shutdown open.
//
// The library scan uses this rather than its own goroutines: one concurrency
// mechanism, one place where a file is read.
func (f *Fingerprinter) ProcessBatch(ids []int64, onResult func(FingerprintOutcome)) {
	if len(ids) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		job := fingerprintJob{id: id, done: func(out FingerprintOutcome) {
			if onResult != nil {
				onResult(out)
			}
			wg.Done()
		}}
		if !f.submit(job) {
			// Never queued: release the count we just took and stop feeding.
			wg.Done()
			break
		}
	}
	wg.Wait()
}

// submit hands one job to the fingerprint workers, blocking while the queue is
// full. It reports false when the pool is closing, in which case the job was not
// queued and its callback must not be expected.
func (f *Fingerprinter) submit(job fingerprintJob) bool {
	f.jobsMu.RLock()
	defer f.jobsMu.RUnlock()
	if f.closed {
		return false
	}
	f.jobs <- job
	return true
}

// ConfirmAll reads every id in full through the confirmation worker and blocks
// until the batch is done, reporting each outcome as it lands. It stops early
// when the pool is closing, so a shutdown does not wait for a whole file.
func (f *Fingerprinter) ConfirmAll(ids []int64, onResult func(FullHashOutcome)) {
	if len(ids) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		job := confirmJob{id: id, done: func(out FullHashOutcome) {
			if onResult != nil {
				onResult(out)
			}
			wg.Done()
		}}
		if !f.submitConfirm(job) {
			wg.Done()
			break
		}
	}
	wg.Wait()
}

// submitConfirm hands one job to the confirmation worker, blocking while it is
// busy. It reports false when the pool is closing.
func (f *Fingerprinter) submitConfirm(job confirmJob) bool {
	f.jobsMu.RLock()
	defer f.jobsMu.RUnlock()
	if f.closed {
		return false
	}
	f.confirms <- job
	return true
}

func (f *Fingerprinter) worker() {
	defer f.wg.Done()
	for job := range f.jobs {
		out := f.Process(job.id)
		if job.done != nil {
			job.done(out)
		}
		switch out.Status {
		case FingerprintHashed:
			f.log.Debug("content fingerprint stored", "id", out.ID)
			if f.emit != nil {
				f.emit("fingerprint:updated", map[string]any{"id": out.ID})
			}
		case FingerprintFailed:
			f.log.Warn("content fingerprint failed", "id", out.ID, "err", out.Message)
			if f.emit != nil {
				f.emit("fingerprint:failed", map[string]any{"id": out.ID, "error": out.Message})
			}
		}
	}
}

// Process revalidates or computes the fingerprint of one favorite and stores
// the result. It is the single place where a file is fingerprinted, shared by
// the queue and the library scan, and it is safe to call concurrently.
func (f *Fingerprinter) Process(id int64) FingerprintOutcome {
	v, err := f.repo.Get(id)
	if err != nil {
		// Removed while queued. Nothing to hash, and nothing to report: this is
		// not a file the user can act on.
		f.log.Debug("fingerprint skipped, video unavailable", "id", id, "err", err)
		return FingerprintOutcome{ID: id, Status: FingerprintFailed, Message: err.Error()}
	}

	// The cheap check first: a fingerprint is only worth anything while the file
	// it describes is still the same file, so compare the on-disk facts before
	// reading anything. This is what makes re-enqueuing a file free.
	info, err := f.stat(v.Path)
	if err != nil {
		if f.volumeOffline != nil && f.volumeOffline(v.Path) {
			// The disk is not attached. The file is not unreadable, and clearing
			// its digest would forget the bytes that identify it when the drive
			// comes back — which is exactly what the move detection relies on.
			return FingerprintOutcome{ID: v.ID, Status: FingerprintFailed,
				Message: "volume not attached"}
		}
		f.recordFailure(v.ID, err)
		return FingerprintOutcome{ID: v.ID, Status: FingerprintFailed, Message: err.Error()}
	}
	if fingerprintDescribes(v, info.Size(), info.ModTime().UnixNano()) {
		return FingerprintOutcome{ID: v.ID, Status: FingerprintSkipped, Digest: v.ContentHash}
	}

	res, err := f.compute(v.Path)
	if err != nil {
		f.recordFailure(v.ID, err)
		return FingerprintOutcome{ID: v.ID, Status: FingerprintFailed, Message: err.Error()}
	}
	if err := f.repo.UpdateFingerprint(v.ID, res.Digest, FingerprintVersion, res.Size, res.Mtime, f.now()); err != nil {
		f.log.Warn("store content fingerprint failed", "id", v.ID, "err", err)
		return FingerprintOutcome{ID: v.ID, Status: FingerprintFailed, Message: err.Error()}
	}
	return FingerprintOutcome{ID: v.ID, Status: FingerprintHashed, Digest: res.Digest}
}

// confirmWorker drains the confirmation queue one file at a time.
func (f *Fingerprinter) confirmWorker() {
	defer f.wg.Done()
	for job := range f.confirms {
		out := f.ProcessFullHash(job.id)
		if job.done != nil {
			job.done(out)
		}
		if !out.Succeeded() {
			f.log.Warn("content confirmation failed", "id", out.ID, "err", out.Message)
		}
	}
}

// ProcessFullHash reads one favorite's file in full and stores its digest as the
// confirmation of the group it belongs to.
//
// A failed read stores nothing: the group then keeps its partial verdict and
// stays unconfirmed, which is the honest outcome — the caller must not be able
// to present a confirmation that was never computed.
func (f *Fingerprinter) ProcessFullHash(id int64) FullHashOutcome {
	v, err := f.repo.Get(id)
	if err != nil {
		f.log.Debug("confirmation skipped, video unavailable", "id", id, "err", err)
		return FullHashOutcome{ID: id, Message: err.Error()}
	}
	res, err := f.fullHash(v.Path)
	if err != nil {
		f.log.Warn("full hash failed", "id", v.ID, "path", v.Path, "err", err)
		return FullHashOutcome{ID: v.ID, Message: err.Error()}
	}
	if err := f.repo.UpdateFullHash(v.ID, res.Digest, f.now()); err != nil {
		f.log.Warn("store full hash failed", "id", v.ID, "err", err)
		return FullHashOutcome{ID: v.ID, Message: err.Error()}
	}
	return FullHashOutcome{ID: v.ID, Digest: res.Digest, Size: res.Size, Mtime: res.Mtime}
}

// recordFailure notes that the file could not be read.
func (f *Fingerprinter) recordFailure(id int64, cause error) {
	if err := f.repo.UpdateFingerprintFailure(id, cause.Error(), f.now()); err != nil {
		f.log.Warn("record content fingerprint failure failed", "id", id, "err", err)
	}
}

// fingerprintDescribes reports whether the stored fingerprint was taken from a
// file with these facts. Size and mtime are the whole test: the app cannot know
// what happened to a file it did not write, and a mismatch means the stored
// verdict may be about different bytes.
func fingerprintDescribes(v *Video, size, mtime int64) bool {
	return v.ContentHash != "" &&
		v.HashVersion == FingerprintVersion &&
		v.HashState.Size == size &&
		v.HashState.Mtime == mtime
}
