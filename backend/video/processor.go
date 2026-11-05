package video

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"muse-app/backend/thumbnail"
)

const thumbnailAttempts = 3
const thumbnailRetryDelay = 2 * time.Second

// Processor is a bounded worker pool that extracts metadata and thumbnails
// in the background. Two workers cap concurrent ffmpeg processes when large
// batches are dropped in (spec-v1.md §28, spec-v1.1.md §11).
type Processor struct {
	jobs              chan int64
	ctx               context.Context
	cancel            context.CancelFunc
	repo              VideoRepository
	ffprobeBin        string
	ffmpegBin         string
	thumbDir          string
	generateThumbnail func(context.Context, string, string, int64, string, int64) (string, error)
	sleep             func(time.Duration)
	emit              func(name string, data ...any)
	log               *slog.Logger
	wg                sync.WaitGroup
	closeOnce         sync.Once
	jobsMu            sync.RWMutex
	closed            bool
	// autoGroup4K, when set, is invoked after metadata extraction for a video
	// confirmed to be 3840×2160 so the orchestrator can auto-assign it to the
	// "4K" group (creating it on first use). Set by the caller (wiring) — kept
	// out of the video domain to avoid the api↔video dependency cycle.
	autoGroup4K func(videoID int64)
	mu          sync.RWMutex
}

// SetAutoGroup4K installs the optional callback that assigns a detected 4K
// video to the auto-created "4K" group (see the comment on autoGroup4K).
func (p *Processor) SetAutoGroup4K(fn func(videoID int64)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.autoGroup4K = fn
}

// FFmpegBin exposes the resolved ffmpeg path for callers that need to run a
// one-off ffmpeg command in the same tool environment (e.g. the cover picker).
func (p *Processor) FFmpegBin() string { return p.ffmpegBin }

func NewProcessor(repo VideoRepository, ffprobeBin, ffmpegBin, thumbDir string,
	emit func(name string, data ...any), log *slog.Logger, workers int) *Processor {
	if workers < 1 {
		workers = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Processor{
		jobs:              make(chan int64, 512),
		ctx:               ctx,
		cancel:            cancel,
		repo:              repo,
		ffprobeBin:        ffprobeBin,
		ffmpegBin:         ffmpegBin,
		thumbDir:          thumbDir,
		generateThumbnail: thumbnail.GenerateContext,
		sleep:             time.Sleep,
		emit:              emit,
		log:               log,
	}
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
	return p
}

// Enqueue schedules background processing for a video. Never blocks the caller.
func (p *Processor) Enqueue(id int64) {
	p.jobsMu.Lock()
	defer p.jobsMu.Unlock()
	if p.closed {
		return
	}
	select {
	case p.jobs <- id:
	default:
		p.log.Warn("thumbnail queue full, dropping job", "id", id)
	}
}

// Close stops accepting new jobs and ends the workers. It cancels the probe or
// ffmpeg extraction in flight (the command is killed through its context) and
// deliberately does not drain the queue: a closed window must end the process,
// while a missing thumbnail is re-enqueued by the startup backfill.
func (p *Processor) Close() {
	p.closeOnce.Do(func() {
		p.cancel()
		p.jobsMu.Lock()
		p.closed = true
		close(p.jobs)
		p.jobsMu.Unlock()
		p.wg.Wait()
	})
}

func (p *Processor) worker() {
	defer p.wg.Done()
	for id := range p.jobs {
		// Close marks the context done before the queued ids are consumed, so
		// this drains the channel without starting each abandoned job.
		if p.ctx.Err() != nil {
			continue
		}
		p.process(id)
	}
}

func (p *Processor) process(id int64) {
	if p.ctx.Err() != nil {
		return
	}
	v, err := p.repo.Get(id)
	if err != nil {
		// The favorite may have been removed while queued; that is fine.
		p.log.Warn("process: video unavailable", "id", id, "err", err)
		return
	}

	var durationMs int64
	md, err := ProbeContext(p.ctx, p.ffprobeBin, v.Path)
	if err != nil {
		if p.ctx.Err() != nil {
			return // shutdown canceled the probe; not a metadata failure
		}
		// Metadata is optional for a favorite (spec-v1.md §29 non-critical).
		p.log.Warn("ffprobe failed", "id", id, "path", v.Path, "err", err)
	} else {
		durationMs = md.DurationMs
		if err := p.repo.UpdateMetadata(id, md.DurationMs, md.Width, md.Height, md.FrameRate, md.Bitrate, md.VideoCodec, md.AudioCodec); err != nil {
			p.log.Warn("update metadata failed", "id", id, "err", err)
		}
		// Auto-group rule: a confirmed 4K (3840×2160) video is added to the "4K"
		// group by the caller, asynchronously after ffprobe resolves the
		// resolution (spec: no group suggestion for resolution, it is automatic).
		if md.Width == 3840 && md.Height == 2160 {
			p.mu.RLock()
			fn := p.autoGroup4K
			p.mu.RUnlock()
			if fn != nil {
				fn(id)
			} else {
				p.log.Info("4K video detected, no auto-group handler", "id", id, "width", md.Width, "height", md.Height)
			}
		}
	}

	p.processThumbnail(v, durationMs)
}

// processThumbnail generates the automatic cover for the video and records it.
func (p *Processor) processThumbnail(v *Video, durationMs int64) {
	// The user picked this cover by hand; never let an automatic run overwrite
	// it (the metadata refresh above is still fine).
	if v.CustomCoverAt > 0 {
		return
	}

	generate := p.generateThumbnail
	if generate == nil {
		generate = thumbnail.GenerateContext
	}
	var rel string
	var err error
	sleep := p.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	for attempt := 1; attempt <= thumbnailAttempts; attempt++ {
		if p.ctx.Err() != nil {
			return // shutdown: do not start another extraction or retry
		}
		rel, err = generate(p.ctx, p.ffmpegBin, v.Path, durationMs, p.thumbDir, v.ID)
		if err == nil {
			break
		}
		if p.ctx.Err() != nil {
			// The context canceled the command; this is shutdown, not a
			// thumbnail failure worth retrying or reporting.
			return
		}
		if attempt < thumbnailAttempts {
			p.log.Warn("thumbnail attempt failed", "id", v.ID, "attempt", attempt, "err", err)
			sleep(thumbnailRetryDelay)
		}
	}
	if err != nil {
		p.log.Warn("thumbnail failed", "id", v.ID, "path", v.Path, "attempts", thumbnailAttempts, "err", err)
		return
	}
	if _, err := p.repo.UpdateThumbnailIfAuto(v.ID, rel); err != nil {
		p.log.Warn("update thumbnail failed", "id", v.ID, "err", err)
		return
	}
	p.log.Info("thumbnail generated", "id", v.ID)
	if p.emit != nil {
		p.emit("thumbnail:generated", map[string]any{"id": v.ID})
	}
}
