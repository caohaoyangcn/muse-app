package windowstate

import "testing"

// TestPlacementValid verifies the guard used before saving or restoring a
// placement: only rectangles that could come from a real display pass.
func TestPlacementValid(t *testing.T) {
	tests := []struct {
		name string
		p    Placement
		want bool
	}{
		{name: "typical", p: Placement{Left: 120, Top: 80, Width: 1100, Height: 720}, want: true},
		{name: "negative origin", p: Placement{Left: -1800, Top: -200, Width: 800, Height: 600}, want: true},
		{name: "zero width", p: Placement{Width: 0, Height: 720}, want: false},
		{name: "negative height", p: Placement{Width: 800, Height: -1}, want: false},
		{name: "absurd size", p: Placement{Width: maxPlacementCoordinate + 1, Height: 720}, want: false},
		{name: "absurd origin", p: Placement{Left: maxPlacementCoordinate, Width: 800, Height: 600}, want: false},
		{name: "absurd negative origin", p: Placement{Left: -maxPlacementCoordinate, Width: 800, Height: 600}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Valid(); got != tt.want {
				t.Errorf("Valid(%+v) = %v, want %v", tt.p, got, tt.want)
			}
		})
	}
}

// TestPlacementFitWithin verifies a saved placement is kept on the current
// screen: a disconnected monitor moves the window onto the nearest one, an
// oversized window shrinks to the work area, and the minimum size still wins
// when the display is smaller than it.
func TestPlacementFitWithin(t *testing.T) {
	primary := workArea{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
	leftMonitor := workArea{Left: -1920, Top: 0, Right: 0, Bottom: 1040}

	tests := []struct {
		name string
		p    Placement
		work workArea
		minW int
		minH int
		want Placement
	}{
		{
			name: "already inside",
			p:    Placement{Left: 100, Top: 100, Width: 1100, Height: 720},
			work: primary, minW: 960, minH: 600,
			want: Placement{Left: 100, Top: 100, Width: 1100, Height: 720},
		},
		{
			name: "same layout keeps maximised flag",
			p:    Placement{Left: 100, Top: 100, Width: 1100, Height: 720, Maximised: true},
			work: primary, minW: 960, minH: 600,
			want: Placement{Left: 100, Top: 100, Width: 1100, Height: 720, Maximised: true},
		},
		{
			name: "off-screen right is pulled back",
			p:    Placement{Left: 3000, Top: 100, Width: 800, Height: 600},
			work: primary, minW: 960, minH: 600,
			want: Placement{Left: 960, Top: 100, Width: 960, Height: 600},
		},
		{
			name: "negative monitor is accepted",
			p:    Placement{Left: -1800, Top: 50, Width: 800, Height: 600},
			work: leftMonitor, minW: 960, minH: 600,
			want: Placement{Left: -1800, Top: 50, Width: 960, Height: 600},
		},
		{
			name: "too tall and wide shrinks to work area",
			p:    Placement{Left: 0, Top: 0, Width: 4000, Height: 3000},
			work: primary, minW: 960, minH: 600,
			want: Placement{Left: 0, Top: 0, Width: 1920, Height: 1040},
		},
		{
			name: "below the bottom edge moves up",
			p:    Placement{Left: 100, Top: 2000, Width: 1100, Height: 720},
			work: primary, minW: 960, minH: 600,
			want: Placement{Left: 100, Top: 320, Width: 1100, Height: 720},
		},
		{
			name: "small window grows to minimum",
			p:    Placement{Left: 100, Top: 100, Width: 400, Height: 300},
			work: primary, minW: 960, minH: 600,
			want: Placement{Left: 100, Top: 100, Width: 960, Height: 600},
		},
		{
			name: "minimum larger than display loses to display",
			p:    Placement{Left: 0, Top: 0, Width: 400, Height: 300},
			work: workArea{Left: 0, Top: 0, Right: 800, Bottom: 500}, minW: 960, minH: 600,
			want: Placement{Left: 0, Top: 0, Width: 800, Height: 500},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.fitWithin(tt.work, tt.minW, tt.minH)
			if got != tt.want {
				t.Errorf("fitWithin(%+v, %+v) = %+v, want %+v", tt.p, tt.work, got, tt.want)
			}
		})
	}
}
