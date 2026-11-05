package cloud

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func cacheSpec(videoID int64, name string, size int64) CacheSpec {
	return CacheSpec{VideoID: videoID, SourceID: 3, RemotePath: "/媒体/" + name, FileName: name, Size: size}
}

func cachedStat(size, modified int64) func(context.Context, int64, string) (*Entry, error) {
	return func(context.Context, int64, string) (*Entry, error) {
		return &Entry{Path: "/媒体/a.mp4", Size: size, Modified: modified, ContentType: "video/mp4"}, nil
	}
}

func TestCacheDownloadsAndReuses(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir, 1<<20, nil, nil)
	body := "0123456789"
	opens := 0
	open := func(context.Context, int64, string, int64, int64) (*RangeResponse, error) {
		opens++
		return &RangeResponse{
			Body:          io.NopCloser(strings.NewReader(body)),
			StatusCode:    200,
			ContentType:   "video/mp4",
			ContentLength: int64(len(body)),
		}, nil
	}
	spec := cacheSpec(7, "影片 a.mp4", int64(len(body)))
	path, err := cache.Ensure(context.Background(), spec, cachedStat(int64(len(body)), 100), open)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != body {
		t.Errorf("cached body = %q", got)
	}
	if filepath.Base(path) != "7-影片 a.mp4" {
		t.Errorf("cache path = %q", path)
	}
	// A matching cache is a hit: no stat/download body needed beyond the
	// freshness stat.
	path2, err := cache.Ensure(context.Background(), spec, cachedStat(int64(len(body)), 100), open)
	if err != nil || path2 != path {
		t.Fatalf("cache hit = %q, %v", path2, err)
	}
	if opens != 1 {
		t.Errorf("opens = %d, want 1 (second Ensure should hit the cache)", opens)
	}
	// A changed remote mtime invalidates the copy.
	path3, err := cache.Ensure(context.Background(), spec, cachedStat(int64(len(body)), 101), open)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if path3 != path || opens != 2 {
		t.Errorf("refresh path=%q opens=%d", path3, opens)
	}
}

func TestCacheResumesPartialDownload(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir, 1<<20, nil, nil)
	spec := cacheSpec(9, "a.mp4", 10)
	// A previous attempt wrote four bytes and recorded its matching remote
	// facts. The fresh attempt must ask for offset 4, not restart.
	if err := os.WriteFile(cache.partPath(9), []byte("0123"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cache.writeDownloadMeta(spec, 55); err != nil {
		t.Fatal(err)
	}
	var offset int64
	open := func(_ context.Context, _ int64, _ string, start, _ int64) (*RangeResponse, error) {
		offset = start
		return &RangeResponse{
			Body:          io.NopCloser(strings.NewReader("456789")),
			StatusCode:    206,
			ContentLength: 6,
			ContentRange:  "bytes 4-9/10",
		}, nil
	}
	path, err := cache.Ensure(context.Background(), spec, cachedStat(10, 55), open)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "0123456789" {
		t.Errorf("resumed body = %q", got)
	}
	if offset != 4 {
		t.Errorf("resume offset = %d, want 4", offset)
	}
}

func TestCacheEvictsLRUAndClears(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir, 10, nil, nil)
	for _, id := range []int64{1, 2} {
		path := filepath.Join(dir, "x")
		if err := os.WriteFile(path, []byte("12345678"), 0o644); err != nil {
			t.Fatal(err)
		}
		final := filepath.Join(dir, filepath.Base(cache.finalPath(CacheSpec{VideoID: id, FileName: "a.mp4"})))
		if err := os.Rename(path, final); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(cache.metaPath(id), []byte(`{"sourceId":3,"size":8}`), 0o644)
		old := time.Now().Add(-time.Duration(3-id) * time.Hour)
		_ = os.Chtimes(final, old, old)
	}
	if info := cache.Info(); info.Bytes < 16 {
		t.Fatalf("info bytes = %d", info.Bytes)
	}
	if err := cache.EnforceLimit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "1-a.mp4")); !os.IsNotExist(err) {
		t.Errorf("oldest cache file should be evicted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2-a.mp4")); err != nil {
		t.Errorf("newest cache file should survive: %v", err)
	}
	// Removing one favorite deletes its final file and metadata, not others.
	cache.Remove(2)
	if _, err := os.Stat(filepath.Join(dir, "2-a.mp4")); !os.IsNotExist(err) {
		t.Error("Remove did not delete the favorite cache")
	}
	if _, err := os.Stat(cache.metaPath(2)); !os.IsNotExist(err) {
		t.Error("Remove did not delete the metadata")
	}
	if _, err := cache.Clear(); err != nil {
		t.Fatal(err)
	}
	if info := cache.Info(); info.Files != 0 {
		t.Errorf("cache not empty after Clear: %+v", info)
	}
}

func TestCacheRefusesOversizedFileAndSanitizesName(t *testing.T) {
	cache := NewCache(t.TempDir(), 10, nil, nil)
	_, err := cache.Ensure(context.Background(), cacheSpec(1, "big.mp4", 20), cachedStat(20, 1),
		func(context.Context, int64, string, int64, int64) (*RangeResponse, error) {
			t.Fatal("oversized file must not be downloaded")
			return nil, nil
		})
	if !errors.Is(err, ErrCacheTooSmall) {
		t.Errorf("oversize err = %v, want ErrCacheTooSmall", err)
	}
	for input, want := range map[string]string{
		"../../etc/passwd": "passwd",
		`a:b*c?.mp4`:       "a_b_c_.mp4",
		"":                 "video",
	} {
		if got := safeCacheName(input); got != want {
			t.Errorf("safeCacheName(%q) = %q, want %q", input, got, want)
		}
	}
}
