//go:build integration

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"muse-app/backend/api"
	"muse-app/backend/video"
)

// The highlight half of stubVideoRepo. The phone only reads highlights, so these
// are the read paths the HTTP transport needs plus the conditional updates the
// service still goes through.
func (r *stubVideoRepo) CreateHighlight(h *video.Highlight) error {
	r.nextHighlightID++
	h.ID = r.nextHighlightID
	r.highlights = append(r.highlights, *h)
	return nil
}

func (r *stubVideoRepo) GetHighlight(id int64) (*video.Highlight, error) {
	for i := range r.highlights {
		if r.highlights[i].ID == id {
			return &r.highlights[i], nil
		}
	}
	return nil, video.ErrNotFound
}

func (r *stubVideoRepo) GetVideoHighlight(videoID, id int64) (*video.Highlight, error) {
	h, err := r.GetHighlight(id)
	if err != nil {
		return nil, err
	}
	if h.VideoID != videoID {
		return nil, video.ErrNotFound
	}
	return h, nil
}

func (r *stubVideoRepo) ListHighlights(videoID int64) ([]video.Highlight, error) {
	out := make([]video.Highlight, 0)
	for _, h := range r.highlights {
		if h.VideoID == videoID {
			out = append(out, h)
		}
	}
	return out, nil
}

func (r *stubVideoRepo) ListQueuedHighlightIDs() ([]int64, error) {
	out := make([]int64, 0)
	for _, h := range r.highlights {
		if h.Status == video.HighlightStatusQueued {
			out = append(out, h.ID)
		}
	}
	return out, nil
}

func (r *stubVideoRepo) ListHighlightMediaPaths() ([]string, error) {
	out := make([]string, 0, len(r.highlights))
	for _, h := range r.highlights {
		if h.MediaPath != "" {
			out = append(out, h.MediaPath)
		}
	}
	return out, nil
}

func (r *stubVideoRepo) ClaimHighlight(id int64) (bool, error) {
	h, err := r.GetHighlight(id)
	if err != nil || h.Status != video.HighlightStatusQueued {
		return false, nil
	}
	h.Status = video.HighlightStatusRunning
	return true, nil
}

func (r *stubVideoRepo) FinishHighlight(id int64, status, errMsg string, finishedAt int64) (bool, error) {
	h, err := r.GetHighlight(id)
	if err != nil {
		return false, nil
	}
	h.Status, h.Error, h.FinishedAt = status, errMsg, finishedAt
	return true, nil
}

func (r *stubVideoRepo) RetryHighlight(videoID, id int64) (bool, error) {
	h, err := r.GetVideoHighlight(videoID, id)
	if err != nil || h.Status != video.HighlightStatusFailed {
		return false, nil
	}
	h.Status, h.Error, h.FinishedAt = video.HighlightStatusQueued, "", 0
	return true, nil
}

func (r *stubVideoRepo) RecoverHighlights(maxInterruptions int) ([]int64, int, error) {
	failed := 0
	for i := range r.highlights {
		h := &r.highlights[i]
		if h.Status != video.HighlightStatusRunning {
			continue
		}
		h.Interruptions++
		if h.Interruptions >= maxInterruptions {
			h.Status = video.HighlightStatusFailed
			h.Error = "生成被多次中断"
			failed++
			continue
		}
		h.Status = video.HighlightStatusQueued
	}
	ids, err := r.ListQueuedHighlightIDs()
	return ids, failed, err
}

func (r *stubVideoRepo) DeleteHighlight(videoID, id int64) error {
	out := r.highlights[:0]
	for _, h := range r.highlights {
		if h.ID != id || h.VideoID != videoID {
			out = append(out, h)
		}
	}
	r.highlights = out
	return nil
}

// newHighlightServer starts a LAN server whose library already holds the given
// highlights, so the read-only transport can be exercised without a database.
func newHighlightServer(t *testing.T, highlights ...video.Highlight) (*Server, string, string, *stubVideoRepo) {
	t.Helper()
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "sample.mp4")
	if err := os.WriteFile(mediaPath, []byte("video bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &stubVideoRepo{
		byID:       map[int64]*video.Video{},
		all:        []video.Video{{ID: 1, Path: mediaPath, FileName: "sample.mp4", Status: video.StatusAvailable}},
		highlights: highlights,
	}
	repo.byID[1] = &repo.all[0]
	svc := video.NewService(repo, nil, dir, log, nil)
	lib := api.New(svc, log)

	srv := New(lib, dir, log)
	listenLoopback(srv)
	info, err := srv.Start(0)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv, dir, fmt.Sprintf("http://127.0.0.1:%d", info.Port), repo
}

// TestVideoHighlightsEndpoint: the phone reads every state and filters failed
// rows itself, so the transport must not hide them.
func TestVideoHighlightsEndpoint(t *testing.T) {
	_, _, base, _ := newHighlightServer(t,
		video.Highlight{ID: 1, VideoID: 1, Kind: video.HighlightKindMoment, StartMs: 1000, Status: video.HighlightStatusReady, MediaPath: "highlights/h-1-2-3.jpg"},
		video.Highlight{ID: 2, VideoID: 1, Kind: video.HighlightKindRange, StartMs: 2000, EndMs: 7000, Status: video.HighlightStatusFailed, Error: "生成精彩时刻失败：boom"},
		video.Highlight{ID: 3, VideoID: 9, Kind: video.HighlightKindMoment, StartMs: 0, Status: video.HighlightStatusReady},
	)

	res, err := http.Get(base + "/api/videos/1/highlights")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var list []video.Highlight
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %+v, want the favorite's two rows", list)
	}
	if list[0].Status != video.HighlightStatusReady || list[1].Status != video.HighlightStatusFailed {
		t.Errorf("statuses = %q/%q, want both states returned", list[0].Status, list[1].Status)
	}
	if list[1].Error == "" {
		t.Error("a failed row must carry its message so the client can explain it")
	}
}

// TestHighlightMediaRoute: the generated image is servable by its own name, and
// only by names the generator writes.
func TestHighlightMediaRoute(t *testing.T) {
	_, dataDir, base, _ := newHighlightServer(t)
	name := "h-1-1700000000000000000-1.jpg"
	path := filepath.Join(dataDir, video.HighlightsRelativeDir, name)
	want := []byte("jpeg bytes")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := http.Get(base + "/highlights/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, want) {
		t.Errorf("body = %q, want the generated file", body)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}

	for _, bad := range []string{"1.jpg", "h-1-2-3.png", "h-1-2-3.jpg.bak", "notes.txt"} {
		res, err := http.Get(base + "/highlights/" + bad)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode == http.StatusOK && bytes.Equal(body, want) {
			t.Errorf("%q was served, want it refused", bad)
		}
	}
}
