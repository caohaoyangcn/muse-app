//go:build darwin

package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Command creates a normal macOS child process. Finder-launched apps do not
// have a console window to hide.
func Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// CommandContext is Command with cancellation support.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// OpenWithDefaultApp opens a file with the user's default application.
func OpenWithDefaultApp(path string) error {
	if err := Command("open", path).Run(); err != nil {
		return fmt.Errorf("open default app: %w", err)
	}
	return nil
}

// OpenURL opens an http(s) URL in the user's default browser.
func OpenURL(rawURL string) error {
	if err := Command("open", rawURL).Run(); err != nil {
		return fmt.Errorf("open URL: %w", err)
	}
	return nil
}

// OpenContainingFolder reveals a file in Finder.
func OpenContainingFolder(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if err := Command("open", "-R", abs).Run(); err != nil {
		return fmt.Errorf("reveal in Finder: %w", err)
	}
	return nil
}

// CopyText writes text to the macOS clipboard when the Wails clipboard API is
// unavailable.
func CopyText(text string) error {
	cmd := Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
