//go:build integration

package video

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The unit tests pin the frame rate and the command line; this one proves the real
// ffmpeg and the service end up with a playable file. "Playable" is the part that
// matters: the media routes decide the Content-Type from the extension, so an
// .mp4 that is not an MP4 would be served as video and fail in the browser.

func requireFFmpegTools(t *testing.T) (ffmpegBin, ffprobeBin string) {
	t.Helper()
	ffmpegBin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	ffprobeBin, err = exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not on PATH; skipping integration test")
	}
	return ffmpegBin, ffprobeBin
}

// makeHighlightClip writes a short clip with real motion for the generator.
func makeHighlightClip(t *testing.T, ffmpegBin, dir string) string {
	t.Helper()
	out := filepath.Join(dir, "clip.mp4")
	cmd := exec.Command(ffmpegBin, "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=duration=4:size=320x240:rate=30",
		"-pix_fmt", "yuv420p", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg test clip: %v: %s", err, output)
	}
	return out
}

// clipStream is what ffprobe reports about the rendered file's video stream.
type clipStream struct {
	codec    string
	fps      float64
	frames   int
	duration float64
}

func probeClipStream(t *testing.T, ffprobeBin, path string) clipStream {
	t.Helper()
	out, err := exec.Command(ffprobeBin, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name,avg_frame_rate,nb_frames,duration",
		"-of", "default=nw=1", path).Output()
	if err != nil {
		t.Fatalf("ffprobe clip: %v", err)
	}
	var got clipStream
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "codec_name":
			got.codec = value
		case "avg_frame_rate":
			num, den, _ := strings.Cut(value, "/")
			n, _ := strconv.ParseFloat(num, 64)
			d, _ := strconv.ParseFloat(den, 64)
			if d > 0 {
				got.fps = n / d
			}
		case "nb_frames":
			got.frames, _ = strconv.Atoi(value)
		case "duration":
			got.duration, _ = strconv.ParseFloat(value, 64)
		}
	}
	return got
}

func TestServiceRendersAPlayableClip(t *testing.T) {
	ffmpegBin, ffprobeBin := requireFFmpegTools(t)
	svc, dir := newTestService(t)
	t.Cleanup(svc.CloseHighlights)
	// The test fixture builds its processor without tools; point it at the real
	// one so the generator seam runs exactly the command production runs.
	svc.processor.ffmpegBin = ffmpegBin

	clip := makeHighlightClip(t, ffmpegBin, dir)
	v, err := svc.AddVideo(clip)
	if err != nil {
		t.Fatalf("add video: %v", err)
	}
	if err := svc.repo.UpdateMetadata(v.ID, 4000, 320, 240, 30, 500_000, "h264", ""); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	h, err := svc.AddHighlight(v.ID, 1000, 3000) // a two-second range
	if err != nil {
		t.Fatalf("add highlight: %v", err)
	}
	svc.StartHighlights()
	ready := waitForHighlight(t, svc, v.ID, h.ID, HighlightStatusReady)

	if !strings.HasSuffix(ready.MediaPath, ".mp4") {
		t.Fatalf("media path = %q, want .mp4", ready.MediaPath)
	}
	path := svc.highlightPath(ready)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read clip: %v", err)
	}
	// A size field followed by the "ftyp" box is what every MP4 starts with.
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		t.Fatalf("output is not an MP4 (first bytes %q)", data[:min(12, len(data))])
	}

	got := probeClipStream(t, ffprobeBin, path)
	if got.codec != "h264" {
		t.Errorf("codec = %q, want h264", got.codec)
	}
	if got.fps < 59 || got.fps > 61 {
		t.Errorf("fps = %v, want %d", got.fps, highlightClipFPS)
	}
	// Two seconds at the clip rate. The seek and the exact cut leave a frame or
	// two of slack; what must not happen is a stalled or truncated encode.
	if got.frames < 110 || got.frames > 130 {
		t.Errorf("frames = %d, want about %d for a 2 s range", got.frames, 2*highlightClipFPS)
	}
	if got.duration < 1.9 || got.duration > 2.1 {
		t.Errorf("duration = %v s, want about 2", got.duration)
	}
}
