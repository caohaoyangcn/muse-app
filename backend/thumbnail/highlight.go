package thumbnail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"muse-app/backend/platform"
)

// 精彩时刻 generation (docs/spec/highlight-moments.md §7).
//
// Two products come out of ffmpeg here: a single frame saved as a JPEG, and a
// time range saved as a looping GIF. Both read the source video and write one
// derived file elsewhere; the source is never modified, which is the same
// "收藏 ≠ 文件" contract the rest of the app keeps.
//
// Unlike the cover extractor next door, the output path is decided by the caller
// (the highlight row already recorded it), so nothing here invents a name.

const (
	// HighlightMomentWidth is the width of a saved moment frame. The height
	// follows the source aspect ratio.
	HighlightMomentWidth = 1280
	// HighlightGIFWidth is the width of a looping GIF. 480 keeps a ten-second
	// loop inside the size ceiling on ordinary material.
	HighlightGIFWidth = 480
	// HighlightGIFColors caps the GIF palette. GIF itself allows 256; 128 is
	// the size/quality compromise the spec picked.
	HighlightGIFColors = 128
)

var (
	// ErrFFmpegMissing means the tool could not be resolved at startup, so no
	// generation can be attempted at all.
	ErrFFmpegMissing = errors.New("未找到 ffmpeg")
	// ErrHighlightTimeout means one ffmpeg attempt exceeded its own bound. It is
	// a definite failure: the user retries by hand rather than the app retrying
	// a file that may take the worker down every time.
	ErrHighlightTimeout = errors.New("ffmpeg 超时")
)

// MomentArgs is the ffmpeg command line for a single frame.
//
// -ss before -i is the fast seek: the decoder starts at the requested instant
// instead of decoding everything before it. A highlight at 1:30 of a 2-hour file
// would otherwise take minutes.
func MomentArgs(videoPath string, atMs int64, outPath string) []string {
	args := []string{"-y", "-hide_banner", "-loglevel", "error"}
	if atMs > 0 {
		args = append(args, "-ss", formatSeconds(atMs))
	}
	return append(args,
		"-i", videoPath,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:-2", HighlightMomentWidth),
		"-q:v", "3",
		outPath,
	)
}

// GIFArgs is the ffmpeg command line for a time range.
//
// The filter chain is the compression story: the palette is built from the
// difference between frames (stats_mode=diff) and applied with rectangle
// differencing, so flat, unchanged areas cost almost nothing while single-frame
// quality is untouched. +transdiff completes that by writing transparent frames
// for the regions that did not change.
func GIFArgs(videoPath string, startMs, endMs int64, fps int, outPath string) []string {
	return []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-ss", formatSeconds(startMs),
		"-t", formatSeconds(endMs - startMs),
		"-i", videoPath,
		"-an", "-loop", "0",
		"-vf", gifFilter(fps),
		"-gifflags", "+transdiff",
		outPath,
	}
}

// gifFilter is the single -vf argument. It has to stay on one line: ffmpeg
// treats the whole string as one filter graph and does not accept newlines here.
func gifFilter(fps int) string {
	return fmt.Sprintf(
		"fps=%d,scale=%d:-2:flags=lanczos,"+
			"split[s0][s1];"+
			"[s0]palettegen=max_colors=%d:stats_mode=diff[p];"+
			"[s1][p]paletteuse=dither=bayer:bayer_scale=3:diff_mode=rectangle",
		fps, HighlightGIFWidth, HighlightGIFColors)
}

// GenerateHighlightMoment writes the frame at atMs to outPath as a JPEG.
func GenerateHighlightMoment(ctx context.Context, ffmpegBin, videoPath string, atMs int64, outPath string, timeout time.Duration) error {
	return runHighlight(ctx, ffmpegBin, MomentArgs(videoPath, atMs, outPath), outPath, timeout)
}

// GenerateHighlightGIF writes [startMs, endMs) to outPath as a looping GIF.
//
// fps is a parameter rather than a constant because the worker regenerates at a
// lower frame rate when the first file exceeds the size ceiling.
func GenerateHighlightGIF(ctx context.Context, ffmpegBin, videoPath string, startMs, endMs int64, fps int, outPath string, timeout time.Duration) error {
	return runHighlight(ctx, ffmpegBin, GIFArgs(videoPath, startMs, endMs, fps, outPath), outPath, timeout)
}

// runHighlight executes one ffmpeg attempt and guarantees outPath is either a
// complete file or absent.
//
// The distinction between the two error kinds matters to the worker: a canceled
// parent context is the application shutting down (leave the row running so the
// next start can recover it), while everything else is a failure to report.
func runHighlight(parent context.Context, ffmpegBin string, args []string, outPath string, timeout time.Duration) error {
	if ffmpegBin == "" {
		return ErrFFmpegMissing
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create highlight dir: %w", err)
	}
	if timeout <= 0 {
		timeout = ffmpegTimeout
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := platform.CommandContext(ctx, ffmpegBin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// A half-written file must not survive: the row would be marked failed
		// while a file bearing its name still existed.
		_ = os.Remove(outPath)
		if parent.Err() != nil {
			return parent.Err()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ErrHighlightTimeout
		}
		if line := strings.TrimSpace(firstLine(stderr.String())); line != "" {
			return errors.New(line)
		}
		return fmt.Errorf("ffmpeg: %w", err)
	}

	// ffmpeg can exit 0 having written nothing (an empty range, a stream it
	// could not decode). An empty file is not a saved moment.
	info, err := os.Stat(outPath)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(outPath)
		if line := strings.TrimSpace(firstLine(stderr.String())); line != "" {
			return errors.New(line)
		}
		return errors.New("ffmpeg 没有生成输出文件")
	}
	return nil
}

// formatSeconds renders milliseconds as the "12.345" form ffmpeg's -ss/-t take.
func formatSeconds(ms int64) string {
	return strconv.FormatFloat(float64(ms)/1000, 'f', 3, 64)
}
