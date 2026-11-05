//go:build integration

package video

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"muse-app/backend/thumbnail"
)

func probeBinary(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not on PATH; skipping integration test")
	}
	return bin
}

// makeTestVideo generates a tiny 2-second 320x240 test clip with ffmpeg.
func makeTestVideo(t *testing.T, ffmpegBin, dir string) string {
	t.Helper()
	out := filepath.Join(dir, "sample.mp4")
	cmd := exec.Command(ffmpegBin, "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=10",
		"-pix_fmt", "yuv420p", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg test clip: %v: %s", err, output)
	}
	return out
}

// makeAVIVideo generates a clip in a container browsers cannot play (Xvid/AVI),
// used to prove the codec probe reports the real codec rather than guessing from
// the extension.
func makeAVIVideo(t *testing.T, ffmpegBin, dir string) string {
	t.Helper()
	out := filepath.Join(dir, "sample.avi")
	cmd := exec.Command(ffmpegBin, "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=10",
		"-c:v", "mpeg4", "-vtag", "xvid", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg avi clip: %v: %s", err, output)
	}
	return out
}

func TestProbeIntegration(t *testing.T) {
	ffprobeBin := probeBinary(t)
	ffmpegBin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}

	md, err := Probe(ffprobeBin, makeTestVideo(t, ffmpegBin, t.TempDir()))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if md.DurationMs < 1800 || md.DurationMs > 2200 {
		t.Errorf("duration = %d ms, want ~2000", md.DurationMs)
	}
	if md.Width != 320 || md.Height != 240 {
		t.Errorf("resolution = %dx%d, want 320x240", md.Width, md.Height)
	}
	if md.VideoCodec != "h264" {
		t.Errorf("video codec = %q, want h264", md.VideoCodec)
	}
}

// TestProbeCodecIntegration is the point of the codec columns: a file's real
// codec must be reported even when the extension would suggest something else,
// because clients use the codec for display and choose their own decoder.
func TestProbeCodecIntegration(t *testing.T) {
	ffprobeBin := probeBinary(t)
	ffmpegBin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}

	md, err := Probe(ffprobeBin, makeAVIVideo(t, ffmpegBin, t.TempDir()))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if md.VideoCodec != "mpeg4" {
		t.Errorf("video codec = %q, want mpeg4 (AVI/Xvid)", md.VideoCodec)
	}
}

func TestProbeBadFile(t *testing.T) {
	ffprobeBin := probeBinary(t)
	p := filepath.Join(t.TempDir(), "garbage.mp4")
	if err := os.WriteFile(p, []byte("not a video at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Probe(ffprobeBin, p); err == nil {
		t.Error("probe of garbage file should fail")
	}
}

func TestThumbnailGenerateIntegration(t *testing.T) {
	ffprobeBin := probeBinary(t)
	ffmpegBin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}

	dir := t.TempDir()
	videoPath := makeTestVideo(t, ffmpegBin, dir)
	md, err := Probe(ffprobeBin, videoPath)
	if err != nil {
		t.Fatal(err)
	}

	thumbDir := filepath.Join(dir, "thumbnails")
	rel, err := thumbnail.Generate(ffmpegBin, videoPath, md.DurationMs, thumbDir, 42)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if rel != "thumbnails/42.jpg" {
		t.Errorf("rel = %q", rel)
	}
	info, err := os.Stat(filepath.Join(thumbDir, "42.jpg"))
	if err != nil {
		t.Fatalf("thumbnail file missing: %v", err)
	}
	if info.Size() == 0 {
		t.Error("thumbnail is empty")
	}
}

func TestThumbnailBadVideo(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "garbage.mp4")
	if err := os.WriteFile(p, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := thumbnail.Generate("ffmpeg", p, 2000, filepath.Join(dir, "thumbs"), 7); err == nil {
		t.Error("thumbnail of garbage file should fail")
	}
}
