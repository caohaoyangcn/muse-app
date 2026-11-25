//go:build integration

package video

import (
	"bytes"
	"encoding/csv"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The unit tests pin the frame-rate constants; this one proves the real ffmpeg
// honours them. A GIF's delay is stored in hundredths of a second, so "50 fps"
// is only true if every frame lands on exactly 2 cs — and 2 cs is the shortest
// delay browsers play instead of clamping to 100 ms.

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

// gifFrameDelays returns one frame's delay in seconds, deduplicated.
func gifFrameDelays(t *testing.T, ffprobeBin, path string) (delays []float64, frames int) {
	t.Helper()
	out, err := exec.Command(ffprobeBin, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "frame=duration_time", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe frames: %v", err)
	}
	seen := map[float64]struct{}{}
	reader := csv.NewReader(bytes.NewReader(out))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse ffprobe output: %v", err)
	}
	for _, record := range records {
		if len(record) == 0 || strings.TrimSpace(record[0]) == "" {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(record[0]), 64)
		if err != nil {
			continue
		}
		frames++
		seen[value] = struct{}{}
	}
	for value := range seen {
		delays = append(delays, value)
	}
	return delays, frames
}

func TestServiceGeneratesGIFAtTheDefaultFrameRate(t *testing.T) {
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

	path := svc.highlightPath(ready)
	delays, frames := gifFrameDelays(t, ffprobeBin, path)
	if len(delays) != 1 {
		t.Fatalf("frame delays = %v, want one constant delay at the default rate", delays)
	}
	if diff := delays[0] - 0.02; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("delay = %v s, want 0.02 s (50 fps is the shortest delay browsers honour)", delays[0])
	}
	// Two seconds at 50 fps. The seek and the exact cut leave a frame or two of
	// slack; what must not happen is 10 fps (20 frames) or a stalled encode.
	if frames < 90 || frames > 110 {
		t.Errorf("frames = %d, want about 100 for a 2 s range at 50 fps", frames)
	}
}
