package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The persistent data root is resolved before logging and the database open,
// so it cannot live in the database it controls. Two bootstrap files next to
// the executable (and nothing else) can move it:
//
//	muse.config.json  {"dataDir": "D:\\MuseData"}  explicit root
//	muse.portable     marker file -> <exe dir>/data
//
// The config file wins when both exist. A relative dataDir is resolved against
// the executable's directory and the value may contain environment variables.
const (
	ConfigFileName     = "muse.config.json"
	PortableMarkerName = "muse.portable"

	// DataDirDefault is %LOCALAPPDATA%\Muse on Windows and
	// ~/Library/Application Support/Muse on macOS.
	DataDirDefault = "default"
	// DataDirConfig means muse.config.json supplied the path.
	DataDirConfig = "config"
	// DataDirPortable means the muse.portable marker selected <exe>/data.
	DataDirPortable = "portable"

	portableDataDirName = "data"
)

// DataDirResolution reports where persistent data lives and why. Source lets
// startup logging explain an unexpected location; Warning is set when a
// bootstrap file exists but was ignored (the default root is used instead).
type DataDirResolution struct {
	Path       string
	Source     string
	ConfigPath string
	Warning    string
}

// ResolveDataDir picks the persistent data root for appName. Only a failure to
// determine the platform default is fatal: a broken bootstrap file falls back
// to the default root and reports why, because exiting would be worse than
// starting with the standard location.
func ResolveDataDir(appName string) (DataDirResolution, error) {
	defaultDir, err := AppDataDir(appName)
	if err != nil {
		return DataDirResolution{}, err
	}
	exeDir, err := executableDir()
	if err != nil {
		return DataDirResolution{Path: defaultDir, Source: DataDirDefault}, nil
	}
	return resolveDataDir(defaultDir, exeDir), nil
}

// resolveDataDir is the testable core: exeDir is where the bootstrap files are
// looked up, defaultDir is the fallback.
func resolveDataDir(defaultDir, exeDir string) DataDirResolution {
	res := DataDirResolution{Path: defaultDir, Source: DataDirDefault}

	configPath := filepath.Join(exeDir, ConfigFileName)
	raw, err := os.ReadFile(configPath)
	switch {
	case err == nil:
		configured, warning := configuredDataDir(raw, exeDir)
		if warning != "" {
			res.Warning = warning
			return res
		}
		res.Path = configured
		res.Source = DataDirConfig
		res.ConfigPath = configPath
		return res
	case errors.Is(err, fs.ErrNotExist):
		// No config file: portable marker may still apply.
	default:
		res.Warning = fmt.Sprintf("%s: %v", configPath, err)
		return res
	}

	if _, err := os.Stat(filepath.Join(exeDir, PortableMarkerName)); err == nil {
		res.Path = filepath.Join(exeDir, portableDataDirName)
		res.Source = DataDirPortable
	}
	return res
}

// configuredDataDir parses muse.config.json. It returns either an absolute
// path or a warning explaining why the file cannot be used.
func configuredDataDir(raw []byte, exeDir string) (string, string) {
	var conf struct {
		DataDir string `json:"dataDir"`
	}
	if err := json.Unmarshal(raw, &conf); err != nil {
		return "", fmt.Sprintf("%s: %v", ConfigFileName, err)
	}
	dir := strings.TrimSpace(conf.DataDir)
	if dir == "" {
		return "", fmt.Sprintf("%s: dataDir is empty", ConfigFileName)
	}
	dir = expandEnv(dir)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(exeDir, dir)
	}
	return filepath.Clean(dir), ""
}

// percentEnvPattern matches Windows-style %VAR% references. os.ExpandEnv only
// understands $VAR / ${VAR}, but Windows users write %LOCALAPPDATA% by habit,
// so both forms are accepted on every platform.
var percentEnvPattern = regexp.MustCompile(`%([^%]+)%`)

func expandEnv(value string) string {
	value = os.ExpandEnv(value)
	return percentEnvPattern.ReplaceAllStringFunc(value, func(match string) string {
		if expanded, ok := os.LookupEnv(strings.Trim(match, "%")); ok {
			return expanded
		}
		return match
	})
}

// executableDir returns the directory holding the running binary, with any
// symlink resolved so a launcher link does not hide the config file.
func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe), nil
}
