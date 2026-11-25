package thumbnail

import (
	"strings"
	"testing"
)

// The ffmpeg command lines are the product here: the frame size, the frame rate
// and the difference-based filter chain are all in the arguments, so a test that
// only checked "some file appeared" would let a quality regression through.

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

func TestGIFArgsUseOneLineDifferenceFilter(t *testing.T) {
	args := GIFArgs("a.mp4", 90_000, 95_000, 10, "out.gif")

	ss := indexOf(args, "-ss")
	duration := indexOf(args, "-t")
	input := indexOf(args, "-i")
	if ss < 0 || duration < 0 || input < 0 || ss > input || duration > input {
		t.Fatalf("args = %v, want -ss and -t before -i", args)
	}
	if args[ss+1] != "90.000" || args[duration+1] != "5.000" {
		t.Errorf("start/duration = %q/%q, want 90.000/5.000", args[ss+1], args[duration+1])
	}

	filter := args[indexOf(args, "-vf")+1]
	if strings.Contains(filter, "\n") {
		t.Errorf("filter %q must be a single line", filter)
	}
	for _, want := range []string{
		"fps=10",
		"scale=480:-2:flags=lanczos",
		"palettegen=max_colors=128:stats_mode=diff",
		"paletteuse=dither=bayer:bayer_scale=3:diff_mode=rectangle",
		"split[s0][s1]",
	} {
		if !strings.Contains(filter, want) {
			t.Errorf("filter %q missing %q", filter, want)
		}
	}
	// A GIF with sound is a larger file for nothing; the loop flag is what makes
	// it play as a loop outside the app.
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-an") {
		t.Errorf("args = %v, want the audio track dropped", args)
	}
	if !strings.Contains(joined, "-loop 0") {
		t.Errorf("args = %v, want an infinite loop", args)
	}
	if !strings.Contains(joined, "-gifflags +transdiff") {
		t.Errorf("args = %v, want transparent frame differencing", args)
	}
}

func TestGIFFilterFollowsFrameRate(t *testing.T) {
	// The worker's fallback rate must reach the filter, or the retry would
	// rewrite the same file at the same speed it was rejected for.
	args := GIFArgs("a.mp4", 0, 3000, 8, "out.gif")
	if filter := args[indexOf(args, "-vf")+1]; !strings.Contains(filter, "fps=8") {
		t.Errorf("filter = %q, want fps=8", filter)
	}
}

func TestGenerateHighlightRequiresFFmpeg(t *testing.T) {
	if err := GenerateHighlightMoment(t.Context(), "", "a.mp4", 0, t.TempDir()+"/out.jpg", 0); err != ErrFFmpegMissing {
		t.Errorf("err = %v, want ErrFFmpegMissing", err)
	}
	if err := GenerateHighlightGIF(t.Context(), "", "a.mp4", 0, 1000, 10, t.TempDir()+"/out.gif", 0); err != ErrFFmpegMissing {
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
