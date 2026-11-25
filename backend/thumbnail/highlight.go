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
// Two products come out of ffmpeg here: a single frame saved as a JPEG, and a time
// range saved as a short, muted, looping MP4. Both read the source video and write
// one derived file elsewhere; the source is never modified, which is the same
// "收藏 ≠ 文件" contract the rest of the app keeps.
//
// The range used to be a GIF. A GIF stores frame delays in hundredths of a second
// and caps out at 50 fps, so it cannot express a 60 fps loop at all — and it is
// roughly twenty times larger than h264 at the same size and quality, because it
// has no inter-frame prediction beyond "this rectangle is unchanged". An MP4 is
// smaller, smoother, full colour, and cheap enough to encode that the 20 s ceiling
// needs no size fallback.
//
// Unlike the cover extractor next door, the output path is decided by the caller
// (the highlight row already recorded it), so nothing here invents a name.

const (
	// HighlightMomentWidth is the width of a saved moment frame. The height
	// follows the source aspect ratio.
	HighlightMomentWidth = 1280
	// HighlightClipWidth is the width of a looping range. 480 is the scale the
	// GIF version established, kept so a saved highlight looks the same as it did
	// before the switch; at 60 fps a 20 s range is around 750 KB, so this is now a
	// quality/size choice rather than a hard limit.
	HighlightClipWidth = 480
	// clipCRF trades size for quality at a fixed quantiser. 26 is visually
	// transparent at 480 px and keeps a 20 s range under a megabyte.
	clipCRF = "26"
	// clipPreset: medium is the default speed/size balance; these clips are short,
	// so a faster preset would only save a fraction of a second.
	clipPreset = "medium"
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

// ClipArgs is the ffmpeg command line for a time range.
//
// -an drops the audio: a muted loop is what the UI plays, and an audio track
// would only add bytes. +faststart moves the index to the front so a phone can
// start playing before the file has finished arriving over the LAN. setsar=1
// removes any anamorphic pixel aspect so the browser does not stretch the frame.
func ClipArgs(videoPath string, startMs, endMs int64, fps int, outPath string) []string {
	return []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-ss", formatSeconds(startMs),
		"-t", formatSeconds(endMs - startMs),
		"-i", videoPath,
		"-an",
		"-vf", fmt.Sprintf("fps=%d,scale=%d:-2:flags=lanczos,setsar=1", fps, HighlightClipWidth),
		"-c:v", "libx264",
		"-preset", clipPreset,
		"-crf", clipCRF,
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		outPath,
	}
}

// GenerateHighlightMoment writes the frame at atMs to outPath as a JPEG.
func GenerateHighlightMoment(ctx context.Context, ffmpegBin, videoPath string, atMs int64, outPath string, timeout time.Duration) error {
	return runHighlight(ctx, ffmpegBin, MomentArgs(videoPath, atMs, outPath), outPath, timeout)
}

// GenerateHighlightClip writes [startMs, endMs) to outPath as a looping MP4.
func GenerateHighlightClip(ctx context.Context, ffmpegBin, videoPath string, startMs, endMs int64, fps int, outPath string, timeout time.Duration) error {
	return runHighlight(ctx, ffmpegBin, ClipArgs(videoPath, startMs, endMs, fps, outPath), outPath, timeout)
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
