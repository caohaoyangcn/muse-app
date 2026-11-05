package providers

import "testing"

func TestParseFC2DurationText(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int
	}{
		{name: "hours", text: "01:12:49", want: 72},
		{name: "minutes seconds", text: "12:49", want: 12},
		{name: "minutes label", text: "95 min", want: 95},
		{name: "seconds label", text: "4369 seconds", want: 72},
		{name: "bare minutes", text: "120", want: 120},
		{name: "bare seconds", text: "4369", want: 72},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFC2DurationText(tc.text)
			if got == nil || *got != tc.want {
				t.Fatalf("parseFC2DurationText(%q) = %#v, want %d", tc.text, got, tc.want)
			}
		})
	}
}

func TestParseFC2DurationValueObjects(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  int
	}{
		{name: "seconds object", value: map[string]any{"seconds": float64(4369)}, want: 72},
		{name: "minutes object", value: map[string]any{"minutes": float64(95)}, want: 95},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFC2DurationValue(tc.value)
			if got == nil || *got != tc.want {
				t.Fatalf("parseFC2DurationValue(%#v) = %#v, want %d", tc.value, got, tc.want)
			}
		})
	}
}
