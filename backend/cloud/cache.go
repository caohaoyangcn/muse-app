package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// DefaultCacheLimit is the desktop cache ceiling (10 GiB). It is a preference
// applied by cache eviction, not a cap on what can be streamed.
const DefaultCacheLimit int64 = 10 << 30

var (
	// ErrDownloadInProgress means the same favorite is already downloading.
	ErrDownloadInProgress = errors.New("cloud download already in progress")
	// ErrCacheTooSmall means a file is larger than the whole cache limit, so
	// downloading it would immediately be evicted by its own next cleanup.
	ErrCacheTooSmall = errors.New("cloud file is larger than the cache limit")
)

// CacheSpec describes one remote favorite the cache should hold.
type CacheSpec struct {
	VideoID    int64
	SourceID   int64
	RemotePath string
	FileName   string
	Size       int64
}

// CacheInfo is what the settings screen shows.
type CacheInfo struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
	Limit int64 `json:"limit"`
}

// DownloadProgress is the cloud:download payload. Status is one of
// downloading / done / error / cancelled.
type DownloadProgress struct {
	VideoID    int64  `json:"videoId"`
	FileName   string `json:"fileName"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Status     string `json:"status"`
	Error      string `json:"error"`
}

// Cache stores downloaded copies of remote favorites. It is deliberately a
// downloader rather than a streaming cache: v1 downloads before opening the
// system player, and the tiny metadata proxy handles covers without writing a
// full file. Deleting a favorite removes its cached copy; deleting the cache
// never touches the WebDAV source.
type Cache struct {
	dir   string
	limit int64
	emit  func(name string, data ...any)
	log   *slog.Logger

	mu        sync.Mutex
	downloads map[int64]context.CancelFunc
}

// NewCache builds the cache under dir. limit <= 0 uses DefaultCacheLimit.
func NewCache(dir string, limit int64, emit func(name string, data ...any), log *slog.Logger) *Cache {
	if limit <= 0 {
		limit = DefaultCacheLimit
	}
	return &Cache{
		dir:       dir,
		limit:     limit,
		emit:      emit,
		log:       log,
		downloads: make(map[int64]context.CancelFunc),
	}
}

// Dir returns the cache root (used only for display/diagnostics).
func (c *Cache) Dir() string { return c.dir }

// Ensure returns a local path for the remote file, downloading it if the cache
// is absent, stale (size/mtime changed), or only partially written. It resumes
// a matching .part file with a Range request when the server supports it.
func (c *Cache) Ensure(
	ctx context.Context,
	spec CacheSpec,
	stat func(context.Context, int64, string) (*Entry, error),
	open func(context.Context, int64, string, int64, int64) (*RangeResponse, error),
) (string, error) {
	if spec.VideoID <= 0 || spec.SourceID <= 0 || spec.RemotePath == "" {
		return "", ErrInvalidPath
	}
	if stat == nil || open == nil {
		return "", errors.New("cloud cache requires stat and open callbacks")
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return "", fmt.Errorf("create cloud cache: %w", err)
	}

	// A completed copy is a fallback for an offline source. Revalidate the
	// remote mtime with a short bound: if the source answers and the file
	// changed, download again; if it cannot answer, play what is already here.
	meta, hasMeta := c.readMeta(spec.VideoID)
	cached := ""
	if hasMeta && meta.SourceID == spec.SourceID && meta.RemotePath == spec.RemotePath &&
		(spec.Size <= 0 || meta.Size == spec.Size) {
		candidate := c.finalPath(CacheSpec{VideoID: spec.VideoID, FileName: meta.FileName})
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			cached = candidate
		}
	}
	var entry *Entry
	var err error
	if cached != "" {
		revalidateCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		entry, err = stat(revalidateCtx, spec.SourceID, spec.RemotePath)
		cancel()
		if err != nil {
			c.touch(cached)
			return cached, nil
		}
	} else {
		entry, err = stat(ctx, spec.SourceID, spec.RemotePath)
		if err != nil {
			return "", err
		}
	}
	if entry.IsDir {
		return "", ErrNotFound
	}
	actual := CacheSpec{
		VideoID:    spec.VideoID,
		SourceID:   spec.SourceID,
		RemotePath: spec.RemotePath,
		FileName:   spec.FileName,
		Size:       entry.Size,
	}
	finalPath := c.finalPath(actual)
	if cached != "" && meta.matches(actual, entry.Modified) {
		c.touch(cached)
		return cached, nil
	}
	if c.limit > 0 && actual.Size > c.limit {
		return "", ErrCacheTooSmall
	}

	runCtx, cancel, err := c.begin(ctx, spec.VideoID)
	if err != nil {
		return "", err
	}
	defer c.end(spec.VideoID)
	defer cancel()

	if err := c.download(runCtx, actual, entry.Modified, finalPath, stat, open); err != nil {
		if errors.Is(err, context.Canceled) {
			c.emitProgress(actual, 0, actual.Size, "cancelled", "")
			return "", err
		}
		c.emitProgress(actual, 0, actual.Size, "error", UserMessage(err))
		return "", err
	}
	c.removeOtherFinals(actual.VideoID, finalPath)
	c.emitProgress(actual, actual.Size, actual.Size, "done", "")
	if err := c.enforceLimitAt(finalPath); err != nil && c.log != nil {
		c.log.Warn("cloud cache eviction failed", "err", err)
	}
	c.touch(finalPath)
	return finalPath, nil
}

type cacheMeta struct {
	SourceID   int64  `json:"sourceId"`
	RemotePath string `json:"remotePath"`
	Size       int64  `json:"size"`
	Modified   int64  `json:"modified"`
	FileName   string `json:"fileName"`
	Downloaded int64  `json:"downloadedAt"`
}

func (m cacheMeta) matches(spec CacheSpec, modified int64) bool {
	return m.SourceID == spec.SourceID && m.RemotePath == spec.RemotePath &&
		m.Size == spec.Size && m.Modified == modified
}

func (c *Cache) begin(parent context.Context, id int64) (context.Context, context.CancelFunc, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, busy := c.downloads[id]; busy {
		return nil, nil, ErrDownloadInProgress
	}
	ctx, cancel := context.WithCancel(parent)
	c.downloads[id] = cancel
	return ctx, cancel, nil
}

func (c *Cache) end(id int64) {
	c.mu.Lock()
	delete(c.downloads, id)
	c.mu.Unlock()
}

// Cancel stops one in-flight download. A partial file is kept so a later
// attempt can resume it.
func (c *Cache) Cancel(videoID int64) {
	c.mu.Lock()
	cancel := c.downloads[videoID]
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *Cache) download(ctx context.Context, spec CacheSpec, modified int64, finalPath string,
	stat func(context.Context, int64, string) (*Entry, error),
	open func(context.Context, int64, string, int64, int64) (*RangeResponse, error)) error {
	partPath := c.partPath(spec.VideoID)
	downloadMetaPath := c.downloadMetaPath(spec.VideoID)

	offset := int64(0)
	if partial, ok := c.readDownloadMeta(spec.VideoID); ok && partial.matches(spec, modified) {
		if info, err := os.Stat(partPath); err == nil && info.Mode().IsRegular() && info.Size() <= spec.Size {
			offset = info.Size()
		}
	}
	// A complete partial file needs no network at all.
	if spec.Size > 0 && offset == spec.Size {
		if err := c.finalize(partPath, finalPath, downloadMetaPath, spec, modified); err != nil {
			return err
		}
		return nil
	}
	if err := c.writeDownloadMeta(spec, modified); err != nil {
		return err
	}
	if offset > 0 {
		c.emitProgress(spec, offset, spec.Size, "downloading", "")
	} else {
		c.emitProgress(spec, 0, spec.Size, "downloading", "")
	}

	resp, err := open(ctx, spec.SourceID, spec.RemotePath, offset, 0)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 200 && offset > 0 {
		// The server ignored the resume range; restart from byte zero rather
		// than appending the whole file to its own tail.
		offset = 0
		if err := os.Remove(partPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	flags := os.O_CREATE | os.O_WRONLY
	if offset == 0 {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(partPath, flags, 0o644)
	if err != nil {
		return fmt.Errorf("open cloud cache part: %w", err)
	}
	written := offset
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			_ = f.Close()
			return err
		}
	}
	buf := make([]byte, 256<<10)
	lastEmit := time.Now()
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				_ = f.Close()
				return fmt.Errorf("write cloud cache: %w", err)
			}
			written += int64(n)
			if time.Since(lastEmit) >= 250*time.Millisecond {
				c.emitProgress(spec, written, spec.Size, "downloading", "")
				lastEmit = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = f.Close()
			return readErr
		}
		if ctx.Err() != nil {
			_ = f.Close()
			return ctx.Err()
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if spec.Size > 0 && written != spec.Size {
		return fmt.Errorf("cloud download incomplete: %d/%d bytes", written, spec.Size)
	}
	if err := c.finalize(partPath, finalPath, downloadMetaPath, spec, modified); err != nil {
		return err
	}
	return nil
}

// removeOtherFinals deletes a previous cache file for the same video id when
// the display name (and therefore the file name) changed.
func (c *Cache) removeOtherFinals(videoID int64, keep string) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	prefix := fmt.Sprintf("%d-", videoID)
	for _, entry := range entries {
		path := filepath.Join(c.dir, entry.Name())
		if path != keep && strings.HasPrefix(entry.Name(), prefix) && !strings.HasSuffix(entry.Name(), ".part") {
			_ = os.Remove(path)
		}
	}
}

func (c *Cache) finalize(partPath, finalPath, downloadMetaPath string, spec CacheSpec, modified int64) error {
	// Remove the previous final copy before rename so Windows can replace it.
	_ = os.Remove(finalPath)
	if err := os.Rename(partPath, finalPath); err != nil {
		return fmt.Errorf("finish cloud download: %w", err)
	}
	meta := cacheMeta{
		SourceID:   spec.SourceID,
		RemotePath: spec.RemotePath,
		Size:       spec.Size,
		Modified:   modified,
		FileName:   spec.FileName,
		Downloaded: time.Now().UnixMilli(),
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(c.metaPath(spec.VideoID), raw, 0o644); err != nil {
		return fmt.Errorf("write cloud cache metadata: %w", err)
	}
	_ = os.Remove(downloadMetaPath)
	return nil
}

// Lookup returns the cached path when it still matches the size/mtime the
// caller observed. A stale cache is not removed here; Ensure owns that.
func (c *Cache) Lookup(videoID int64, size, modified int64) (string, bool) {
	meta, ok := c.readMeta(videoID)
	if !ok || meta.Size != size || meta.Modified != modified {
		return "", false
	}
	path := c.finalPath(CacheSpec{VideoID: videoID, FileName: meta.FileName, Size: size})
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		return path, true
	}
	return "", false
}

// Remove deletes every cached artifact for one favorite (final, part, meta).
func (c *Cache) Remove(videoID int64) {
	c.Cancel(videoID)
	prefix := fmt.Sprintf("%d-", videoID)
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, prefix) || name == fmt.Sprintf("%d.json", videoID) || name == fmt.Sprintf("%d.download.json", videoID) {
			_ = os.Remove(filepath.Join(c.dir, name))
		}
	}
}

// Clear cancels every download and removes all cached files.
func (c *Cache) Clear() (int64, error) {
	c.mu.Lock()
	for _, cancel := range c.downloads {
		cancel()
	}
	c.mu.Unlock()

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var removed int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(c.dir, entry.Name())
		if info, statErr := entry.Info(); statErr == nil {
			removed += info.Size()
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
	}
	return removed, nil
}

// Info reports current usage and the configured limit.
func (c *Cache) Info() CacheInfo {
	info := CacheInfo{Limit: c.limit}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return info
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if fileInfo, err := entry.Info(); err == nil {
			info.Bytes += fileInfo.Size()
			// Metadata and partial files consume space but are not playable
			// copies; the UI's file count means "downloaded videos".
			if !strings.HasSuffix(entry.Name(), ".json") && !strings.HasSuffix(entry.Name(), ".part") {
				info.Files++
			}
		}
	}
	return info
}

// EnforceLimit evicts least-recently-used completed downloads until the cache
// is under its limit. It is safe to call at startup and after a download.
func (c *Cache) EnforceLimit() error { return c.enforceLimitAt("") }

func (c *Cache) enforceLimitAt(keep string) error {
	if c.limit <= 0 {
		return nil
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	type cached struct {
		path  string
		size  int64
		mtime time.Time
	}
	files := make([]cached, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".part") || strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, cached{path: filepath.Join(c.dir, entry.Name()), size: info.Size(), mtime: info.ModTime()})
		total += info.Size()
	}
	if total <= c.limit {
		return nil
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.Before(files[j].mtime) })
	for _, file := range files {
		if total <= c.limit {
			break
		}
		if file.path == keep {
			continue
		}
		id := videoIDFromCacheName(filepath.Base(file.path))
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			continue
		}
		if id > 0 {
			_ = os.Remove(c.metaPath(id))
		}
		total -= file.size
	}
	return nil
}

func (c *Cache) finalPath(spec CacheSpec) string {
	return filepath.Join(c.dir, fmt.Sprintf("%d-%s", spec.VideoID, safeCacheName(spec.FileName)))
}

func (c *Cache) partPath(videoID int64) string {
	return filepath.Join(c.dir, fmt.Sprintf("%d.part", videoID))
}

func (c *Cache) metaPath(videoID int64) string {
	return filepath.Join(c.dir, fmt.Sprintf("%d.json", videoID))
}

func (c *Cache) downloadMetaPath(videoID int64) string {
	return filepath.Join(c.dir, fmt.Sprintf("%d.download.json", videoID))
}

func (c *Cache) readMeta(videoID int64) (cacheMeta, bool) {
	var meta cacheMeta
	raw, err := os.ReadFile(c.metaPath(videoID))
	if err != nil {
		return meta, false
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return meta, false
	}
	return meta, true
}

func (c *Cache) writeDownloadMeta(spec CacheSpec, modified int64) error {
	meta := cacheMeta{
		SourceID:   spec.SourceID,
		RemotePath: spec.RemotePath,
		Size:       spec.Size,
		Modified:   modified,
		FileName:   spec.FileName,
		Downloaded: time.Now().UnixMilli(),
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(c.downloadMetaPath(spec.VideoID), raw, 0o644); err != nil {
		return fmt.Errorf("write cloud download metadata: %w", err)
	}
	return nil
}

func (c *Cache) readDownloadMeta(videoID int64) (cacheMeta, bool) {
	var meta cacheMeta
	raw, err := os.ReadFile(c.downloadMetaPath(videoID))
	if err != nil {
		return meta, false
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return meta, false
	}
	return meta, true
}

func (c *Cache) emitProgress(spec CacheSpec, downloaded, total int64, status, message string) {
	if c.emit == nil {
		return
	}
	c.emit("cloud:download", DownloadProgress{
		VideoID:    spec.VideoID,
		FileName:   spec.FileName,
		Downloaded: downloaded,
		Total:      total,
		Status:     status,
		Error:      message,
	})
}

func (c *Cache) touch(path string) {
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

// safeCacheName keeps a display name usable on Windows without allowing path
// traversal. The video id already makes the file unique.
func safeCacheName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteRune('_')
		case unicode.IsControl(r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(strings.TrimSpace(b.String()), ".")
	if out == "" {
		out = "video"
	}
	if len(out) > 120 {
		ext := filepath.Ext(out)
		if len(ext) > 10 {
			ext = ""
		}
		out = out[:120-len(ext)] + ext
	}
	return out
}

func videoIDFromCacheName(name string) int64 {
	dash := strings.IndexByte(name, '-')
	if dash <= 0 {
		return 0
	}
	var id int64
	for _, r := range name[:dash] {
		if r < '0' || r > '9' {
			return 0
		}
		id = id*10 + int64(r-'0')
	}
	return id
}
