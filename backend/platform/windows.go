//go:build windows

// Package platform wraps Windows shell integration and tool discovery.
package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// ---------- Generic command helpers ----------

// Command builds an exec.Cmd with the console window hidden, which prevents
// flashing a cmd.exe box when launching ffmpeg or the shell from the GUI app.
func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

// CommandContext is Command with a context for cancellation/timeout.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

// OpenWithDefaultApp opens path with the system default application
// (spec-v1.md §14).
//
// It uses ShellExecuteW rather than `cmd /c start`, because cmd.exe interprets
// shell metacharacters inside the path. A caret (^) in a file name is cmd's
// escape character and gets silently swallowed (^abc -> abc), so `start`
// receives a path that does not exist and Windows pops an "unable to open"
// error dialog. Amphersands (&) and friends split the command line entirely.
// ShellExecuteW takes the literal path and dispatches to the registered
// default handler with no shell re-parsing, so arbitrary file names work.
func OpenWithDefaultApp(path string) error {
	op := "open"
	opPtr, err := syscall.UTF16PtrFromString(op)
	if err != nil {
		return fmt.Errorf("encode shell operation: %w", err)
	}
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("encode shell path: %w", err)
	}
	rc, _, err := procShellExecuteW.Call(
		0, // hwnd: no owner window
		uintptr(unsafe.Pointer(opPtr)),
		uintptr(unsafe.Pointer(pathPtr)),
		0, // lpParameters
		0, // lpDirectory
		syscall.SW_SHOWNORMAL,
	)
	if rc <= 32 {
		// ShellExecuteW returns an error code <= 32 on failure.
		return fmt.Errorf("ShellExecuteW failed: code=%d err=%v", int32(rc), err)
	}
	return nil
}

// OpenURL opens an http(s) URL in the user's default browser. ShellExecuteW is
// the same dispatch OpenWithDefaultApp uses, with no cmd.exe re-parsing.
func OpenURL(rawURL string) error {
	opPtr, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return fmt.Errorf("encode shell operation: %w", err)
	}
	urlPtr, err := syscall.UTF16PtrFromString(rawURL)
	if err != nil {
		return fmt.Errorf("encode URL: %w", err)
	}
	rc, _, callErr := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(opPtr)), uintptr(unsafe.Pointer(urlPtr)), 0, 0, syscall.SW_SHOWNORMAL)
	if rc <= 32 {
		return fmt.Errorf("open URL failed: code=%d err=%v", int32(rc), callErr)
	}
	return nil
}

// ---------- Windows Shell API via syscall ----------
//
// "Open containing folder" is implemented with SHOpenFolderAndSelectItems, the
// official Shell API, rather than `explorer.exe /select`. The explorer CLI
// silently fails (and does not bring an already-open folder window to the
// foreground), whereas SHOpenFolderAndSelectItems reliably focuses the parent
// folder window and highlights the item — the behavior users expect.

var (
	shell32              = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW    = shell32.NewProc("ShellExecuteW")
	procSHOpenFolderSel  = shell32.NewProc("SHOpenFolderAndSelectItems")
	procILCreateFromPath = shell32.NewProc("ILCreateFromPath")
	procILFree           = shell32.NewProc("ILFree")
	ole32                = syscall.NewLazyDLL("ole32.dll")
	procCoInit           = ole32.NewProc("CoInitializeEx")
	procCoUninit         = ole32.NewProc("CoUninitialize")
)

// OpenContainingFolder opens the parent folder in Explorer with the file
// selected and focused (spec-v1.md §15). It works even when the folder is
// already open. The original video is never modified.
func OpenContainingFolder(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory")
	}

	parent := filepath.Dir(path)
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	parentPtr, err := syscall.UTF16PtrFromString(parent)
	if err != nil {
		return fmt.Errorf("encode parent path: %w", err)
	}
	absPtr, err := syscall.UTF16PtrFromString(abs)
	if err != nil {
		return fmt.Errorf("encode file path: %w", err)
	}

	// Shell item browsing needs COM initialized on this thread (STA).
	_, _, _ = procCoInit.Call(0, 2) // COINIT_APARTMENTTHREADED
	defer func() { _, _, _ = procCoUninit.Call() }()

	parentPIDL, _, perr := procILCreateFromPath.Call(uintptr(unsafe.Pointer(parentPtr)))
	if parentPIDL == 0 {
		return fmt.Errorf("resolve parent pidl: %v", perr)
	}
	defer func() { _, _, _ = procILFree.Call(parentPIDL) }()

	childPIDL, _, cerr := procILCreateFromPath.Call(uintptr(unsafe.Pointer(absPtr)))
	if childPIDL == 0 {
		return fmt.Errorf("resolve file pidl: %v", cerr)
	}
	defer func() { _, _, _ = procILFree.Call(childPIDL) }()

	apidl := []uintptr{childPIDL}
	// flags=0: no prompt, no edit box; focus the window and select the item.
	if rcode, _, _ := procSHOpenFolderSel.Call(parentPIDL, 1, uintptr(unsafe.Pointer(&apidl[0])), 0); rcode != 0 {
		return fmt.Errorf("SHOpenFolderAndSelectItems: hresult %#x", rcode)
	}
	return nil
}

// CopyText writes text to the Windows clipboard when the Wails clipboard API
// is unavailable.
func CopyText(text string) error {
	cmd := Command("cmd", "/c", "clip")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
