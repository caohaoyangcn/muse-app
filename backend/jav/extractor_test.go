package jav

import (
	"reflect"
	"testing"
)

func TestExtractCodes(t *testing.T) {
	tests := []struct {
		name string
		file string
		want []string
	}{
		{"hyphen", "IPX-633 1080p.mp4", []string{"IPX-633"}},
		{"compact", "ipx633.mkv", []string{"IPX-633"}},
		{"three digit leading zeros", "stko-004.mp4", []string{"STKO-004"}},
		{"leading zeros", "sivr00412_1_8k.mp4", []string{"SIVR-412"}},
		{"part suffix", "dv-1622B.mp4", []string{"DV-1622"}},
		{"suffix", "ABC_12A-test.mp4", []string{"ABC-12"}},
		{"dedupe", "IPX-633_IPX633.mp4", []string{"IPX-633"}},
		{"multiple candidates", "hhd800.com@ABW-180.mp4", []string{"HHD-800", "ABW-180"}},
		{"fc2 ppv", "FC2-PPV-4970405.mp4", []string{"FC2-4970405"}},
		{"fc2 compact", "fc2ppv4970405.mkv", []string{"FC2-4970405"}},
		{"fc2 separators", "FC2_PPV_004970405.mp4", []string{"FC2-4970405"}},
		{"fc2 direct", "FC2-4970405 1080p.mp4", []string{"FC2-4970405"}},
		{"fc2 underscore part", "hhd800.com@FC2-PPV-3121790_3.mp4", []string{"FC2-3121790"}},
		{"fc2 hyphen part", "FC2-PPV-4894253-2.mp4", []string{"FC2-4894253"}},
		{"fc2 hd suffix", "fc2-573637_hd.mp4", []string{"FC2-573637"}},
		{"fc2 parenthesized", "(FC2)(677922)初撮り.mp4", []string{"FC2-677922"}},
		{"fc2 marker without number", "FC2-PPV-preview.mp4", []string{}},
		{"fc2 too long", "FC2-PPV-12345678.mp4", []string{}},

		{"none", "holiday-video-1080p.mp4", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractCodes(tt.file); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExtractCodes(%q) = %#v, want %#v", tt.file, got, tt.want)
			}
		})
	}
}

// The part marker tells episodes apart from duplicate copies, so it must only
// be read where the name really marks a part: never from a resolution or a
// trailing word.
func TestExtractNameCodesPartMarkers(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		wantCode string
		wantPart string
	}{
		{"hyphen part", "abc-123-1.mp4", "ABC-123", "1"},
		{"underscore part", "abc-123_2.mp4", "ABC-123", "2"},
		{"dot part", "abc-123.3.mkv", "ABC-123", "3"},
		{"space part", "ABC-123 4.mp4", "ABC-123", "4"},
		{"zero padded part", "abc-123_02.mp4", "ABC-123", "2"},
		{"part with suffix", "abc-123-2 1080p.mp4", "ABC-123", "2"},
		{"no part", "IPX-633 1080p.mp4", "IPX-633", ""},
		{"resolution is not a part", "abc-123 1080p.mp4", "ABC-123", ""},
		{"quality marker is not a part", "ABC-123-4K.mp4", "ABC-123", ""},
		{"audio layout is not a part", "abc-123 5.1ch.mkv", "ABC-123", ""},
		{"bare suffix letter is not a part", "dv-1622B.mp4", "DV-1622", ""},
		{"long trailing number is not a part", "abc-123-1234.mp4", "ABC-123", ""},
		{"fc2 hyphen part", "FC2-PPV-4894253-2.mp4", "FC2-4894253", "2"},
		{"fc2 underscore part", "FC2-4894253_3.mp4", "FC2-4894253", "3"},
		{"fc2 hd suffix is not a part", "fc2-573637_hd.mp4", "FC2-573637", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codes := ExtractNameCodes(tt.file)
			if len(codes) == 0 {
				t.Fatalf("ExtractNameCodes(%q) returned no candidates", tt.file)
			}
			if codes[0].Code != tt.wantCode || codes[0].Part != tt.wantPart {
				t.Fatalf("ExtractNameCodes(%q)[0] = %#v, want code %q part %q", tt.file, codes[0], tt.wantCode, tt.wantPart)
			}
		})
	}
}

func TestPartForFile(t *testing.T) {
	tests := []struct {
		name string
		file string
		code string
		want string
	}{
		{"part of the matched code", "abc-123-2.mp4", "ABC-123", "2"},
		{"code the name does not spell", "abc-123-2.mp4", "XYZ-999", ""},
		{"manual override has no part", "random name.mp4", "ABC-123", ""},
		{"second candidate keeps its own part", "hhd800.com@ABW-180_2.mp4", "ABW-180", "2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PartForFile(tt.file, tt.code); got != tt.want {
				t.Fatalf("PartForFile(%q, %q) = %q, want %q", tt.file, tt.code, got, tt.want)
			}
		})
	}
}
