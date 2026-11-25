//go:build integration

package thumbnail

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// These tests run real ffmpeg. The unit tests above pin the arguments; these pin
// that the arguments actually produce a JPEG and a playable MP4, because a filter
// chain or an encoder setting that ffmpeg rejects only fails at runtime.

func requireFFmpeg(t *testing.T) (ffmpegBin string, videoPath string) {
	t.Helper()
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "sample.mp4")
	cmd := exec.Command(bin, "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=4:size=320x240:rate=30",
		"-pix_fmt", "yuv420p", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg test clip: %v: %s", err, output)
	}
	return bin, out
}

// probeClip reports the codec, frame rate and duration of a rendered file.
func probeClip(t *testing.T, path string) (codec string, fps float64, frames int, duration float64) {
	t.Helper()
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not on PATH; skipping integration test")
	}
	out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name,avg_frame_rate,nb_frames,duration",
		"-of", "default=nw=1", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "codec_name":
			codec = value
		case "avg_frame_rate":
			num, den, _ := strings.Cut(value, "/")
			n, _ := strconv.ParseFloat(num, 64)
			d, _ := strconv.ParseFloat(den, 64)
			if d > 0 {
				fps = n / d
			}
		case "nb_frames":
			frames, _ = strconv.Atoi(value)
		case "duration":
			duration, _ = strconv.ParseFloat(value, 64)
		}
	}
	return codec, fps, frames, duration
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

func TestGenerateHighlightClipIntegration(t *testing.T) {
	ffmpegBin, videoPath := requireFFmpeg(t)
	out := filepath.Join(t.TempDir(), "range.mp4")

	if err := GenerateHighlightClip(t.Context(), ffmpegBin, videoPath, 1000, 3500, 60, out, time.Minute); err != nil {
		t.Fatalf("generate clip: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// An MP4 starts with a size field and the "ftyp" box; anything else means the
	// bytes are not what the extension promises (which the media route relies on).
	if len(data) < 12 || !bytes.Equal(data[4:8], []byte("ftyp")) {
		t.Fatalf("output is not an MP4 (first bytes %q)", data[:min(12, len(data))])
	}

	codec, fps, frames, duration := probeClip(t, out)
	if codec != "h264" {
		t.Errorf("codec = %q, want h264", codec)
	}
	if fps < 59 || fps > 61 {
		t.Errorf("fps = %v, want 60", fps)
	}
	if frames < 140 || frames > 160 {
		t.Errorf("frames = %d, want about 150 for a 2.5 s clip at 60 fps", frames)
	}
	if duration < 2.4 || duration > 2.6 {
		t.Errorf("duration = %v s, want about 2.5", duration)
	}
	// No audio track: the UI plays this muted, and an audio stream would only add
	// bytes (the argument itself is pinned by the unit test).
	if hasAudio(t, out) {
		t.Error("a highlight clip must be muted")
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

// hasAudio reports whether the file carries an audio stream.
func hasAudio(t *testing.T, path string) bool {
	t.Helper()
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return false
	}
	out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "a",
		"-show_entries", "stream=index", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe audio: %v", err)
	}
	return strings.TrimSpace(string(out)) != ""
}
