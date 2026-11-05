package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// FindTool locates a bundled or installed command-line tool. Bundled tools
// next to the app executable take priority so the packaged app works on
// machines without FFmpeg in PATH (spec-v1.md §38 "无开发环境可运行").
func FindTool(name string) (string, error) {
	if runtime.GOOS == "darwin" {
		for _, candidate := range []string{
			filepath.Join("/opt/homebrew/bin", name),
			filepath.Join("/usr/local/bin", name),
		} {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	if exe, err := os.Executable(); err == nil {
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".exe"
		}
		candidates := []string{filepath.Join(filepath.Dir(exe), name+ext)}
		if runtime.GOOS == "darwin" {
			candidates = append(candidates,
				filepath.Join(filepath.Dir(exe), "..", "Resources", "bin", name))
		}
		for _, candidate := range candidates {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("%s not found; install FFmpeg or place %s.exe next to the app", name, name)
	}
	return "", fmt.Errorf("%s not found; install it with Homebrew or add it to PATH", name)
}

// AppDataDir returns the persistent application data root.
func AppDataDir(appName string) (string, error) {
	var root string
	if runtime.GOOS == "windows" {
		root = os.Getenv("LOCALAPPDATA")
		if root == "" {
			return "", fmt.Errorf("LOCALAPPDATA is not set")
		}
	} else {
		var err error
		root, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve user config directory: %w", err)
		}
	}
	return filepath.Join(root, appName), nil
}
