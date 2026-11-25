package video

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// 精彩时刻 generation (docs/spec/highlight-moments.md §5).
//
// Division of labour, the same one the rest of the video domain uses:
//
//   - the database is the durable truth (video_highlights rows),
//   - the queue is an in-memory list of ids that can be lost at any moment,
//   - this service is the only writer, so there is no second scheduler.
//
// Deleting a row is how the user cancels; there is no separate cancel state. The
// worker therefore never trusts what it read a moment ago: every transition is a
// conditional UPDATE, and a false answer means "the user took this away".

const (
	// highlightMaxInterruptions is how often a crash or a quit may interrupt one
	// row before it is marked failed instead of being requeued. A file that
	// takes the whole app down every time must stop being retried automatically.
	highlightMaxInterruptions = 3

	// highlightGIFMaxBytes is the 8 MB soft ceiling on a range GIF. It is soft:
	// the fallback is one regeneration at a lower frame rate, and a file that is
	// still too big is kept rather than degraded further.
	highlightGIFMaxBytes = 8 * 1024 * 1024

	// Frame rates for a range GIF: the default, and the one fallback used when
	// the default file is over the ceiling. Dropping frames keeps every frame
	// at full resolution and colour; lowering either would not.
	//
	// 25 is the smoothness ceiling that still fits a GIF: the format stores frame
	// delays in hundredths of a second, so 25 fps is an exact 4 cs per frame and
	// every browser honours it (0–1 cs is what gets clamped to a 10 cs delay, not
	// 4). 10 fps — the first choice — reads as visibly choppy on motion, and the
	// size cost is sub-linear because unchanged pixels are transmitted
	// transparently: a 5 s 480 px range measured 361 KB at 10 fps and 834 KB at
	// 25.
	highlightGIFDefaultFPS  = 25
	highlightGIFFallbackFPS = 15

	// highlightTasksUpdateInterval coalesces the dashboard event the same way the
	// translation queue does: a burst of finished rows is one redraw.
	highlightTasksUpdateInterval = time.Second
)

// Per-attempt ffmpeg bounds. A remote favorite's first byte may wait on an
// OpenList/WebDAV backend, so it gets more patience than a local disk.
const (
	highlightLocalTimeout  = 60 * time.Second
	highlightRemoteTimeout = 90 * time.Second
)

// Error messages shown on a failed card (docs/spec/highlight-moments.md §5.5).
// They say what the user has to change, not which syscall failed.
const (
	highlightErrSourceMissing = "源文件缺失，无法生成精彩时刻"
	highlightErrVolumeOffline = "磁盘未接入，无法生成精彩时刻"
	highlightErrSourceOffline = "网盘离线，无法生成精彩时刻"
	highlightErrTrashed       = "影片在回收站中，恢复后可重试"
	highlightErrDeletedVideo  = "影片已被删除"
)

// HighlightTaskStatus is the read-only projection of the generator behind the
// 后台任务 dashboard row.
//
// The invariant is Done + Failed + pending == Total, where pending is derived
// (Total - Done - Failed) rather than stored: a cancellation counts as Done and
// is also counted in Canceled so the summary can name it.
type HighlightTaskStatus struct {
	// Active is true while anything is queued or being generated.
	Active bool `json:"active"`
	// Total is how many highlights this round has accepted.
	Total int `json:"total"`
	// Done counts successes and cancellations; Failed counts generations that
	// ended with an error.
	Done     int `json:"done"`
	Failed   int `json:"failed"`
	Canceled int `json:"canceled"`
	// Current names what the worker is generating right now ("文件名 · 1:30"),
	// empty between jobs.
	Current string `json:"current,omitempty"`
	// StartedAt and UpdatedAt are unix milliseconds.
	StartedAt int64 `json:"startedAt,omitempty"`
	UpdatedAt int64 `json:"updatedAt,omitempty"`
}

// Pending is the work the round has accepted but not resolved yet. It is derived
// so the invariant cannot drift from the counters.
func (s HighlightTaskStatus) Pending() int {
	pending := s.Total - s.Done - s.Failed
	if pending < 0 {
		return 0
	}
	return pending
}

// highlightRunState is the mutable half of HighlightTaskStatus, guarded by
// highlightState.mu.
//
// The pending set is the source of truth for Total: the total is derived from
// Done + Failed + len(pending) rather than incremented alongside them, which is
// what makes "Done + Failed + 当前 pending == Total" impossible to break. It also
// makes queueing idempotent, so the startup recovery cannot count a row this
// process had already queued twice.
type highlightRunState struct {
	Done      int
	Failed    int
	Canceled  int
	Current   string
	StartedAt int64
	UpdatedAt int64
	pending   map[int64]struct{}
}

func (r *highlightRunState) pendingCount() int { return len(r.pending) }

func (r *highlightRunState) total() int {
	return r.Done + r.Failed + len(r.pending)
}

// track accepts rows into the round, starting a new one when nothing is pending:
// a stale "上次生成" must not be added to. Rows already in the set are ignored,
// so a repeated recovery or a retried row is still one item.
func (r *highlightRunState) track(ids []int64, now int64) {
	if len(ids) == 0 {
		return
	}
	if len(r.pending) == 0 {
		r.Done, r.Failed, r.Canceled, r.Current = 0, 0, 0, ""
		r.StartedAt = now
	}
	if r.pending == nil {
		r.pending = make(map[int64]struct{}, len(ids))
	}
	for _, id := range ids {
		r.pending[id] = struct{}{}
	}
	r.UpdatedAt = now
}

// resolve closes one row. It reports whether the row belonged to this round, so a
// late worker answer about a row the user already removed cannot move the
// counters a second time.
func (r *highlightRunState) resolve(id int64, success bool, now int64) bool {
	if _, ok := r.pending[id]; !ok {
		return false
	}
	delete(r.pending, id)
	if success {
		r.Done++
	} else {
		r.Failed++
	}
	r.Current = ""
	r.UpdatedAt = now
	return true
}

// cancel closes rows the user removed. A cancellation is an outcome, so it counts
// as Done and is also counted in Canceled for the summary.
func (r *highlightRunState) cancel(ids []int64, now int64) {
	canceled := 0
	for _, id := range ids {
		if _, ok := r.pending[id]; !ok {
			continue
		}
		delete(r.pending, id)
		r.Done++
		r.Canceled++
		canceled++
	}
	if canceled > 0 {
		r.UpdatedAt = now
	}
}

// highlightState owns the generator: its queue, its worker, the round counters
// and the seams tests use to describe ffmpeg without running it.
type highlightState struct {
	mu      sync.Mutex
	queue   *workQueue[int64]
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
	closed  bool
	// seq numbers the generated files within this process, so two rows created
	// in the same nanosecond cannot collide.
	seq atomic.Int64
	run highlightRunState
	// emitScheduled is the coalescing flag for the dashboard event.
	emitScheduled bool

	// Seams. wired by NewService to the real ffmpeg generator and os.Stat; tests
	// replace them to describe a generator that succeeds, fails, overshoots the
	// size ceiling or blocks until the app shuts down.
	generateMoment func(ctx context.Context, videoPath string, atMs int64, outPath string, timeout time.Duration) error
	generateGIF    func(ctx context.Context, videoPath string, startMs, endMs int64, fps int, outPath string, timeout time.Duration) error
	fileSize       func(path string) (int64, error)
}

// StartHighlights starts the generator and re-queues whatever the previous
// process left behind. It is called once at startup; a second call is a no-op.
//
// The recovery pass runs before the worker picks anything up, so a row the last
// process had marked running is already back in queued (or failed) when the round
// counters are set. That is what makes "重启后自动重排" survive an unclean exit.
func (s *Service) StartHighlights() {
	s.highlights.mu.Lock()
	if s.highlights.started {
		s.highlights.mu.Unlock()
		return
	}
	s.highlights.started = true
	s.highlights.queue = newWorkQueue[int64]()
	s.highlights.ctx, s.highlights.cancel = context.WithCancel(context.Background())
	s.highlights.wg.Add(1)
	s.highlights.mu.Unlock()

	go s.highlightWorker()

	requeue, failed, err := s.repo.RecoverHighlights(highlightMaxInterruptions)
	if err != nil {
		s.log.Error("recover highlights failed", "err", err)
		s.scheduleHighlightTasksUpdate()
		return
	}
	if failed > 0 {
		s.log.Warn("highlights abandoned after repeated interruptions", "count", failed)
	}
	// A recovery pass is a round of its own: the dashboard row describes the
	// work the user is about to see, not what some earlier session did.
	s.noteHighlightQueued(requeue...)
	for _, id := range requeue {
		s.enqueueHighlight(id)
	}
	if len(requeue) > 0 || failed > 0 {
		s.log.Info("highlights recovered", "queued", len(requeue), "failed", failed)
	}
	s.scheduleHighlightTasksUpdate()
}

// CloseHighlights stops the generator: it cancels the running ffmpeg, ends the
// queue and waits for the worker.
//
// A row that was being generated keeps its running status on purpose. Marking it
// failed would report the user's own quit as a failure; the next start treats it
// as an interruption and re-queues it.
func (s *Service) CloseHighlights() {
	s.highlights.mu.Lock()
	if !s.highlights.started || s.highlights.closed {
		s.highlights.mu.Unlock()
		return
	}
	s.highlights.closed = true
	queue, cancel := s.highlights.queue, s.highlights.cancel
	s.highlights.mu.Unlock()

	// Cancel before closing: the queue's consumer may be inside ffmpeg, and the
	// context is what stops it.
	cancel()
	queue.close()
	s.highlights.wg.Wait()
}

// ListHighlights returns a favorite's 精彩时刻 in timeline order, every status
// included. Filtering is a presentation decision: the desktop shows failed rows,
// the phone hides them.
func (s *Service) ListHighlights(videoID int64) ([]Highlight, error) {
	return s.repo.ListHighlights(videoID)
}

// AddHighlight queues a new 精彩时刻. The row is written with the path its file
// will have and returned immediately, so the dialog can close: the image itself
// arrives later through the update event.
func (s *Service) AddHighlight(videoID, startMs, endMs int64) (*Highlight, error) {
	v, err := s.repo.Get(videoID)
	if err != nil {
		return nil, err
	}
	if err := ValidateHighlightTimes(startMs, endMs, v.DurationMs); err != nil {
		return nil, err
	}
	// Reject a request the worker could only fail: the user is standing in front
	// of the dialog and can still plug the disk in.
	if _, err := s.highlightSource(v); err != nil {
		return nil, err
	}

	kind := HighlightKindMoment
	if endMs > 0 {
		kind = HighlightKindRange
	}
	now := nowMillis()
	h := &Highlight{
		VideoID:   videoID,
		Kind:      kind,
		StartMs:   startMs,
		EndMs:     endMs,
		MediaPath: highlightMediaPath(videoID, s.highlights.seq.Add(1), kind),
		Status:    HighlightStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.CreateHighlight(h); err != nil {
		return nil, err
	}
	// The row's path is already recorded, so the folder has to exist before the
	// worker writes into it — including on a fresh data dir and when a test
	// replaces the generator with something that writes the file directly.
	if err := s.ensureHighlightDir(); err != nil {
		_ = s.repo.DeleteHighlight(videoID, h.ID)
		return nil, err
	}
	s.noteHighlightQueued(h.ID)
	if !s.enqueueHighlight(h.ID) {
		// The queue is closing. The row stays queued and the next start picks it
		// up, so the user's click is not lost to a race with a quit.
		s.log.Warn("highlight queued during shutdown", "id", h.ID, "video", videoID)
	}
	s.log.Info("highlight queued", "id", h.ID, "video", videoID, "kind", kind, "startMs", startMs, "endMs", endMs)
	s.emitHighlightUpdated(h)
	s.scheduleHighlightTasksUpdate()
	return h, nil
}

// RetryHighlight puts a failed highlight back in the queue, keeping its times.
// Only an explicit failure is retried by hand; a crash is recovered automatically
// at startup.
func (s *Service) RetryHighlight(videoID, id int64) error {
	h, err := s.repo.GetVideoHighlight(videoID, id)
	if err != nil {
		return err
	}
	switch h.Status {
	case HighlightStatusQueued, HighlightStatusRunning:
		// Already waiting: pressing 重试 twice is still one job.
		return nil
	case HighlightStatusReady:
		return fmt.Errorf("该精彩时刻已生成，无需重试")
	}
	requeued, err := s.repo.RetryHighlight(videoID, id)
	if err != nil {
		return err
	}
	if !requeued {
		// The row changed under us (deleted, or already retried elsewhere).
		return nil
	}
	h.Status = HighlightStatusQueued
	h.Error = ""
	h.FinishedAt = 0
	h.UpdatedAt = nowMillis()
	s.noteHighlightQueued(id)
	if !s.enqueueHighlight(id) {
		s.log.Warn("highlight retried during shutdown", "id", id, "video", videoID)
	}
	s.log.Info("highlight retried", "id", id, "video", videoID)
	s.emitHighlightUpdated(h)
	s.scheduleHighlightTasksUpdate()
	return nil
}

// DeleteHighlight removes one highlight and the file it generated.
//
// Deleting a pending row is the cancel gesture. A row that is being generated is
// removed immediately; the running ffmpeg finishes into a file nobody owns, and
// the worker deletes it when its final write finds the row gone.
func (s *Service) DeleteHighlight(videoID, id int64) error {
	h, err := s.repo.GetVideoHighlight(videoID, id)
	if errors.Is(err, ErrNotFound) {
		// Already gone, which is the outcome the caller asked for: a card goes
		// stale whenever another window removed the same highlight.
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.repo.DeleteHighlight(videoID, id); err != nil {
		return err
	}
	s.removeHighlightFile(h.MediaPath)
	if h.IsPending() {
		s.cancelHighlights([]int64{h.ID})
	}
	s.log.Info("highlight deleted", "id", id, "video", videoID, "status", h.Status)
	s.emitHighlightRemoved(h)
	s.scheduleHighlightTasksUpdate()
	return nil
}

// HighlightTaskStatus returns a copy of the current (or last) round.
func (s *Service) HighlightTaskStatus() HighlightTaskStatus {
	s.highlights.mu.Lock()
	defer s.highlights.mu.Unlock()
	run := s.highlights.run
	return HighlightTaskStatus{
		Active:    run.pendingCount() > 0,
		Total:     run.total(),
		Done:      run.Done,
		Failed:    run.Failed,
		Canceled:  run.Canceled,
		Current:   run.Current,
		StartedAt: run.StartedAt,
		UpdatedAt: run.UpdatedAt,
	}
}

// ---- worker ----

// highlightWorker renders queued highlights one at a time. One worker is the
// point: ffmpeg is CPU and disk bound, and a batch of moments would otherwise
// fight the cover extractor for the same drive.
func (s *Service) highlightWorker() {
	defer s.highlights.wg.Done()
	for {
		id, ok := s.highlights.queue.pop()
		if !ok {
			return
		}
		if s.highlights.ctx.Err() != nil {
			// Closing: drain without starting another ffmpeg run.
			return
		}
		s.processHighlight(id)
	}
}

// processHighlight renders one row.
//
// Every early return leaves the dashboard counters consistent: either the row was
// resolved here, or it was deleted by the user (which already counted the
// cancellation), or a shutdown left it running for the next start to recover.
func (s *Service) processHighlight(id int64) {
	h, err := s.repo.GetHighlight(id)
	if err != nil {
		// The row was deleted while it waited. DeleteHighlight counted that.
		return
	}
	if h.Status != HighlightStatusQueued {
		return
	}
	claimed, err := s.repo.ClaimHighlight(id)
	if err != nil {
		s.log.Error("claim highlight failed", "id", id, "err", err)
		return
	}
	if !claimed {
		// Deleted (or claimed elsewhere) between the read and the update.
		return
	}
	h.Status = HighlightStatusRunning

	v, err := s.repo.GetAny(h.VideoID)
	if err != nil {
		// The favorite is gone. Its rows normally went with it (the delete runs in
		// one transaction), so reaching here means an orphan left by an interrupted
		// delete: record it as failed instead of leaving it counted as in flight
		// forever.
		s.finishHighlight(h, HighlightStatusFailed, highlightErrDeletedVideo)
		return
	}
	s.setHighlightCurrent(v.FileName, h)

	// The disk may have been unplugged, the file deleted or the cloud source
	// gone since the row was queued. Each case has its own message.
	if v.DeletedAt > 0 {
		s.finishHighlight(h, HighlightStatusFailed, highlightErrTrashed)
		return
	}
	media, srcErr := s.highlightSource(v)
	if srcErr != nil {
		s.finishHighlight(h, HighlightStatusFailed, srcErr.Error())
		return
	}

	outPath := s.highlightPath(h)
	if genErr := s.generateHighlight(s.highlights.ctx, v, h, media, outPath); genErr != nil {
		if s.highlights.ctx.Err() != nil {
			// Shutdown killed ffmpeg. The row stays running: the next start
			// recovers it and bumps the interruption counter instead of
			// reporting a failure the user did not cause.
			s.clearHighlightCurrent()
			return
		}
		_ = os.Remove(outPath)
		s.finishHighlight(h, HighlightStatusFailed, "生成精彩时刻失败："+genErr.Error())
		return
	}
	s.finishHighlight(h, HighlightStatusReady, "")
}

// generateHighlight renders the row's file at exactly one path: a moment is the
// frame at StartMs, a range is the GIF of [StartMs, EndMs).
//
// The 8 MB ceiling is handled here rather than in the generator because only the
// worker knows the row: a retry overwrites the same path, so the UI never sees an
// intermediate file.
func (s *Service) generateHighlight(ctx context.Context, v *Video, h *Highlight, media, outPath string) error {
	timeout := highlightLocalTimeout
	if v.IsRemote() {
		timeout = highlightRemoteTimeout
	}
	if h.Kind == HighlightKindMoment {
		return s.highlights.generateMoment(ctx, media, h.StartMs, outPath, timeout)
	}
	if err := s.highlights.generateGIF(ctx, media, h.StartMs, h.EndMs, highlightGIFDefaultFPS, outPath, timeout); err != nil {
		return err
	}
	size, err := s.highlights.fileSize(outPath)
	if err != nil || size <= highlightGIFMaxBytes {
		return nil
	}
	s.log.Info("highlight gif over the size ceiling, retrying at a lower frame rate",
		"id", h.ID, "bytes", size, "fps", highlightGIFFallbackFPS)
	return s.highlights.generateGIF(ctx, media, h.StartMs, h.EndMs, highlightGIFFallbackFPS, outPath, timeout)
}

// highlightSource answers "can this favorite's bytes be read right now?" and
// returns what ffmpeg should open. Missing, detached disk and offline cloud are
// three different problems with three different fixes.
func (s *Service) highlightSource(v *Video) (string, error) {
	if v.IsRemote() {
		if s.remoteOnline != nil && !s.remoteOnline(v) {
			return "", errors.New(highlightErrSourceOffline)
		}
		if s.processor == nil {
			return "", errors.New("网盘媒体不可用")
		}
		return s.processor.MediaPath(v)
	}
	if root := s.volumeDetached(v.Path); root != "" {
		return "", errors.New(highlightErrVolumeOffline)
	}
	if _, err := s.stat(v.Path); err != nil {
		return "", errors.New(highlightErrSourceMissing)
	}
	return v.Path, nil
}

// finishHighlight writes a terminal status and accounts for it.
//
// A false answer from the repository means the user deleted the row while it was
// being generated: DeleteHighlight already counted that cancellation, so counting
// it again would break the round's invariant, and the file it just produced is
// nobody's and is removed here.
func (s *Service) finishHighlight(h *Highlight, status, message string) {
	finished := nowMillis()
	written, err := s.repo.FinishHighlight(h.ID, status, message, finished)
	if err != nil {
		s.log.Error("write highlight result failed", "id", h.ID, "status", status, "err", err)
	}
	if !written {
		_ = os.Remove(s.highlightPath(h))
		s.clearHighlightCurrent()
		return
	}
	h.Status = status
	h.Error = message
	h.FinishedAt = finished
	h.UpdatedAt = finished
	s.resolveHighlight(h.ID, status == HighlightStatusReady)
	if status == HighlightStatusFailed {
		s.log.Warn("highlight generation failed", "id", h.ID, "video", h.VideoID, "err", message)
	} else {
		s.log.Info("highlight generated", "id", h.ID, "video", h.VideoID, "path", h.MediaPath)
	}
	s.emitHighlightUpdated(h)
	s.scheduleHighlightTasksUpdate()
}

// ---- queue and counters ----

// enqueueHighlight hands an id to the worker. False means the queue is closed:
// the row stays queued for the next start.
func (s *Service) enqueueHighlight(id int64) bool {
	s.highlights.mu.Lock()
	queue := s.highlights.queue
	s.highlights.mu.Unlock()
	if queue == nil {
		return false
	}
	return queue.push(id)
}

// noteHighlightQueued accepts rows a producer has handed to the queue. When the
// round has nothing pending this starts a new one and resets the counters, so the
// dashboard row always describes the work that just arrived.
func (s *Service) noteHighlightQueued(ids ...int64) {
	if len(ids) == 0 {
		return
	}
	s.highlights.mu.Lock()
	s.highlights.run.track(ids, nowMillis())
	s.highlights.mu.Unlock()
	s.scheduleHighlightTasksUpdate()
}

// resolveHighlight closes one row. A row the user already removed is not in the
// round any more, so its late answer changes nothing.
func (s *Service) resolveHighlight(id int64, success bool) {
	s.highlights.mu.Lock()
	s.highlights.run.resolve(id, success, nowMillis())
	s.highlights.mu.Unlock()
}

// cancelHighlights closes pending rows the user removed.
func (s *Service) cancelHighlights(ids []int64) {
	if len(ids) == 0 {
		return
	}
	s.highlights.mu.Lock()
	s.highlights.run.cancel(ids, nowMillis())
	s.highlights.mu.Unlock()
}

// setHighlightCurrent publishes what the worker just started on.
func (s *Service) setHighlightCurrent(fileName string, h *Highlight) {
	label := HighlightTimeLabel(h)
	if fileName != "" {
		label = fileName + " · " + label
	}
	s.highlights.mu.Lock()
	s.highlights.run.Current = label
	s.highlights.run.UpdatedAt = nowMillis()
	s.highlights.mu.Unlock()
	s.scheduleHighlightTasksUpdate()
}

func (s *Service) clearHighlightCurrent() {
	s.highlights.mu.Lock()
	s.highlights.run.Current = ""
	s.highlights.mu.Unlock()
}

// ---- files and events ----

// highlightPath is the absolute path of a row's generated file. The recorded
// value is relative to the data dir; only its base name is trusted, so a crafted
// media_path can never point outside highlights/.
func (s *Service) highlightPath(h *Highlight) string {
	return filepath.Join(s.dataDir, HighlightsRelativeDir, filepath.Base(filepath.FromSlash(h.MediaPath)))
}

// ensureHighlightDir creates the folder the generated files live in. It is
// created on demand rather than only at startup so a data dir restored without
// its empty folders still works.
func (s *Service) ensureHighlightDir() error {
	if err := os.MkdirAll(filepath.Join(s.dataDir, HighlightsRelativeDir), 0o755); err != nil {
		return fmt.Errorf("create highlights dir: %w", err)
	}
	return nil
}

// removeHighlightFile deletes one generated file. Best effort: a running row's
// file may not exist yet, and the orphan sweep is the backstop for anything that
// survives a race.
func (s *Service) removeHighlightFile(rel string) {
	if rel == "" {
		return
	}
	file := filepath.Join(s.dataDir, HighlightsRelativeDir, filepath.Base(filepath.FromSlash(rel)))
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		s.log.Warn("remove highlight file failed", "file", file, "err", err)
	}
}

// pendingHighlightIDs returns a favorite's still-pending highlights. A permanent
// delete runs this first so the round can account for the rows its cascade is
// about to remove: without it the dashboard would keep them as "pending" forever.
func (s *Service) pendingHighlightIDs(videoID int64) []int64 {
	list, err := s.repo.ListHighlights(videoID)
	if err != nil {
		s.log.Warn("list highlights for removal failed", "video", videoID, "err", err)
		return nil
	}
	ids := make([]int64, 0, len(list))
	for i := range list {
		if list[i].IsPending() {
			ids = append(ids, list[i].ID)
		}
	}
	return ids
}

// emitHighlightUpdated announces one row's state change. Both clients key their
// detail refresh off it: the desktop invalidates and re-reads, the phone has no
// push and polls while anything is pending.
func (s *Service) emitHighlightUpdated(h *Highlight) {
	if s.emit == nil {
		return
	}
	s.emit("video:highlight:updated", map[string]any{
		"id":      h.ID,
		"videoId": h.VideoID,
		"status":  h.Status,
	})
}

func (s *Service) emitHighlightRemoved(h *Highlight) {
	if s.emit == nil {
		return
	}
	s.emit("video:highlight:updated", map[string]any{
		"id":      h.ID,
		"videoId": h.VideoID,
		"status":  "deleted",
	})
}

// scheduleHighlightTasksUpdate emits one coalesced "the dashboard changed" event
// per interval. The trailing timer guarantees the last result is announced
// instead of being folded away.
func (s *Service) scheduleHighlightTasksUpdate() {
	s.highlights.mu.Lock()
	if s.highlights.emitScheduled {
		s.highlights.mu.Unlock()
		return
	}
	s.highlights.emitScheduled = true
	s.highlights.mu.Unlock()
	time.AfterFunc(highlightTasksUpdateInterval, func() {
		s.highlights.mu.Lock()
		s.highlights.emitScheduled = false
		s.highlights.mu.Unlock()
		if s.emit != nil {
			s.emit("tasks:updated")
		}
	})
}
