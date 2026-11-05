package streaming

import (
	"testing"
)

func TestParseRange(t *testing.T) {
	cases := []struct {
		name  string
		spec  string
		size  int64
		ok    bool
		start int64
		end   int64
	}{
		{"empty", "", 100, false, 0, 0},
		{"no dash", "bytes=10", 100, false, 0, 0},
		{"multi range", "bytes=0-1,3-4", 100, false, 0, 0},
		{"zero size", "bytes=0-9", 0, false, 0, 0},
		{"simple", "bytes=0-9", 100, true, 0, 9},
		{"open ended", "bytes=90-", 100, true, 90, 99},
		{"suffix", "bytes=-10", 100, true, 90, 99},
		{"suffix larger than file", "bytes=-500", 100, true, 0, 99},
		{"start beyond size", "bytes=100-", 100, false, 0, 0},
		{"end before start", "bytes=10-5", 100, false, 0, 0},
		{"clamped end", "bytes=90-1000", 100, true, 90, 99},
		{"full file", "bytes=0-99", 100, true, 0, 99},
		{"not a number", "bytes=abc-5", 100, false, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end, ok := parseRange(c.spec, c.size)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if ok && (start != c.start || end != c.end) {
				t.Fatalf("got [%d,%d], want [%d,%d]", start, end, c.start, c.end)
			}
		})
	}
}

func TestContentTypeFor(t *testing.T) {
	cases := map[string]string{
		"movie.mp4":        "video/mp4",
		"movie.MKV":        "video/x-matroska", // case-insensitive
		"clip.webm":        "video/webm",
		"clip.mov":         "video/mp4",
		"clip.m4v":         "video/mp4",
		"clip.avi":         "video/x-msvideo",
		"clip.ogv":         "video/ogg",
		"clip.unknown_ext": "application/octet-stream",
	}
	for name, want := range cases {
		if got := ContentTypeFor(name); got != want {
			t.Errorf("ContentTypeFor(%q) = %q, want %q", name, got, want)
		}
	}
}
