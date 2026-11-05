package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func startTestProxy(t *testing.T, size int64, open func(context.Context, int64, string, int64, int64) (*RangeResponse, error)) *MediaProxy {
	t.Helper()
	proxy, err := NewMediaProxy(
		func(id int64) (MediaInfo, error) {
			if id != 7 {
				return MediaInfo{}, errors.New("not found")
			}
			return MediaInfo{SourceID: 3, Path: "/媒体/a.mp4", FileName: "a.mp4", Size: size}, nil
		},
		open,
		func(context.Context, int64, string) (*Entry, error) {
			return &Entry{Path: "/媒体/a.mp4", Size: size, ContentType: "video/mp4", Modified: 1700000000000}, nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	if err := proxy.Start(); err != nil {
		t.Fatalf("start proxy: %v", err)
	}
	t.Cleanup(func() { _ = proxy.Close() })
	return proxy
}

func TestMediaProxyForwardsRange(t *testing.T) {
	body := []byte("0123456789abcdefghij")
	var gotStart, gotLength int64
	proxy := startTestProxy(t, int64(len(body)), func(_ context.Context, sourceID int64, path string, start, length int64) (*RangeResponse, error) {
		gotStart, gotLength = start, length
		if sourceID != 3 || path != "/媒体/a.mp4" {
			return nil, fmt.Errorf("wrong media %d %q", sourceID, path)
		}
		end := start + length
		if end > int64(len(body)) {
			end = int64(len(body))
		}
		return &RangeResponse{
			Body:          io.NopCloser(strings.NewReader(string(body[start:end]))),
			StatusCode:    http.StatusPartialContent,
			ContentType:   "video/mp4",
			ContentLength: end - start,
			ContentRange:  fmt.Sprintf("bytes %d-%d/%d", start, end-1, len(body)),
			TotalSize:     int64(len(body)),
		}, nil
	})

	req, _ := http.NewRequest(http.MethodGet, proxy.URL(7), nil)
	req.Header.Set("Range", "bytes=2-4")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || string(got) != "234" {
		t.Errorf("response = %d %q", resp.StatusCode, got)
	}
	if gotStart != 2 || gotLength != 3 {
		t.Errorf("upstream range = %d/%d", gotStart, gotLength)
	}
	if resp.Header.Get("Content-Range") != "bytes 2-4/20" || resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Errorf("headers = %v", resp.Header)
	}
}

func TestMediaProxyHeadAndSecurity(t *testing.T) {
	proxy := startTestProxy(t, 4096, func(context.Context, int64, string, int64, int64) (*RangeResponse, error) {
		return nil, errors.New("HEAD should not open a body")
	})
	resp, err := http.Head(proxy.URL(7))
	if err != nil {
		t.Fatalf("HEAD: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength != 4096 || resp.Header.Get("Content-Type") != "video/mp4" {
		t.Errorf("HEAD = %d len=%d type=%q", resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"))
	}

	// Unknown id and a wrong token are both 404, never a clue about the other.
	for _, u := range []string{proxy.URL(8), strings.Replace(proxy.URL(7), proxy.token[:4], "dead", 1)} {
		r, err := http.Get(u)
		if err != nil {
			t.Fatalf("GET %s: %v", u, err)
		}
		_ = r.Body.Close()
		if r.StatusCode != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", u, r.StatusCode)
		}
	}
	req, _ := http.NewRequest(http.MethodPost, proxy.URL(7), nil)
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Body.Close()
	if r.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", r.StatusCode)
	}
}

func TestMediaProxyRangeFallbackAndInvalid(t *testing.T) {
	body := []byte("abcdefghij")
	proxy := startTestProxy(t, int64(len(body)), func(context.Context, int64, string, int64, int64) (*RangeResponse, error) {
		return &RangeResponse{
			Body:          io.NopCloser(strings.NewReader(string(body))),
			StatusCode:    http.StatusOK,
			ContentType:   "video/mp4",
			ContentLength: int64(len(body)),
			TotalSize:     int64(len(body)),
		}, nil
	})
	req, _ := http.NewRequest(http.MethodGet, proxy.URL(7), nil)
	req.Header.Set("Range", "bytes=2-4")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(got) != string(body) {
		t.Errorf("fallback = %d %q", resp.StatusCode, got)
	}

	// suffix and multi-range are rejected rather than guessed.
	for _, raw := range []string{"bytes=-5", "bytes=0-1,4-5", "items=0-1"} {
		req, _ := http.NewRequest(http.MethodGet, proxy.URL(7), nil)
		req.Header.Set("Range", raw)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
			t.Errorf("Range %q status = %d, want 416", raw, resp.StatusCode)
		}
	}
}

func TestMediaProxyRejectsBeforeStart(t *testing.T) {
	proxy, err := NewMediaProxy(
		func(int64) (MediaInfo, error) { return MediaInfo{}, nil },
		func(context.Context, int64, string, int64, int64) (*RangeResponse, error) { return nil, nil },
		func(context.Context, int64, string) (*Entry, error) { return nil, nil },
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := proxy.URL(1); got != "" {
		t.Errorf("URL before Start = %q, want empty", got)
	}
}
