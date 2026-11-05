// Package windowstate remembers where the desktop window was when Muse was
// last closed, so the next launch opens at the same size and position instead
// of the centered default every time.
//
// The Sampler is the platform seam: Windows reads and restores the real window
// placement with the Win32 placement API, which keeps maximised windows and
// multi-monitor layouts intact. The Tracker is platform independent and only
// decides when a new placement is worth persisting.
package windowstate

// Placement is the main window's normal (un-maximised) rectangle in
// virtual-screen pixels, plus whether the window was maximised when it was last
// seen. Saving the normal rectangle rather than the live one is what lets a
// maximised window restore to the right size once the user un-maximises it.
type Placement struct {
	Left      int  `json:"left"`
	Top       int  `json:"top"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

// maxPlacementCoordinate bounds values read back from the database. The virtual
// desktop is limited to 32767×32767 pixels by Windows, so anything outside that
// range is corruption rather than a window somebody had.
const maxPlacementCoordinate = 32767

// Valid reports whether p describes a restorable window. It rejects values that
// cannot come from a real display so a corrupt settings row never moves the
// window to (0, 0) with a pixel size.
func (p Placement) Valid() bool {
	return p.Width > 0 && p.Width <= maxPlacementCoordinate &&
		p.Height > 0 && p.Height <= maxPlacementCoordinate &&
		p.Left > -maxPlacementCoordinate && p.Left < maxPlacementCoordinate &&
		p.Top > -maxPlacementCoordinate && p.Top < maxPlacementCoordinate
}

// workArea is a monitor work area with exclusive right and bottom edges.
type workArea struct {
	Left   int
	Top    int
	Right  int
	Bottom int
}

// fitWithin keeps p inside work, enforcing the window's minimum size first and
// the work area second. A saved placement from a disconnected monitor therefore
// lands on the nearest display instead of off-screen, and a window that was
// wider than the current display is shrunk to fit rather than half-hidden.
func (p Placement) fitWithin(work workArea, minWidth, minHeight int) Placement {
	width, height := p.Width, p.Height
	if minWidth > 0 && width < minWidth {
		width = minWidth
	}
	if minHeight > 0 && height < minHeight {
		height = minHeight
	}
	if available := work.Right - work.Left; available > 0 && width > available {
		width = available
	}
	if available := work.Bottom - work.Top; available > 0 && height > available {
		height = available
	}

	left, top := p.Left, p.Top
	if left < work.Left {
		left = work.Left
	}
	if maxLeft := work.Right - width; left > maxLeft {
		left = maxLeft
	}
	if top < work.Top {
		top = work.Top
	}
	if maxTop := work.Bottom - height; top > maxTop {
		top = maxTop
	}

	return Placement{
		Left:      left,
		Top:       top,
		Width:     width,
		Height:    height,
		Maximised: p.Maximised,
	}
}
