package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDataDir(t *testing.T) {
	const envName = "MUSE_TEST_DATA_ROOT"
	envTarget := filepath.Join(t.TempDir(), "expanded-root")
	t.Setenv(envName, envTarget)

	tests := []struct {
		name string
		// files are written next to the executable before resolving.
		files func(exeDir string) map[string]string
		// wantPath derives the expected root from exeDir/defaultDir so a case
		// can use a deterministic absolute path.
		wantPath   func(exeDir, defaultDir string) string
		wantSource string
		wantWarn   bool
	}{
		{
			name:       "no bootstrap file keeps the platform default",
			wantPath:   func(_, defaultDir string) string { return defaultDir },
			wantSource: DataDirDefault,
		},
		{
			name:       "portable marker moves data next to the exe",
			files:      func(string) map[string]string { return map[string]string{PortableMarkerName: ""} },
			wantPath:   func(exeDir, _ string) string { return filepath.Join(exeDir, "data") },
			wantSource: DataDirPortable,
		},
		{
			name: "config file accepts an absolute root",
			files: func(exeDir string) map[string]string {
				root := filepath.Join(filepath.Dir(exeDir), "absolute-root")
				return map[string]string{ConfigFileName: `{"dataDir": "` + filepath.ToSlash(root) + `"}`}
			},
			wantPath:   func(exeDir, _ string) string { return filepath.Join(filepath.Dir(exeDir), "absolute-root") },
			wantSource: DataDirConfig,
		},
		{
			name:       "config file resolves a relative root against the exe directory",
			files:      func(string) map[string]string { return map[string]string{ConfigFileName: `{"dataDir": "library"}`} },
			wantPath:   func(exeDir, _ string) string { return filepath.Join(exeDir, "library") },
			wantSource: DataDirConfig,
		},
		{
			name:       "config file trims surrounding whitespace",
			files:      func(string) map[string]string { return map[string]string{ConfigFileName: `{"dataDir": "  library  "}`} },
			wantPath:   func(exeDir, _ string) string { return filepath.Join(exeDir, "library") },
			wantSource: DataDirConfig,
		},
		{
			name: "config file expands environment variables",
			files: func(string) map[string]string {
				return map[string]string{ConfigFileName: `{"dataDir": "$MUSE_TEST_DATA_ROOT"}`}
			},
			wantPath:   func(_, _ string) string { return envTarget },
			wantSource: DataDirConfig,
		},
		{
			name: "config file expands Windows-style percent variables",
			files: func(string) map[string]string {
				return map[string]string{ConfigFileName: `{"dataDir": "%MUSE_TEST_DATA_ROOT%"}`}
			},
			wantPath:   func(_, _ string) string { return envTarget },
			wantSource: DataDirConfig,
		},
		{
			name: "config file ignores unknown fields",
			files: func(string) map[string]string {
				return map[string]string{ConfigFileName: `{"dataDir": "library", "future": true}`}
			},
			wantPath:   func(exeDir, _ string) string { return filepath.Join(exeDir, "library") },
			wantSource: DataDirConfig,
		},
		{
			name: "config file wins over the portable marker",
			files: func(string) map[string]string {
				return map[string]string{ConfigFileName: `{"dataDir": "chosen"}`, PortableMarkerName: ""}
			},
			wantPath:   func(exeDir, _ string) string { return filepath.Join(exeDir, "chosen") },
			wantSource: DataDirConfig,
		},
		{
			name:       "empty dataDir is ignored with a warning",
			files:      func(string) map[string]string { return map[string]string{ConfigFileName: `{"dataDir": "   "}`} },
			wantPath:   func(_, defaultDir string) string { return defaultDir },
			wantSource: DataDirDefault,
			wantWarn:   true,
		},
		{
			name:       "malformed config is ignored with a warning",
			files:      func(string) map[string]string { return map[string]string{ConfigFileName: `{"dataDir": `} },
			wantPath:   func(_, defaultDir string) string { return defaultDir },
			wantSource: DataDirDefault,
			wantWarn:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exeDir := t.TempDir()
			defaultDir := filepath.Join(t.TempDir(), "Muse")
			if tc.files != nil {
				for name, content := range tc.files(exeDir) {
					if err := os.WriteFile(filepath.Join(exeDir, name), []byte(content), 0o644); err != nil {
						t.Fatalf("write %s: %v", name, err)
					}
				}
			}

			res := resolveDataDir(defaultDir, exeDir)

			want := tc.wantPath(exeDir, defaultDir)
			if res.Path != want {
				t.Errorf("Path = %q, want %q", res.Path, want)
			}
			if res.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", res.Source, tc.wantSource)
			}
			if gotWarn := res.Warning != ""; gotWarn != tc.wantWarn {
				t.Errorf("Warning = %q, wantWarning %v", res.Warning, tc.wantWarn)
			}
			if tc.wantSource == DataDirConfig && res.ConfigPath != filepath.Join(exeDir, ConfigFileName) {
				t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, filepath.Join(exeDir, ConfigFileName))
			}
		})
	}
}

func TestExecutableDirPointsAtARunningBinary(t *testing.T) {
	dir, err := executableDir()
	if err != nil {
		t.Fatalf("executableDir: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("executableDir = %q, want an absolute path", dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat executable dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", dir)
	}
}
