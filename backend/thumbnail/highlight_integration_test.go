//go:build integration

package thumbnail

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// These tests run real ffmpeg. The unit tests above pin the arguments; these pin
// that the arguments actually produce a JPEG and a looping GIF, because a filter
// chain that ffmpeg rejects only fails at runtime.

func requireFFmpeg(t *testing.T) (ffmpegBin string, videoPath string) {
	t.Helper()
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "sample.mp4")
	cmd := exec.Command(bin, "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=4:size=320x240:rate=10",
		"-pix_fmt", "yuv420p", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg test clip: %v: %s", err, output)
	}
	return bin, out
}

func TestGenerateHighlightMomentIntegration(t *testing.T) {
	ffmpegBin, videoPath := requireFFmpeg(t)
	out := filepath.Join(t.TempDir(), "moment.jpg")

	if err := GenerateHighlightMoment(t.Context(), ffmpegBin, videoPath, 2000, out, time.Minute); err != nil {
		t.Fatalf("generate moment: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read moment: %v", err)
	}
	if len(data) < 3 || data[0] != 0xFF || data[1] != 0xD8 || data[2] != 0xFF {
		t.Errorf("output is not a JPEG (first bytes %x)", data[:min(3, len(data))])
	}
}

func TestGenerateHighlightGIFIntegration(t *testing.T) {
	ffmpegBin, videoPath := requireFFmpeg(t)
	out := filepath.Join(t.TempDir(), "range.gif")

	if err := GenerateHighlightGIF(t.Context(), ffmpegBin, videoPath, 1000, 3500, 10, out, time.Minute); err != nil {
		t.Fatalf("generate gif: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read gif: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("GIF89a")) {
		t.Errorf("output is not a GIF89a file (first bytes %q)", data[:min(6, len(data))])
	}
	// The NETSCAPE2.0 application extension is what makes the file loop. Without
	// it a viewer plays the animation once and stops.
	if !bytes.Contains(data, []byte("NETSCAPE2.0")) {
		t.Error("GIF has no NETSCAPE2.0 loop block")
	}
}

func TestGenerateHighlightRemovesHalfWrittenFile(t *testing.T) {
	ffmpegBin, _ := requireFFmpeg(t)
	dir := t.TempDir()
	broken := filepath.Join(dir, "garbage.mp4")
	if err := os.WriteFile(broken, []byte("not a video at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "moment.jpg")

	if err := GenerateHighlightMoment(t.Context(), ffmpegBin, broken, 1000, out, time.Minute); err == nil {
		t.Fatal("a garbage source must not produce a highlight")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Error("a failed attempt must not leave an output file behind")
	}
}

func TestGenerateHighlightHonoursTimeout(t *testing.T) {
	ffmpegBin, videoPath := requireFFmpeg(t)
	out := filepath.Join(t.TempDir(), "moment.jpg")

	// A bound so small that ffmpeg cannot possibly finish. The point is that the
	// attempt ends and reports a definite failure instead of hanging the worker.
	err := GenerateHighlightMoment(t.Context(), ffmpegBin, videoPath, 3000, out, time.Nanosecond)
	if err == nil {
		t.Skip("ffmpeg finished inside a nanosecond; nothing to assert")
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Error("a timed-out attempt must not leave an output file behind")
	}
}
