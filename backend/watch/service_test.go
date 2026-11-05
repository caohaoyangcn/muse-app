package watch

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"muse-app/backend/video"
)

// fakeLibrary emulates the favorites façade: it dedups by path like
// video.Service, records what was imported, and records which videos were
// auto-tagged by the watcher.
type fakeLibrary struct {
	mu     sync.Mutex
	videos map[string]string // lower-cased path -> file name
	seq    int64
	adds   [][]string
	tagged []string
	// relocating holds the lower-cased paths whose add has to report a
	// relocation instead of a new favorite, the way a moved file behaves.
	relocating map[string]bool
}

func newFakeLibrary() *fakeLibrary {
	return &fakeLibrary{videos: map[string]string{}}
}

func (f *fakeLibrary) ListVideos() ([]video.Video, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]video.Video, 0, len(f.videos))
	for p, name := range f.videos {
		out = append(out, video.Video{ID: 1, Path: p, FileName: name})
	}
	return out, nil
}

func (f *fakeLibrary) AddVideos(paths []string) []video.AddVideoResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.adds = append(f.adds, paths)
	results := make([]video.AddVideoResult, 0, len(paths))
	for _, p := range paths {
		key := strings.ToLower(p)
		if _, ok := f.videos[key]; ok {
			results = append(results, video.AddVideoResult{Path: p, Error: "already added"})
			continue
		}
		f.seq++
		f.videos[key] = filepath.Base(p)
		results = append(results, video.AddVideoResult{
			Path: p, Success: true, VideoID: f.seq, Relocated: f.relocating[key],
		})
	}
	return results
}

func (f *fakeLibrary) AutoTagVideos(results []video.AddVideoResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range results {
		if r.Success && !r.Relocated {
			f.tagged = append(f.tagged, r.Path)
		}
	}
}

func (f *fakeLibrary) has(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.videos[strings.ToLower(path)]
	return ok
}

type capturedEvent struct {
	name string
	data map[string]any
}

func newTestService(t *testing.T, lib LibrarySource) (*Service, *[]capturedEvent) {
	t.Helper()
	events := &[]capturedEvent{}
	svc := New(newTestRepo(t), lib, func(name string, data ...any) {
		m, _ := data[0].(map[string]any)
		*events = append(*events, capturedEvent{name: name, data: m})
	}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	t.Cleanup(svc.Stop)
	return svc, events
}

// touch creates an empty file (extension decides whether a scan picks it up).
func touch(t *testing.T, dir string, parts ...string) string {
	t.Helper()
	p := filepath.Join(append([]string{dir}, parts...)...)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestServiceAddScansRecursively(t *testing.T) {
	lib := newFakeLibrary()
	svc, events := newTestService(t, lib)
	root := t.TempDir()

	a := touch(t, root, "a.mp4")
	b := touch(t, root, "sub", "b.mkv")
	touch(t, root, "notes.txt")             // not a video extension
	touch(t, root, ".hidden", "c.mp4")      // hidden directories are skipped
	touch(t, root, "$RECYCLE.BIN", "d.mp4") // Windows system dirs are skipped

	if _, err := svc.Add(root); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Add queues the initial import on the scan worker; the synchronous
	// ScanAll stands in for it in tests.
	summary := svc.ScanAll()

	if summary.Added != 2 || summary.Failed != 0 || summary.Dirs != 1 {
		t.Errorf("summary: got %+v, want added=2 failed=0 dirs=1", summary)
	}
	if !lib.has(a) || !lib.has(b) {
		t.Error("expected a.mp4 and sub/b.mkv to be imported")
	}
	// Every successfully imported file must be passed to the auto-tag rules so
	// directory-monitored videos are grouped exactly like manual adds.
	if len(lib.tagged) != 2 {
		t.Errorf("expected 2 videos auto-tagged, got %v", lib.tagged)
	}
	for _, p := range []string{a, b} {
		found := false
		for _, tp := range lib.tagged {
			if tp == p {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s to be auto-tagged, got %v", p, lib.tagged)
		}
	}
	if lib.has(filepath.Join(root, "notes.txt")) {
		t.Error("non-video file must not be imported")
	}
	if lib.has(filepath.Join(root, ".hidden", "c.mp4")) {
		t.Error("hidden directory must be skipped")
	}
	if lib.has(filepath.Join(root, "$RECYCLE.BIN", "d.mp4")) {
		t.Error("system directory must be skipped")
	}

	// The scan reports its outcome so the frontend can toast + refresh.
	found := false
	for _, ev := range *events {
		if ev.name == "watch:scanned" && ev.data["added"] == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected watch:scanned event with added=2, got %+v", *events)
	}

	// Scanning again is idempotent: nothing new is imported.
	if again := svc.ScanAll(); again.Added != 0 {
		t.Errorf("rescan added %d videos, want 0", again.Added)
	}
}

// TestServiceScanOnceImportsWithoutWatching is the 扫描文件夹 entry: the files
// are imported, but the directory is not added to the watch list and no
// watch:scanned event is emitted. The one-off pass also must not silently apply
// the folder-watch auto-tag rules; the UI offers group suggestions instead.
func TestServiceScanOnceImportsWithoutWatching(t *testing.T) {
	lib := newFakeLibrary()
	svc, events := newTestService(t, lib)
	root := t.TempDir()

	a := touch(t, root, "a.mp4")
	b := touch(t, root, "sub", "b.mkv")
	touch(t, root, "notes.txt")
	touch(t, root, ".hidden", "c.mp4")
	touch(t, root, "$RECYCLE.BIN", "d.mp4")

	got, err := svc.ScanOnce(root)
	if err != nil {
		t.Fatalf("scan once: %v", err)
	}
	if got.Added != 2 || got.Failed != 0 || got.Skipped != 0 {
		t.Errorf("result: got %+v, want added=2 failed=0 skipped=0", got)
	}
	if len(got.Results) != 2 {
		t.Errorf("results: got %d, want 2", len(got.Results))
	}
	if !lib.has(a) || !lib.has(b) {
		t.Error("expected a.mp4 and sub/b.mkv to be imported")
	}
	if len(lib.tagged) != 0 {
		t.Errorf("one-off scan must not auto-tag, got %v", lib.tagged)
	}

	// It is a scan, not a watch: no persisted root and no watch event.
	dirs, err := svc.List()
	if err != nil {
		t.Fatalf("list watch dirs: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("one-off scan must not create watched dirs, got %+v", dirs)
	}
	if len(*events) != 0 {
		t.Errorf("one-off scan must not emit watch events, got %+v", *events)
	}

	// Running it again is idempotent: existing paths are filtered before add.
	again, err := svc.ScanOnce(root)
	if err != nil {
		t.Fatalf("second scan once: %v", err)
	}
	if again.Added != 0 || len(again.Results) != 0 {
		t.Errorf("second scan: got %+v, want added=0 results=0", again)
	}
}

func TestServiceScanOnceSkipsLockedFiles(t *testing.T) {
	lib := newFakeLibrary()
	svc, _ := newTestService(t, lib)
	root := t.TempDir()
	touch(t, root, "locked.mp4")
	svc.canImport = func(string) bool { return false }

	got, err := svc.ScanOnce(root)
	if err != nil {
		t.Fatalf("scan once: %v", err)
	}
	if got.Added != 0 || got.Failed != 0 || got.Skipped != 1 {
		t.Errorf("result: got %+v, want added=0 failed=0 skipped=1", got)
	}
	if len(got.Results) != 0 {
		t.Errorf("locked file must not reach AddVideos, got %+v", got.Results)
	}
}

func TestServiceScanOnceRejectsInvalidDirectory(t *testing.T) {
	lib := newFakeLibrary()
	svc, _ := newTestService(t, lib)
	root := t.TempDir()

	if _, err := svc.ScanOnce(filepath.Join(root, "missing")); !errors.Is(err, ErrNotExist) {
		t.Errorf("missing dir: err = %v, want ErrNotExist", err)
	}
	file := touch(t, root, "not-a-dir.mp4")
	if _, err := svc.ScanOnce(file); !errors.Is(err, ErrNotDir) {
		t.Errorf("file path: err = %v, want ErrNotDir", err)
	}
}

func TestServiceScanPicksUpNewFiles(t *testing.T) {
	lib := newFakeLibrary()
	svc, _ := newTestService(t, lib)
	root := t.TempDir()

	touch(t, root, "old.mp4")
	if _, err := svc.Add(root); err != nil {
		t.Fatalf("add: %v", err)
	}
	svc.ScanAll()

	// A video appears later — the next scan (the debounced worker in
	// production) must import it.
	fresh := touch(t, root, "new", "fresh.webm")
	if s := svc.ScanAll(); s.Added != 1 {
		t.Fatalf("second scan: got %+v, want added=1", s)
	}
	if !lib.has(fresh) {
		t.Error("expected the new file to be imported")
	}
}

// TestServiceScanCountsRelocatedSeparately keeps a reconnected favorite out of
// the "added" count: the scan did not import that file, it followed it to where
// it moved, and the UI says so.
// TestServiceScanWaitsUntilAFileIsReadable is the import gate: a file another
// program still holds exclusively cannot be identified by content, so importing
// it would create a favorite that a later rename turns into a duplicate. The
// scan leaves it for the next pass instead.
func TestServiceScanWaitsUntilAFileIsReadable(t *testing.T) {
	lib := newFakeLibrary()
	svc, _ := newTestService(t, lib)
	root := t.TempDir()
	ready := touch(t, root, "ready.mp4")
	locked := touch(t, root, "locked.mp4")
	if _, err := svc.Add(root); err != nil {
		t.Fatalf("add watch dir: %v", err)
	}

	svc.canImport = func(path string) bool { return !strings.EqualFold(path, locked) }
	summary := svc.ScanAll()
	if summary.Added != 1 || summary.Failed != 0 {
		t.Fatalf("summary: got %+v, want added=1 failed=0 (a locked file is not a failure)", summary)
	}
	if !lib.has(ready) {
		t.Error("the readable file must be imported")
	}
	if lib.has(locked) {
		t.Error("a file that cannot be read yet must not be imported")
	}

	// The writer lets go.
	svc.canImport = func(string) bool { return true }
	if s := svc.ScanAll(); s.Added != 1 {
		t.Fatalf("second scan: got %+v, want added=1", s)
	}
	if !lib.has(locked) {
		t.Error("the file must be imported once it is readable")
	}
}

func TestServiceScanCountsRelocatedSeparately(t *testing.T) {
	lib := newFakeLibrary()
	svc, events := newTestService(t, lib)
	root := t.TempDir()

	moved := touch(t, root, "moved.mp4")
	fresh := touch(t, root, "fresh.mp4")
	lib.relocating = map[string]bool{strings.ToLower(moved): true}

	if _, err := svc.Add(root); err != nil {
		t.Fatalf("add: %v", err)
	}
	summary := svc.ScanAll()
	if summary.Relocated != 1 || summary.Added != 1 || summary.Failed != 0 {
		t.Errorf("summary: got %+v, want added=1 relocated=1 failed=0", summary)
	}
	if !lib.has(moved) || !lib.has(fresh) {
		t.Error("both files must belong to the favorites after the scan")
	}
	// Only the freshly imported file goes through the auto-tag rules: the
	// reconnected one already went through them when it was first imported.
	if len(lib.tagged) != 1 || lib.tagged[0] != fresh {
		t.Errorf("auto-tagged %v, want only %s", lib.tagged, fresh)
	}
	found := false
	for _, ev := range *events {
		if ev.name == "watch:scanned" && ev.data["relocated"] == 1 && ev.data["added"] == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected watch:scanned with added=1 relocated=1, got %+v", *events)
	}
	// A reconnected favorite is not re-imported on the next pass either.
	if again := svc.ScanAll(); again.Added != 0 || again.Relocated != 0 {
		t.Errorf("rescan: got %+v, want nothing new", again)
	}
}

// TestWatcherImportsNewFilesLive exercises the real fsnotify loop: after a
// directory is watched, creating a nested directory and a video file in it —
// with no manual rescan at all — must land the file in the favorites.
func TestWatcherImportsNewFilesLive(t *testing.T) {
	lib := newFakeLibrary()
	svc, _ := newTestService(t, lib)
	root := t.TempDir()

	touch(t, root, "seed.mp4")
	if _, err := svc.Add(root); err != nil {
		t.Fatalf("add: %v", err)
	}
	svc.Start()

	waitFor(t, 10*time.Second, func() bool {
		return lib.has(filepath.Join(root, "seed.mp4"))
	}, "initial import of seed.mp4")

	// Appearing after the watch is armed: a new subdirectory plus a video in
	// it. The Create events must (a) register a watch on the new directory and
	// (b) trigger the debounced rescan that imports the file.
	fresh := touch(t, root, "later", "new.mp4")
	waitFor(t, 15*time.Second, func() bool {
		return lib.has(fresh)
	}, "live import of later/new.mp4")
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestServiceAddValidation(t *testing.T) {
	lib := newFakeLibrary()
	svc, _ := newTestService(t, lib)

	if _, err := svc.Add("  "); !errors.Is(err, ErrEmptyPath) {
		t.Errorf("empty path: err = %v, want ErrEmptyPath", err)
	}
	if _, err := svc.Add(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, ErrNotExist) {
		t.Errorf("missing dir: err = %v, want ErrNotExist", err)
	}
	file := touch(t, t.TempDir(), "a.mp4")
	if _, err := svc.Add(file); !errors.Is(err, ErrNotDir) {
		t.Errorf("file path: err = %v, want ErrNotDir", err)
	}

	root := t.TempDir()
	if _, err := svc.Add(root); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := svc.Add(root); !errors.Is(err, ErrAlreadyWatched) {
		t.Errorf("duplicate: err = %v, want ErrAlreadyWatched", err)
	}
	// Case-variant spelling of the same Windows folder is still a duplicate.
	if _, err := svc.Add(strings.ToUpper(root)); !errors.Is(err, ErrAlreadyWatched) {
		t.Errorf("case-variant duplicate: err = %v, want ErrAlreadyWatched", err)
	}
}

func TestServiceRemove(t *testing.T) {
	lib := newFakeLibrary()
	svc, _ := newTestService(t, lib)
	root := t.TempDir()
	touch(t, root, "a.mp4")

	d, err := svc.Add(root)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := svc.Remove(d.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	dirs, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("list after remove: got %+v", dirs)
	}
	if err := svc.Remove(d.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("remove twice: err = %v, want ErrNotFound", err)
	}
	// Removing the watch never touches favorites (收藏 ≠ 文件).
	if s := svc.ScanAll(); s.Dirs != 0 || s.Added != 0 {
		t.Errorf("scan after remove: got %+v", s)
	}
}

func TestUserMessage(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{ErrEmptyPath, "目录路径为空"},
		{ErrNotExist, "目录不存在"},
		{ErrNotDir, "该路径不是文件夹"},
		{ErrAlreadyWatched, "该目录已在监控清单中"},
		{ErrNotFound, "找不到该监控目录"},
	}
	for _, c := range cases {
		if got := UserMessage(c.err); got != c.want {
			t.Errorf("UserMessage(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}
