//go:build windows

package windowstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

// wailsWindowClass is the window class Wails registers for the application
// window. Muse does not override windows.Options.WindowClassName, so this is
// stable for the lifetime of the app.
const wailsWindowClass = "wailsWindow"

const (
	swShowMaximized         = 3
	monitorDefaultToNearest = 2
	gwHwndNext              = 2
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")

	procGetTopWindow       = user32.NewProc("GetTopWindow")
	procGetWindow          = user32.NewProc("GetWindow")
	procGetWindowThreadPID = user32.NewProc("GetWindowThreadProcessId")
	procGetClassNameW      = user32.NewProc("GetClassNameW")
	procIsWindow           = user32.NewProc("IsWindow")
	procGetWindowPlacement = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement = user32.NewProc("SetWindowPlacement")
	procMonitorFromRect    = user32.NewProc("MonitorFromRect")
	procGetMonitorInfoW    = user32.NewProc("GetMonitorInfoW")
)

type winPoint struct {
	X int32
	Y int32
}

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

// winWindowPlacement mirrors WINDOWPLACEMENT. Its RcNormalPosition is the
// rectangle WindowStartState restoration needs: while the window is maximised
// the live rectangle is the whole screen, but the normal rectangle still holds
// where the window would go when un-maximised.
type winWindowPlacement struct {
	Length           uint32
	Flags            uint32
	ShowCmd          uint32
	PtMinPosition    winPoint
	PtMaxPosition    winPoint
	RcNormalPosition winRect
}

type winMonitorInfo struct {
	CbSize    uint32
	RcMonitor winRect
	RcWork    winRect
	DwFlags   uint32
}

// NewSampler returns a Sampler for this process's main Wails window. The context
// and minimums are part of the shared constructor shape; on Windows the window
// is located through the Win32 API instead.
func NewSampler(_ context.Context, minWidth, minHeight int) Sampler {
	return &sampler{minWidth: minWidth, minHeight: minHeight}
}

type sampler struct {
	minWidth  int
	minHeight int
}

// Current reads the window's normal placement and maximised flag. It returns
// false before the window exists and while it is being destroyed, so the tracker
// simply skips those samples.
func (s *sampler) Current() (Placement, bool) {
	hwnd := mainWindowHandle()
	if hwnd == 0 {
		return Placement{}, false
	}
	placement, err := windowPlacement(hwnd)
	if err != nil {
		return Placement{}, false
	}
	normal := placement.RcNormalPosition
	return Placement{
		Left:      int(normal.Left),
		Top:       int(normal.Top),
		Width:     int(normal.Right - normal.Left),
		Height:    int(normal.Bottom - normal.Top),
		Maximised: placement.ShowCmd == swShowMaximized,
	}, true
}

// Apply restores p as the window's normal placement, moving it onto the nearest
// monitor when the saved display is gone and shrinking it to that monitor's work
// area when needed. It must run before Wails first shows the window, and it
// leaves the show command alone: Wails itself shows the window and applies
// options.WindowStartState (which carries the maximised flag).
func (s *sampler) Apply(p Placement) error {
	if !p.Valid() {
		return errors.New("invalid window placement")
	}
	hwnd := mainWindowHandle()
	if hwnd == 0 {
		return errors.New("main window not found")
	}
	fitted := s.fitToWorkArea(p)

	placement, err := windowPlacement(hwnd)
	if err != nil {
		return err
	}
	placement.RcNormalPosition = winRect{
		Left:   int32(fitted.Left),
		Top:    int32(fitted.Top),
		Right:  int32(fitted.Left + fitted.Width),
		Bottom: int32(fitted.Top + fitted.Height),
	}
	if ok, _, callErr := procSetWindowPlacement.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&placement))); ok == 0 {
		return fmt.Errorf("set window placement: %v", callErr)
	}
	return nil
}

func windowPlacement(hwnd syscall.Handle) (winWindowPlacement, error) {
	var placement winWindowPlacement
	placement.Length = uint32(unsafe.Sizeof(placement))
	if ok, _, callErr := procGetWindowPlacement.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&placement))); ok == 0 {
		return winWindowPlacement{}, fmt.Errorf("get window placement: %v", callErr)
	}
	return placement, nil
}

// fitToWorkArea resolves the monitor the saved rectangle belongs to (the nearest
// one when that display is gone) and keeps the window inside its work area.
func (s *sampler) fitToWorkArea(p Placement) Placement {
	saved := winRect{
		Left:   int32(p.Left),
		Top:    int32(p.Top),
		Right:  int32(p.Left + p.Width),
		Bottom: int32(p.Top + p.Height),
	}
	monitor, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&saved)), monitorDefaultToNearest)
	if monitor == 0 {
		return p
	}
	var info winMonitorInfo
	info.CbSize = uint32(unsafe.Sizeof(info))
	if ok, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return p
	}
	work := workArea{
		Left:   int(info.RcWork.Left),
		Top:    int(info.RcWork.Top),
		Right:  int(info.RcWork.Right),
		Bottom: int(info.RcWork.Bottom),
	}
	return p.fitWithin(work, s.minWidth, s.minHeight)
}

// mainWindowHandle returns the cached main window handle, re-resolving it when
// Windows says the cached handle is no longer a window.
var (
	windowMu     sync.Mutex
	windowHandle syscall.Handle
)

func mainWindowHandle() syscall.Handle {
	windowMu.Lock()
	defer windowMu.Unlock()
	if windowHandle != 0 {
		if ok, _, _ := procIsWindow.Call(uintptr(windowHandle)); ok != 0 {
			return windowHandle
		}
		windowHandle = 0
	}
	windowHandle = findMainWindow()
	return windowHandle
}

// findMainWindow walks the top-level windows in Z order and returns the Wails
// window owned by this process. Matching the process ID first means a second
// Muse instance can never be mistaken for ours.
func findMainWindow() syscall.Handle {
	processID := uint32(os.Getpid())
	hwnd, _, _ := procGetTopWindow.Call(0)
	for hwnd != 0 {
		var ownerID uint32
		_, _, _ = procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&ownerID)))
		if ownerID == processID && windowClassName(hwnd) == wailsWindowClass {
			return syscall.Handle(hwnd)
		}
		hwnd, _, _ = procGetWindow.Call(hwnd, gwHwndNext)
	}
	return 0
}

func windowClassName(hwnd uintptr) string {
	var name [128]uint16
	length, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	if length == 0 {
		return ""
	}
	return syscall.UTF16ToString(name[:length])
}
