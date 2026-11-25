package thumbnail

import (
	"strings"
	"testing"
)

// The ffmpeg command lines are the product here: the frame size, the frame rate,
// the codec settings and the audio drop are all in the arguments, so a test that
// only checked "some file appeared" would let a quality or size regression through.

func TestMomentArgsSeekBeforeInput(t *testing.T) {
	args := MomentArgs(`D:\v\a.mp4`, 90_000, `D:\data\highlights\h-1-2-3.jpg`)
	joined := strings.Join(args, " ")

	// A fast seek only works before -i; after it, ffmpeg decodes from the start.
	ss := indexOf(args, "-ss")
	input := indexOf(args, "-i")
	if ss < 0 || input < 0 || ss > input {
		t.Fatalf("args = %v, want -ss before -i", args)
	}
	if args[ss+1] != "90.000" {
		t.Errorf("-ss = %q, want the start in seconds", args[ss+1])
	}
	if !strings.Contains(joined, "-frames:v 1") {
		t.Errorf("args = %v, want a single frame", args)
	}
	if !strings.Contains(joined, "scale=1280:-2") {
		t.Errorf("args = %v, want the 1280 wide frame", args)
	}
	if !strings.Contains(joined, "-q:v 3") {
		t.Errorf("args = %v, want the JPEG quality the spec fixed", args)
	}
}

func TestMomentArgsSkipZeroSeek(t *testing.T) {
	// A moment at 0 should take ffmpeg's default first frame rather than a
	// seek to 0.
	if indexOf(MomentArgs("a.mp4", 0, "out.jpg"), "-ss") >= 0 {
		t.Error("a zero moment must not seek")
	}
}

func TestClipArgsProduceALoopableMutedMP4(t *testing.T) {
	args := ClipArgs("a.mp4", 90_000, 105_000, 60, "out.mp4")

	ss := indexOf(args, "-ss")
	duration := indexOf(args, "-t")
	input := indexOf(args, "-i")
	if ss < 0 || duration < 0 || input < 0 || ss > input || duration > input {
		t.Fatalf("args = %v, want -ss and -t before -i", args)
	}
	if args[ss+1] != "90.000" || args[duration+1] != "15.000" {
		t.Errorf("start/duration = %q/%q, want 90.000/15.000", args[ss+1], args[duration+1])
	}
	if indexOf(args, "-an") < 0 {
		t.Errorf("args = %v, want the audio track dropped", args)
	}

	filter := args[indexOf(args, "-vf")+1]
	for _, want := range []string{
		"fps=60",
		// The width is capped by the source so a small favorite is not upscaled
		// into a blurrier, bigger file.
		"scale='min(960,iw)':-2:flags=lanczos",
		"setsar=1",
	} {
		if !strings.Contains(filter, want) {
			t.Errorf("filter = %q missing %q", filter, want)
		}
	}

	// The codec settings are what make the file small, seekable and playable in
	// any browser; none of them is optional.
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-c:v libx264",
		"-crf 22",
		"-pix_fmt yuv420p",
		"-movflags +faststart",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args = %v missing %q", args, want)
		}
	}
}

func TestClipFilterFollowsFrameRate(t *testing.T) {
	args := ClipArgs("a.mp4", 0, 3000, 30, "out.mp4")
	if filter := args[indexOf(args, "-vf")+1]; !strings.Contains(filter, "fps=30") {
		t.Errorf("filter = %q, want fps=30", filter)
	}
}

func TestGenerateHighlightRequiresFFmpeg(t *testing.T) {
	if err := GenerateHighlightMoment(t.Context(), "", "a.mp4", 0, t.TempDir()+"/out.jpg", 0); err != ErrFFmpegMissing {
		t.Errorf("err = %v, want ErrFFmpegMissing", err)
	}
	if err := GenerateHighlightClip(t.Context(), "", "a.mp4", 0, 1000, 60, t.TempDir()+"/out.mp4", 0); err != ErrFFmpegMissing {
		t.Errorf("err = %v, want ErrFFmpegMissing", err)
	}
}

// indexOf returns the position of the first flag, or -1.
func indexOf(args []string, flag string) int {
	for i, arg := range args {
		if arg == flag {
			return i
		}
	}
	return -1
}
