// Package thumbnail extracts a single preview frame from a video with ffmpeg.
package thumbnail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"muse-app/backend/platform"
)

// RelativeDir is the thumbnail folder name inside the app data dir; the
// database stores paths relative to it (spec-v1.md §11).
const RelativeDir = "thumbnails"

// ffmpegTimeout bounds a single thumbnail extraction. A file that makes ffmpeg
// hang (network drive, damaged stream) must release the worker; otherwise every
// worker in the bounded pool stalls on that job and later thumbnails never
// generate — the "封面加载不出来" symptom after a large batch import.
const ffmpegTimeout = 30 * time.Second

// Generate writes thumbnails/<id>.jpg for videoPath and returns the relative
// path stored in the database. The frame is taken at 10% of the duration
// because the first frames are frequently black (spec-v1.md §10).
func Generate(ffmpegBin, videoPath string, durationMs int64, thumbDir string, id int64) (string, error) {
	return GenerateContext(context.Background(), ffmpegBin, videoPath, durationMs, thumbDir, id)
}

// GenerateContext is Generate with a caller-owned context. The processor passes
// its shutdown context here so closing the window kills an ffmpeg extraction
// instead of waiting up to ffmpegTimeout for it.
func GenerateContext(parent context.Context, ffmpegBin, videoPath string, durationMs int64, thumbDir string, id int64) (string, error) {
	return GenerateContextWithTimeout(parent, ffmpegBin, videoPath, durationMs, thumbDir, id, ffmpegTimeout)
}

// GenerateContextWithTimeout is GenerateContext with an explicit bound. The
// processor uses a longer bound for a WebDAV/HTTP source, where the first byte
// may wait on the remote drive, but shutdown still cancels it.
func GenerateContextWithTimeout(parent context.Context, ffmpegBin, videoPath string, durationMs int64, thumbDir string, id int64, timeout time.Duration) (string, error) {
	var at float64
	if durationMs > 0 {
		at = float64(durationMs) / 10 / 1000.0
	}
	return ExtractAtContextWithTimeout(parent, ffmpegBin, videoPath, at, thumbDir, id, timeout)
}

// ExtractAt writes thumbnails/<id>.jpg from the frame at atSec and returns the
// relative path stored in the database. atSec <= 0 extracts at ffmpeg's default
// position (used when the duration is unknown). The cover picker calls this
// with the timestamp the user scrubbed to.
func ExtractAt(ffmpegBin, videoPath string, atSec float64, thumbDir string, id int64) (string, error) {
	return ExtractAtContext(context.Background(), ffmpegBin, videoPath, atSec, thumbDir, id)
}

// ExtractAtContext is ExtractAt with a caller-owned context, so an extraction
// can be canceled by application shutdown as well as by its own timeout.
func ExtractAtContext(parent context.Context, ffmpegBin, videoPath string, atSec float64, thumbDir string, id int64) (string, error) {
	return ExtractAtContextWithTimeout(parent, ffmpegBin, videoPath, atSec, thumbDir, id, ffmpegTimeout)
}

// ExtractAtContextWithTimeout is ExtractAtContext with an explicit bound. It is
// the remote-source path: an HTTP range read may take longer than a local
// extraction, and a 30-second cutoff would leave every remote cover empty.
func ExtractAtContextWithTimeout(parent context.Context, ffmpegBin, videoPath string, atSec float64, thumbDir string, id int64, timeout time.Duration) (string, error) {
	if ffmpegBin == "" {
		return "", errors.New("ffmpeg is not available")
	}
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		return "", fmt.Errorf("create thumbnail dir: %w", err)
	}

	fileName := strconv.FormatInt(id, 10) + ".jpg"
	outPath := filepath.Join(thumbDir, fileName)
	args := []string{"-y", "-hide_banner", "-loglevel", "error"}
	if atSec > 0 {
		// "-ss" before "-i" performs a fast seek to the requested position.
		args = append(args, "-ss", strconv.FormatFloat(atSec, 'f', 3, 64))
	}
	args = append(args,
		"-i", videoPath,
		"-frames:v", "1",
		"-vf", "scale=640:-2",
		"-q:v", "3",
		outPath,
	)

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := platform.CommandContext(ctx, ffmpegBin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outPath) // best-effort cleanup of a partial file
		return "", fmt.Errorf("ffmpeg: %w: %s", err, firstLine(stderr.String()))
	}
	return RelativeDir + "/" + fileName, nil
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
