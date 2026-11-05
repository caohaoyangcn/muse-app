package server

import (
	"path/filepath"
)

// filepathFromData joins a path under the app data directory, converting to the
// OS separator. Kept separate so callers don't spread filepath.Join/FromSlash
// knowledge across the package.
func filepathFromData(dataDir string, parts ...string) string {
	all := append([]string{dataDir}, parts...)
	return filepath.Join(all...)
}
