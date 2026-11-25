package video

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// 精彩时刻 (docs/spec/highlight-moments.md): a favorite's saved frame or time
// range, rendered by ffmpeg into a generated image next to the thumbnails.
//
// This file is the domain vocabulary only — the model, the validation rules and
// the shape of the generated file name. The queue, the worker and the dashboard
// counters live in highlights.go; the repository in highlight_repository.go.

// Highlight kinds. A moment is a single frame at start_ms; a range is the
// [start_ms, end_ms) span rendered as a looping GIF.
const (
	HighlightKindMoment = "moment"
	HighlightKindRange  = "range"
)

// Highlight lifecycle. queued and running are pending (the user can still cancel
// them by deleting the row); ready and failed are terminal.
const (
	HighlightStatusQueued  = "queued"
	HighlightStatusRunning = "running"
	HighlightStatusReady   = "ready"
	HighlightStatusFailed  = "failed"
)

// Highlight range bounds: a GIF shorter than one second is not a moment worth
// saving, and one longer than ten seconds is a file the user did not ask for.
const (
	MinHighlightRangeMs int64 = 1000
	MaxHighlightRangeMs int64 = 10000
)

// Highlight is one generated image and the job that produced it.
//
// The row is the durable truth: the in-memory queue can be lost at any moment
// (a crash), so every state transition is written here first and the queue only
// carries ids. media_path is relative to the data dir, like thumbnail_path.
type Highlight struct {
	ID        int64  `json:"id"`
	VideoID   int64  `json:"videoId"`
	Kind      string `json:"kind"`      // moment | range
	StartMs   int64  `json:"startMs"`   // where the frame / GIF begins
	EndMs     int64  `json:"endMs"`     // 0 for a moment
	MediaPath string `json:"mediaPath"` // relative to the data dir, e.g. "highlights/h-12-…-1.jpg"
	Status    string `json:"status"`    // queued | running | ready | failed
	Error     string `json:"error"`
	// Interruptions counts how many times the app died while this row was
	// running. Reaching the limit marks it failed instead of requeueing it
	// again, so one poisonous file cannot restart the app forever.
	Interruptions int   `json:"interruptions,omitempty"`
	CreatedAt     int64 `json:"createdAt"`
	UpdatedAt     int64 `json:"updatedAt"`
	FinishedAt    int64 `json:"finishedAt"`
}

// IsPending reports whether the row is still waiting for (or being served by)
// the generator, i.e. whether deleting it means "cancel".
func (h *Highlight) IsPending() bool {
	return h != nil && (h.Status == HighlightStatusQueued || h.Status == HighlightStatusRunning)
}

// HighlightTimeLabel renders a highlight's time as the UI shows it: "1:30" for a
// moment, "1:30–1:35" for a range. It is used where a terse one-line label is
// needed (the dashboard's Current line, log messages).
func HighlightTimeLabel(h *Highlight) string {
	if h == nil {
		return ""
	}
	if h.Kind == HighlightKindRange && h.EndMs > 0 {
		return FormatHighlightTime(h.StartMs) + "–" + FormatHighlightTime(h.EndMs)
	}
	return FormatHighlightTime(h.StartMs)
}

// FormatHighlightTime renders milliseconds as m:ss, or h:mm:ss past an hour. It
// mirrors utils/highlightTime.ts's formatHighlightTime so a label written by the
// backend and one written by the UI read the same.
func FormatHighlightTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	total := ms / 1000
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// ValidateHighlightTimes applies the backend half of the time rules in
// docs/spec/highlight-moments.md §3.3. The frontend runs the same checks so the
// Save button can be disabled live; this copy exists because a client is never
// the authority. durationMs <= 0 means "the length of the video is unknown", in
// which case the duration checks are skipped rather than guessed.
//
// endMs == 0 means "a moment" (no range), which is the only value that skips the
// range rules.
func ValidateHighlightTimes(startMs, endMs, durationMs int64) error {
	if startMs < 0 {
		return errors.New("开始时间无效")
	}
	if durationMs > 0 && startMs >= durationMs {
		return errors.New("开始时间超出视频时长")
	}
	if endMs <= 0 {
		return nil
	}
	if endMs <= startMs {
		return errors.New("结束时间必须晚于开始时间")
	}
	length := endMs - startMs
	if length < MinHighlightRangeMs {
		return errors.New("时间段至少 1 秒")
	}
	if length > MaxHighlightRangeMs {
		return fmt.Errorf("时间段最长 10 秒（当前 %d 秒）", length/1000)
	}
	if durationMs > 0 && endMs > durationMs {
		return errors.New("结束时间超出视频时长")
	}
	return nil
}

// HighlightsRelativeDir is the folder the generated 精彩时刻 files live in,
// relative to the data dir. It is a sibling of thumbnails/ so the two sweeps
// never have to agree about each other's files.
const HighlightsRelativeDir = "highlights"

// HighlightsPreviewDir holds a rendered highlight the user has not confirmed
// yet. It is deliberately a separate folder from highlights/: files there are
// owned by a row, and the orphan sweep deletes anything a row does not declare.
// A preview has no row yet, so putting it in highlights/ would make the sweep
// eat a preview of a dialog that is still open.
const HighlightsPreviewDir = "preview"

// HighlightPreview is one rendered-but-unconfirmed highlight: what the dialog
// shows before the user decides to keep it.
//
// It carries the request it was rendered for, so the client can tell "preview of
// the times in the form" from "preview of what I typed a moment ago", and the
// save path can verify that the file it is about to adopt was made for exactly
// those times.
type HighlightPreview struct {
	// Path is relative to the data dir, e.g. "preview/p-12-90000-95000-…-1.gif".
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	// Bytes is the size of the rendered file, so the dialog can show it before
	// the user commits.
	Bytes int64 `json:"bytes"`
	// FPS is the frame rate the GIF ended up with (after the 8 MB retry); 0 for a
	// moment.
	FPS int `json:"fps"`
}

// highlightFileNamePattern is the exact shape of a generated highlight file:
// h-<videoID>-<unixNano>-<seq>.<jpg|gif>. Everything that touches these files
// (the sweep, the desktop asset handler, the LAN media route) matches against
// this one expression: a false positive on a media route serves a file the app
// did not write, and a false positive in the sweep deletes one.
var highlightFileNamePattern = regexp.MustCompile(`^h-\d+-\d+-\d+\.(jpg|gif)$`)

// highlightPreviewPattern is the shape of a preview: the same generated-file
// guard, plus the request it was rendered for —
// p-<videoID>-<startMs>-<endMs>-<unixNano>-<seq>.<jpg|gif>.
var highlightPreviewPattern = regexp.MustCompile(`^p-(\d+)-(\d+)-(\d+)-(\d+)-(\d+)\.(jpg|gif)$`)

// ValidHighlightFileName reports whether name is exactly one of the files the
// generator writes. Callers must pass a base name, never a path.
func ValidHighlightFileName(name string) bool {
	return highlightFileNamePattern.MatchString(name)
}

// ValidHighlightPreviewFileName reports whether name is exactly one of the files
// a preview can write. The desktop asset handler uses it to serve previews.
func ValidHighlightPreviewFileName(name string) bool {
	return highlightPreviewPattern.MatchString(name)
}

// highlightPreviewMeta is the request encoded in a preview's file name.
type highlightPreviewMeta struct {
	videoID int64
	startMs int64
	endMs   int64
	ext     string
}

// parseHighlightPreviewName decodes a preview name. The times are part of the
// name so that adopting a preview can be verified against the request instead of
// trusted: a stale preview from another edit cannot be saved as if it described
// the times in the form.
func parseHighlightPreviewName(name string) (highlightPreviewMeta, bool) {
	match := highlightPreviewPattern.FindStringSubmatch(name)
	if match == nil {
		return highlightPreviewMeta{}, false
	}
	numbers := make([]int64, 0, 5)
	for i := 1; i <= 5; i++ {
		value, err := strconv.ParseInt(match[i], 10, 64)
		if err != nil {
			return highlightPreviewMeta{}, false
		}
		numbers = append(numbers, value)
	}
	return highlightPreviewMeta{
		videoID: numbers[0],
		startMs: numbers[1],
		endMs:   numbers[2],
		ext:     "." + match[6],
	}, true
}

// highlightPreviewPath builds the relative path of a new preview file. The
// request is part of the name (see parseHighlightPreviewName); n makes two
// renders of the same request distinct.
func highlightPreviewPath(videoID, startMs, endMs, n int64, kind string) string {
	return HighlightsPreviewDir + "/p-" + strconv.FormatInt(videoID, 10) + "-" +
		strconv.FormatInt(startMs, 10) + "-" + strconv.FormatInt(endMs, 10) + "-" +
		strconv.FormatInt(nowNanos(), 10) + "-" + strconv.FormatInt(n, 10) + highlightFileExt(kind)
}

// highlightFileExt is the extension for a kind. A moment is always a JPEG, a
// range always a GIF.
func highlightFileExt(kind string) string {
	if kind == HighlightKindRange {
		return ".gif"
	}
	return ".jpg"
}

// highlightMediaPath builds the relative path of a new highlight file. n is a
// process-wide sequence number: two rows created in the same nanosecond (a batch
// of moments) must still get different names, and the file name is also what
// makes a stale file recognisable to the sweep.
func highlightMediaPath(videoID, n int64, kind string) string {
	return HighlightsRelativeDir + "/h-" + strconv.FormatInt(videoID, 10) + "-" +
		strconv.FormatInt(nowNanos(), 10) + "-" + strconv.FormatInt(n, 10) + highlightFileExt(kind)
}
