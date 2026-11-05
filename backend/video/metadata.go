package video

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"muse-app/backend/platform"
)

// Metadata is what V1 extracts from the container (spec-v1.md §9).
type Metadata struct {
	DurationMs int64
	Width      int
	Height     int
	FrameRate  float64 // frames per second (from avg_frame_rate)
	Bitrate    int64   // overall bit rate in bits per second (from format.bit_rate)
	// VideoCodec / AudioCodec are the container's stream codec names (ffprobe
	// codec_name), e.g. "h264"/"hevc" and "aac"/"ac3". The LAN client uses them to
	// show file information; playback decoding belongs to the native client.
	// Empty means "not known yet" (audio is also empty for a file with no audio
	// track).
	VideoCodec string
	AudioCodec string
}

type ffprobeOutput struct {
	Format struct {
		Duration string `json:"duration"`
		Bitrate  string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		AvgFrameRate string `json:"avg_frame_rate"`
		// Attached pictures (embedded cover art) are reported as video streams but
		// are not the feature video; their codec must not become the video codec.
		Disposition struct {
			AttachedPic int `json:"attached_pic"`
		} `json:"disposition"`
	} `json:"streams"`
}

// Probe runs ffprobe and parses duration/width/height/frame rate/bit rate plus
// the video and audio codec names.
func Probe(ffprobeBin, videoPath string) (*Metadata, error) {
	return ProbeContext(context.Background(), ffprobeBin, videoPath)
}

// ProbeContext is Probe with a caller-owned context. The processor passes its
// shutdown context here so closing the window kills a probe that is waiting on
// a slow disk instead of keeping the process alive for its 30-second bound.
func ProbeContext(parent context.Context, ffprobeBin, videoPath string) (*Metadata, error) {
	return ProbeContextWithTimeout(parent, ffprobeBin, videoPath, 30*time.Second)
}

// ProbeContextWithTimeout is ProbeContext with an explicit bound. A remote
// source gets a longer one because ffprobe may read across the network.
func ProbeContextWithTimeout(parent context.Context, ffprobeBin, videoPath string, timeout time.Duration) (*Metadata, error) {
	if ffprobeBin == "" {
		return nil, errors.New("ffprobe is not available")
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := platform.CommandContext(ctx, ffprobeBin,
		"-v", "error", "-print_format", "json", "-show_format", "-show_streams", videoPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe: %w: %s", err, firstLine(stderr.String()))
	}

	var out ffprobeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	md := &Metadata{}
	if out.Format.Duration != "" {
		if secs, err := strconv.ParseFloat(out.Format.Duration, 64); err == nil && secs > 0 {
			md.DurationMs = int64(secs * 1000)
		}
	}
	if out.Format.Bitrate != "" {
		if b, err := strconv.ParseInt(strings.TrimSpace(out.Format.Bitrate), 10, 64); err == nil && b > 0 {
			md.Bitrate = b
		}
	}
	// Walk every stream: take geometry and frame rate from the first real video
	// stream and the codecs from the first video and first audio stream.
	for _, stream := range out.Streams {
		switch stream.CodecType {
		case "video":
			if stream.Disposition.AttachedPic != 0 {
				continue
			}
			if md.Width == 0 && stream.Width > 0 {
				md.Width, md.Height = stream.Width, stream.Height
			}
			if md.FrameRate == 0 {
				md.FrameRate = parseFrameRate(stream.AvgFrameRate)
			}
			if md.VideoCodec == "" {
				md.VideoCodec = stream.CodecName
			}
		case "audio":
			if md.AudioCodec == "" {
				md.AudioCodec = stream.CodecName
			}
		}
	}
	return md, nil
}

// parseFrameRate converts an ffprobe rational like "30000/1001" (or a plain
// "25.0") into a frames-per-second float. Returns 0 when it cannot be parsed.
func parseFrameRate(r string) float64 {
	if r == "" {
		return 0
	}
	var numerator float64
	denominator := 1.0
	if slash := strings.IndexByte(r, '/'); slash >= 0 {
		var err error
		numerator, err = strconv.ParseFloat(strings.TrimSpace(r[:slash]), 64)
		if err != nil {
			return 0
		}
		denominator, err = strconv.ParseFloat(strings.TrimSpace(r[slash+1:]), 64)
		if err != nil || denominator <= 0 {
			return 0
		}
	} else {
		n, err := strconv.ParseFloat(strings.TrimSpace(r), 64)
		if err != nil {
			return 0
		}
		numerator = n
	}
	if numerator <= 0 {
		return 0
	}
	return numerator / denominator
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
