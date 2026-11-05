package video

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// supportedExtensions is the first-pass filter for add operations
// (spec-v1.1.md §9). Matching is case-insensitive.
var supportedExtensions = map[string]struct{}{
	".mp4":  {},
	".mkv":  {},
	".webm": {},
	".mov":  {},
	".avi":  {},
	".m4v":  {},
}

// IsSupportedVideoFile reports whether the file name has a supported video
// extension and is a real media file. Files whose name starts with "._" are
// AppleDouble metadata companions that macOS writes next to every file (e.g.
// "._movie.mp4") — they are not the media themselves and must never be
// imported, even though the suffix after the dot looks like a video extension.
func IsSupportedVideoFile(name string) bool {
	if strings.HasPrefix(name, "._") {
		return false
	}
	_, ok := supportedExtensions[strings.ToLower(filepath.Ext(name))]
	return ok
}

// NormalizeAndValidate validates a candidate path and returns its canonical
// form plus the real file info (spec-v1.md §7, §8, §27):
//   - must exist and be a regular file (directories and symlinks are rejected)
//   - must have a supported video extension
//   - normalized with filepath.Abs + Clean for duplicate detection
func NormalizeAndValidate(path string) (string, os.FileInfo, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", nil, ErrEmptyPath
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", nil, fmt.Errorf("resolve path: %w", err)
	}
	abs = filepath.Clean(abs)

	info, err := os.Lstat(abs)
	if err != nil {
		return "", nil, ErrNotExist
	}
	if info.Mode()&os.ModeDir != 0 {
		return "", nil, ErrFolderNotSupported
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, ErrSymlink
	}
	if !info.Mode().IsRegular() {
		return "", nil, ErrNotRegularFile
	}
	if !IsSupportedVideoFile(abs) {
		return "", nil, ErrUnsupportedType
	}

	realInfo, err := os.Stat(abs)
	if err != nil {
		return "", nil, ErrNotExist
	}
	return abs, realInfo, nil
}
