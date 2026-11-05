//go:build darwin

package windowstate

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// NewSampler returns a Sampler backed by Wails' window runtime on macOS.
func NewSampler(ctx context.Context, _, _ int) Sampler {
	return &sampler{ctx: ctx}
}

type sampler struct {
	ctx context.Context

	// Wails exposes no normal-frame query while the window is zoomed, so the
	// sampler remembers the last un-zoomed frame it observed. Without it, a
	// launch that closed maximised would restore the zoomed size as the normal
	// size and the window could never return to its previous bounds.
	normal    Placement
	hasNormal bool
}

func (s *sampler) Current() (Placement, bool) {
	x, y := runtime.WindowGetPosition(s.ctx)
	width, height := runtime.WindowGetSize(s.ctx)
	if width <= 0 || height <= 0 {
		return Placement{}, false
	}
	if !runtime.WindowIsMaximised(s.ctx) {
		s.normal = Placement{Left: x, Top: y, Width: width, Height: height}
		s.hasNormal = true
		return s.normal, true
	}
	if !s.hasNormal {
		// The saved normal frame is still the right answer; reporting the
		// zoomed bounds here would overwrite it on the first poll after a
		// maximised launch.
		return Placement{}, false
	}
	zoomed := s.normal
	zoomed.Maximised = true
	return zoomed, true
}

// Apply restores the normal frame. The maximised state is left to
// options.WindowStartState, which Wails applies when it shows the window.
func (s *sampler) Apply(p Placement) error {
	if !p.Valid() {
		return errors.New("invalid window placement")
	}
	runtime.WindowSetSize(s.ctx, p.Width, p.Height)
	runtime.WindowSetPosition(s.ctx, p.Left, p.Top)
	return nil
}
