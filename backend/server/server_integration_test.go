//go:build integration

package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"muse-app/backend/api"
	"muse-app/backend/group"
	"muse-app/backend/jav"
	"muse-app/backend/video"
)

// stubVideoRepo is a minimal in-memory VideoRepository for wiring a real
// video.Service without a database.
type stubVideoRepo struct {
	byID map[int64]*video.Video
	all  []video.Video
	// contentIgnores stores content-duplicate dismissals for the transport
	// tests, so "keep all" round-trips over HTTP.
	contentIgnores map[string]string
}

func (r *stubVideoRepo) Create(v *video.Video) error {
	v.ID = int64(len(r.all))
	r.all = append(r.all, *v)
	r.byID[v.ID] = &r.all[len(r.all)-1]
	return nil
}
func (r *stubVideoRepo) Get(id int64) (*video.Video, error) {
	if v, ok := r.byID[id]; ok {
		return v, nil
	}
	return nil, video.ErrNotFound
}
func (r *stubVideoRepo) GetByPath(path string) (*video.Video, error) {
	for i := range r.all {
		if r.all[i].Path == path {
			return &r.all[i], nil
		}
	}
	return nil, video.ErrNotFound
}
func (r *stubVideoRepo) List() ([]video.Video, error) { return r.all, nil }
func (r *stubVideoRepo) Search(keyword string) ([]video.Video, error) {
	return r.all, nil
}
func (r *stubVideoRepo) Delete(id int64) error                 { return nil }
func (r *stubVideoRepo) UpdateStatus(id int64, s string) error { return nil }
func (r *stubVideoRepo) UpdateMetadata(id int64, d int64, w, h int, fr float64, br int64, vc, ac string) error {
	return nil
}
func (r *stubVideoRepo) UpdateThumbnail(id int64, p string) error               { return nil }
func (r *stubVideoRepo) UpdateThumbnailIfAuto(id int64, p string) (bool, error) { return true, nil }
func (r *stubVideoRepo) SetScrapedCover(id int64, p string) error               { return nil }
func (r *stubVideoRepo) SetScrapedCoverAllowed(id int64, allowed bool) error    { return nil }
func (r *stubVideoRepo) ApplyScrapedCoverIfAllowed(id int64, p string) (bool, error) {
	return true, nil
}
func (r *stubVideoRepo) SetCoverSource(id int64, source string) error   { return nil }
func (r *stubVideoRepo) UpdateLastOpened(id int64, t int64) error       { return nil }
func (r *stubVideoRepo) ListNeedingMetadata() ([]int64, error)          { return nil, nil }
func (r *stubVideoRepo) UpdateInfo(id int64, title, notes string) error { return nil }
func (r *stubVideoRepo) SetCustomCover(id int64, at int64) error        { return nil }
func (r *stubVideoRepo) ClearCustomCover(id int64) error                { return nil }
func (r *stubVideoRepo) IncrementLikes(id int64) (int64, error)         { return 0, nil }
func (r *stubVideoRepo) ClearLikes(id int64) error                      { return nil }
func (r *stubVideoRepo) UpdateFingerprint(id int64, hash string, version int, size, mtime, at int64) error {
	return nil
}
func (r *stubVideoRepo) UpdateFingerprintFailure(id int64, message string, at int64) error {
	return nil
}
func (r *stubVideoRepo) UpdateFullHash(id int64, hash string, at int64) error {
	v, err := r.Get(id)
	if err != nil {
		return err
	}
	v.HashState.FullHash = hash
	v.HashState.FullHashAt = at
	return nil
}
func (r *stubVideoRepo) ListNeedingFingerprint(version int) ([]int64, error) { return nil, nil }
func (r *stubVideoRepo) ListSameSizePeers(size int64, excludeID int64) ([]int64, error) {
	return nil, nil
}
func (r *stubVideoRepo) ListRelocationCandidates(size int64, version int) ([]video.Video, error) {
	return nil, nil
}
func (r *stubVideoRepo) RelinkPath(id int64, path, fileName, hash string, version int, size, mtime, at int64) error {
	v, err := r.Get(id)
	if err != nil {
		return err
	}
	v.Path = path
	v.FileName = fileName
	v.Status = video.StatusAvailable
	v.ContentHash = hash
	v.HashVersion = version
	v.HashState.Size = size
	v.HashState.Mtime = mtime
	v.HashState.At = at
	return nil
}
func (r *stubVideoRepo) ListFingerprinted() ([]video.Video, error) {
	out := make([]video.Video, 0, len(r.all))
	for i := range r.all {
		if r.all[i].ContentHash != "" && r.all[i].Status == video.StatusAvailable {
			out = append(out, r.all[i])
		}
	}
	return out, nil
}
func (r *stubVideoRepo) ListByContentHash(hash string) ([]video.Video, error) {
	out := make([]video.Video, 0, 2)
	for i := range r.all {
		if r.all[i].ContentHash == hash && r.all[i].Status == video.StatusAvailable {
			out = append(out, r.all[i])
		}
	}
	return out, nil
}
func (r *stubVideoRepo) ListContentDuplicateIgnores() ([]video.IgnoredContentDuplicate, error) {
	out := make([]video.IgnoredContentDuplicate, 0, len(r.contentIgnores))
	for key, ids := range r.contentIgnores {
		out = append(out, video.IgnoredContentDuplicate{Key: key, VideoIDs: ids})
	}
	return out, nil
}
func (r *stubVideoRepo) UpsertContentDuplicateIgnore(key, videoIDs string) error {
	if r.contentIgnores == nil {
		r.contentIgnores = map[string]string{}
	}
	r.contentIgnores[key] = videoIDs
	return nil
}
func (r *stubVideoRepo) DeleteContentDuplicateIgnore(key string) error {
	delete(r.contentIgnores, key)
	return nil
}
func (r *stubVideoRepo) ListVideoIDs() ([]int64, error) {
	ids := make([]int64, 0, len(r.all))
	for _, v := range r.all {
		ids = append(ids, v.ID)
	}
	return ids, nil
}

// stubGroupRepo satisfies group.GroupRepository minimally.
type stubGroupRepo struct{}

func (r *stubGroupRepo) Create(name string, pos int, parentID *int64, at int64) (*group.Group, error) {
	return &group.Group{ID: 1, Name: name, ParentID: parentID}, nil
}
func (r *stubGroupRepo) Get(id int64) (*group.Group, error) {
	if id == 7 { // distinct id used by an add-to-group test
		return &group.Group{ID: 7, Name: "g7"}, nil
	}
	return &group.Group{ID: id, Name: "g"}, nil
}
func (r *stubGroupRepo) FindByName(name string) (*group.Group, error) {
	return &group.Group{ID: 1, Name: name}, nil
}
func (r *stubGroupRepo) List() ([]group.Group, error)                 { return nil, nil }
func (r *stubGroupRepo) Rename(id int64, name string, at int64) error { return nil }
func (r *stubGroupRepo) Delete(id int64) error                        { return nil }
func (r *stubGroupRepo) SetPositions(ids []int64) error               { return nil }
func (r *stubGroupRepo) SetParent(id int64, parentID *int64, at int64) error {
	return nil
}
func (r *stubGroupRepo) Children(parentID int64) ([]int64, error) { return nil, nil }
func (r *stubGroupRepo) VideosInGroups(groupIDs []int64) ([]int64, error) {
	return nil, nil
}
func (r *stubGroupRepo) Members(videoID int64) ([]int64, error)       { return nil, nil }
func (r *stubGroupRepo) SetMembers(videoID int64, ids []int64) error  { return nil }
func (r *stubGroupRepo) AddVideoToGroup(videoID, groupID int64) error { return nil }
func (r *stubGroupRepo) VideosInGroup(groupID int64) ([]int64, error) { return nil, nil }
func (r *stubGroupRepo) ClearVideoMembers(videoID int64) error        { return nil }

func newTestServer(t *testing.T) (*Server, string /*dataDir*/, string /*base url*/) {
	t.Helper()
	dir := t.TempDir()

	// A small real video file to stream.
	payload := bytes.Repeat([]byte("0123456789"), 100) // 1000 bytes
	mediaPath := filepath.Join(dir, "sample.mp4")
	if err := os.WriteFile(mediaPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	repo := &stubVideoRepo{
		byID: map[int64]*video.Video{},
		all: []video.Video{
			{
				ID: 1, Path: mediaPath, FileName: "sample.mp4",
				FileSize: int64(len(payload)), Status: video.StatusAvailable,
			},
		},
	}
	// Wire the map used by Get() to the slice element the test relies on.
	repo.byID[1] = &repo.all[0]
	svc := video.NewService(repo, nil, dir, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	grepo := &stubGroupRepo{}
	g := group.NewService(grepo, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	lib := api.New(svc, g, slog.New(slog.NewTextHandler(io.Discard, nil)))

	srv := New(lib, dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	listenLoopback(srv)
	info, err := srv.Start(0) // free port
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	// Use the loopback URL for the test client.
	return srv, dir, fmt.Sprintf("http://127.0.0.1:%d", info.Port)
}

// listenLoopback keeps a test server off every network interface and out of the
// host firewall. Binding 0.0.0.0 makes Windows Firewall ask whether to allow the
// freshly built test binary — once per run, because every build lands in a new
// temp path — and a real netsh call would edit the developer's rules. The tests
// only ever talk to 127.0.0.1.
func listenLoopback(srv *Server) {
	srv.listen = func(string, string) (net.Listener, error) {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	srv.firewall = nil
}

// newMetadataServer starts a server holding a single favorite with the given
// metadata. It returns the loopback base URL.
func newMetadataServer(t *testing.T, dataDir string, v video.Video) string {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &stubVideoRepo{
		byID: map[int64]*video.Video{},
		all:  []video.Video{v},
	}
	repo.byID[v.ID] = &repo.all[0]
	svc := video.NewService(repo, nil, dataDir, log, nil)
	gsvc := group.NewService(&stubGroupRepo{}, log, nil)
	lib := api.New(svc, gsvc, log)

	srv := New(lib, dataDir, log)
	listenLoopback(srv)
	info, err := srv.Start(0)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return fmt.Sprintf("http://127.0.0.1:%d", info.Port)
}

func TestServeIndex(t *testing.T) {
	_, _, base := newTestServer(t)
	res, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !bytes.Contains([]byte(ct), []byte("html")) {
		t.Errorf("content-type = %q, want html", ct)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Contains(body, []byte("Muse Video")) {
		t.Error("index does not mention Muse Video")
	}
}

func TestServeIndexReferencesBuiltAssets(t *testing.T) {
	_, _, base := newTestServer(t)
	res, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)

	// The mobile SPA is a Vite build whose HTML is generated. Pin the contract
	// the Go static handler and the browser both depend on: index.html must
	// reference the unhashed asset paths. A build that emitted content-hashed
	// filenames (assets/app-<hash>.js) would 404 here.
	for _, want := range []string{"/assets/app.js", "/assets/app.css"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("index.html does not reference %s", want)
		}
	}
	// The mobile client is a LAN web app: it must declare a viewport or phones
	// render it at desktop width.
	if !bytes.Contains(body, []byte("width=device-width")) {
		t.Error("index.html is missing the device-width viewport meta tag")
	}
}

func TestServeWebAssets(t *testing.T) {
	_, _, base := newTestServer(t)
	cases := []struct {
		path   string
		ctPart string
		// content must appear in the asset; blank skips the check. The API
		// prefix is the load-bearing one: the bundle URLs are built from this
		// constant with template literals, so the full paths are not literals.
		content string
	}{
		// The JS bundle must talk to the JSON API, not the Wails bindings.
		{"/assets/app.js", "javascript", `/api`},
		{"/assets/app.css", "css", ""},
	}
	for _, c := range cases {
		res, err := http.Get(base + c.path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s status = %d, want 200", c.path, res.StatusCode)
			continue
		}
		if len(body) == 0 {
			t.Errorf("%s is empty", c.path)
		}
		if ct := res.Header.Get("Content-Type"); !bytes.Contains([]byte(ct), []byte(c.ctPart)) {
			t.Errorf("%s content-type = %q, want to contain %q", c.path, ct, c.ctPart)
		}
		if c.content != "" && !bytes.Contains(body, []byte(c.content)) {
			t.Errorf("%s does not reference %q", c.path, c.content)
		}
	}

	// The mobile bundle must be the HTTP client. Wails bindings only exist in
	// the desktop WebView, so shipping them here would break every request.
	res, err := http.Get(base + "/assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if bytes.Contains(body, []byte("window.go")) || bytes.Contains(body, []byte("window['go']")) {
		t.Error("app.js references Wails bindings; the mobile client must use the HTTP API")
	}
}

func TestServeSpaFallback(t *testing.T) {
	// Deep link that isn't a file must fall back to index.html (SPA routing).
	_, _, base := newTestServer(t)
	res, err := http.Get(base + "/play/123")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !bytes.Contains([]byte(ct), []byte("html")) {
		t.Errorf("content-type = %q, want html", ct)
	}
}

func TestAPIVideos(t *testing.T) {
	_, _, base := newTestServer(t)
	res, err := http.Get(base + "/api/videos")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var videos []video.Video
	if err := json.NewDecoder(res.Body).Decode(&videos); err != nil {
		t.Fatal(err)
	}
	if len(videos) != 1 {
		t.Fatalf("got %d videos, want 1", len(videos))
	}
	if videos[0].FileName != "sample.mp4" {
		t.Errorf("fileName = %q", videos[0].FileName)
	}
}

func TestStreamWholeFile(t *testing.T) {
	_, _, base := newTestServer(t)
	res, err := http.Get(base + "/stream/video/1")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("content-type = %q, want video/mp4", ct)
	}
	if ar := res.Header.Get("Accept-Ranges"); ar != "bytes" {
		t.Errorf("accept-ranges = %q, want bytes", ar)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 1000 {
		t.Errorf("body len = %d, want 1000", len(body))
	}
}

func TestStreamRange(t *testing.T) {
	_, _, base := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, base+"/stream/video/1", nil)
	req.Header.Set("Range", "bytes=100-199")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", res.StatusCode)
	}
	if cr := res.Header.Get("Content-Range"); cr != "bytes 100-199/1000" {
		t.Errorf("content-range = %q", cr)
	}
	body, _ := io.ReadAll(res.Body)
	if len(body) != 100 {
		t.Errorf("body len = %d, want 100", len(body))
	}
	if string(body) != string(bytes.Repeat([]byte("0123456789"), 10)) {
		t.Error("range body content mismatch")
	}
}

func TestStreamMissingVideo(t *testing.T) {
	_, _, base := newTestServer(t)
	res, err := http.Get(base + "/stream/video/999")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
}

func TestThumbnail(t *testing.T) {
	_, dataDir, base := newTestServer(t)
	// Thumbnails live under <dataDir>/thumbnails/<id>.jpg.
	thumbDir := filepath.Join(dataDir, "thumbnails")
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("fake-jpeg-bytes")
	if err := os.WriteFile(filepath.Join(thumbDir, "8.jpg"), want, 0o644); err != nil {
		t.Fatal(err)
	}

	// The regression: `/thumbnails/8.jpg` must not fail ID parsing (the ".jpg"
	// suffix previously produced "无效的 ID"). It must serve the JPEG.
	res, err := http.Get(base + "/thumbnails/8.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, body=%s", res.StatusCode, body)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, want) {
		t.Errorf("thumbnail body mismatch")
	}
}

// The maintenance endpoint removes generated images no favorite refers to and
// keeps the ones that do.
func TestSweepOrphanedImages(t *testing.T) {
	_, dataDir, base := newTestServer(t)
	// The only favorite in this fixture is id 1.
	thumbDir := filepath.Join(dataDir, "thumbnails")
	coverDir := filepath.Join(dataDir, "covers", "jav")
	for _, dir := range []string{thumbDir, coverDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	live := filepath.Join(thumbDir, "1.jpg")
	orphanThumb := filepath.Join(thumbDir, "42.jpg")
	orphanCover := filepath.Join(coverDir, "43.jpg")
	for p, content := range map[string]string{live: "x", orphanThumb: "x", orphanCover: "x"} {
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := http.Post(base+"/api/maintenance/orphaned-images", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, body=%s", res.StatusCode, body)
	}
	var result video.ImageSweepResult
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Removed != 2 {
		t.Errorf("removed = %d, want 2", result.Removed)
	}
	if _, err := os.Stat(live); err != nil {
		t.Errorf("the live thumbnail must survive: %v", err)
	}
	for _, p := range []string{orphanThumb, orphanCover} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should be gone, stat err = %v", p, err)
		}
	}
}

// The 文件检测 endpoints start the file-existence pass and report its state.
// They are the LAN face of the desktop's startup check: a headless deployment
// gets the same answer without a restart.
func TestFileScanRoutes(t *testing.T) {
	_, _, base := newTestServer(t)

	res, body := doJSON(t, http.MethodPost, base+"/api/maintenance/files/scan", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("scan start status = %d, want 200 (body=%s)", res.StatusCode, body)
	}
	var started video.FileScanStatus
	if err := json.Unmarshal(body, &started); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	if started.StartedAt == 0 || !started.Running {
		t.Errorf("started = %+v, want a running pass with a start time", started)
	}

	status := waitForFileScan(t, base)
	// The single fixture favorite's file exists, so the pass checked it and
	// found nothing to clean up.
	if status.Total != 1 || status.Checked != 1 {
		t.Errorf("pass checked %d/%d files, want 1/1", status.Checked, status.Total)
	}
	if status.Removed != 0 || status.Skipped != 0 || status.Error != "" {
		t.Errorf("status = %+v, want a clean pass", status)
	}
}

// waitForFileScan follows the status endpoint until the pass finishes, which is
// how a client is expected to use it.
func waitForFileScan(t *testing.T, base string) video.FileScanStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		res, err := http.Get(base + "/api/maintenance/files/scan")
		if err != nil {
			t.Fatal(err)
		}
		var status video.FileScanStatus
		err = json.NewDecoder(res.Body).Decode(&status)
		res.Body.Close()
		if err != nil {
			t.Fatalf("decode status: %v", err)
		}
		if !status.Running {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatal("the file scan did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestJavCover(t *testing.T) {
	_, dataDir, base := newTestServer(t)
	coverDir := filepath.Join(dataDir, "covers", "jav")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("fake-jav-cover-bytes")
	if err := os.WriteFile(filepath.Join(coverDir, "8.jpg"), want, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := http.Get(base + "/covers/jav/8.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, body=%s", res.StatusCode, body)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, want) {
		t.Errorf("JAV cover body mismatch")
	}
}

// Actor avatars are addressed by the provider's performer id; the LAN client
// only ever sees the file name the scraper wrote.
func TestActorAvatar(t *testing.T) {
	_, dataDir, base := newTestServer(t)
	avatarDir := filepath.Join(dataDir, "avatars")
	if err := os.MkdirAll(avatarDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("fake-actor-avatar-bytes")
	if err := os.WriteFile(filepath.Join(avatarDir, "nxyzzbk.jpg"), want, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := http.Get(base + "/avatars/nxyzzbk.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, want) {
		t.Errorf("actor avatar body mismatch")
	}

	// A name the scraper could never have produced is not a path.
	for _, path := range []string{"/avatars/nxyzzbk.png", "/avatars/.jpg"} {
		bad, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = bad.Body.Close()
		if bad.StatusCode == http.StatusOK {
			t.Fatalf("%s served a file it should not have", path)
		}
	}
}

func doJSON(t *testing.T, method, url, payload string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader([]byte(payload)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, b
}

// API write endpoints are reachable over HTTP (the shared façade), but the web
// UI deliberately exposes no controls for them. These tests pin the routes so a
// future change to the façade can't silently break a capability client.
func TestAPIWrites(t *testing.T) {
	_, _, base := newTestServer(t)

	// Create group → 200 + JSON body.
	res, body := doJSON(t, http.MethodPost, base+"/api/groups", `{"name":"旅行"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create group status = %d, body=%s", res.StatusCode, body)
	}
	var g group.Group
	if err := json.Unmarshal(body, &g); err != nil {
		t.Fatalf("create group JSON: %v (%s)", err, body)
	}
	if g.Name != "旅行" {
		t.Errorf("group name = %q, want 旅行", g.Name)
	}

	// Create group with empty/invalid body → 400.
	res, body = doJSON(t, http.MethodPost, base+"/api/groups", `{bad json`)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("bad json status = %d, want 400 (body=%s)", res.StatusCode, body)
	}
	res, body = doJSON(t, http.MethodPost, base+"/api/groups", `{}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("empty-name create status = %d, want 400 (body=%s)", res.StatusCode, body)
	}

	// Add video to group with invalid videoId → 400.
	res, body = doJSON(t, http.MethodPost, base+"/api/groups/7/videos", `{"videoId":0}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("add-video status = %d, want 400 (body=%s)", res.StatusCode, body)
	}

	// Remove a video → 204 (stub Remove is a no-op but must go through the façade).
	res, body = doJSON(t, http.MethodDelete, base+"/api/videos/1", "")
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("remove video status = %d, want 204 (body=%s)", res.StatusCode, body)
	}

	// Rename group → 200.
	res, body = doJSON(t, http.MethodPut, base+"/api/groups/1", `{"name":"新名"}`)
	if res.StatusCode != http.StatusOK {
		t.Errorf("rename status = %d, want 200 (body=%s)", res.StatusCode, body)
	}
}

// stubJavRepo is an in-memory jav.Repository. Only the read methods the LAN
// API touches carry data; the queue methods are inert stubs.
type stubJavRepo struct {
	detail   *jav.Detail
	task     *jav.Task
	refs     []jav.ActorFilmRef
	merges   []jav.ActorMerge
	actors   []int64
	profiles []jav.ActorProfile
	details  map[int64]*jav.Detail
}

func (r *stubJavRepo) Get(videoID int64) (*jav.Detail, error) {
	if r.details != nil {
		if d, ok := r.details[videoID]; ok {
			return d, nil
		}
		return nil, sql.ErrNoRows
	}
	if r.detail == nil || r.detail.VideoID != videoID {
		return nil, sql.ErrNoRows
	}
	return r.detail, nil
}
func (r *stubJavRepo) GetTask(videoID int64) (*jav.Task, error) {
	if r.task == nil || r.task.VideoID != videoID {
		return nil, sql.ErrNoRows
	}
	return r.task, nil
}
func (r *stubJavRepo) ListTaskOverviews() ([]jav.TaskOverview, error) {
	if r.task == nil {
		return nil, nil
	}
	return []jav.TaskOverview{{Task: *r.task, Category: jav.TaskCategory(r.task.Status)}}, nil
}
func (r *stubJavRepo) ListCodes() (map[int64]string, error) {
	codes := make(map[int64]string)
	for id, d := range r.details {
		if d != nil && d.Code != "" {
			codes[id] = d.Code
		}
	}
	if r.detail != nil && r.detail.Code != "" {
		codes[r.detail.VideoID] = r.detail.Code
	}
	return codes, nil
}
func (r *stubJavRepo) PrepareTask(int64, bool) error { return nil }
func (r *stubJavRepo) ClaimTask(int64, string, int64, int64) (*jav.Task, bool, error) {
	return nil, false, nil
}
func (r *stubJavRepo) SetTaskStatus(int64, string, string, string, string, *int64) error { return nil }
func (r *stubJavRepo) RecoverPending(int64) ([]jav.Task, error)                          { return nil, nil }
func (r *stubJavRepo) SaveSuccess(int64, jav.Metadata, string, int64) error              { return nil }
func (r *stubJavRepo) SetCover(int64, string) error                                      { return nil }
func (r *stubJavRepo) SetOverride(int64, string, string) error                           { return nil }
func (r *stubJavRepo) ListActorFilmRefs() ([]jav.ActorFilmRef, error)                    { return r.refs, nil }
func (r *stubJavRepo) ListActorMerges() ([]jav.ActorMerge, error)                        { return r.merges, nil }
func (r *stubJavRepo) UpsertActorMerge(string, string) error                             { return nil }
func (r *stubJavRepo) DeleteActorMerge(string) error                                     { return nil }
func (r *stubJavRepo) VideoIDsByActorNames([]string) ([]int64, error)                    { return r.actors, nil }
func (r *stubJavRepo) ListDuplicateCandidates() ([]jav.DuplicateCandidate, error)        { return nil, nil }
func (r *stubJavRepo) ListDuplicateIgnores() ([]jav.IgnoredDuplicate, error)             { return nil, nil }
func (r *stubJavRepo) UpsertDuplicateIgnore(string, string) error                        { return nil }
func (r *stubJavRepo) Delete(int64) error                                                { return nil }
func (r *stubJavRepo) DeleteDuplicateIgnore(string) error                                { return nil }
func (r *stubJavRepo) UpsertActorProfiles([]jav.ActorProfile) error                      { return nil }
func (r *stubJavRepo) ListActorProfiles() ([]jav.ActorProfile, error)                    { return r.profiles, nil }
func (r *stubJavRepo) SetActorAvatarPath(string, string) error                           { return nil }
func (r *stubJavRepo) EnqueueActorFills([]string) (int, error)                           { return 0, nil }
func (r *stubJavRepo) ClaimActorFill(int64) (*jav.ActorFillTask, bool, error)            { return nil, false, nil }
func (r *stubJavRepo) SetActorFillResult(string, string, string, string, *int64, *int64) error {
	return nil
}
func (r *stubJavRepo) ListActorFillTasks() ([]jav.ActorFillTask, error) { return nil, nil }
func (r *stubJavRepo) ResetActorFills() error                           { return nil }

// newJavTestServer starts a LAN server whose façade has a JAV service wired to
// an in-memory repository, so the metadata/actor/task endpoints can be
// exercised without a database or network provider.
func newJavTestServer(t *testing.T, repo *stubJavRepo) string {
	t.Helper()
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "sample.mp4")
	if err := os.WriteFile(mediaPath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	vrepo := &stubVideoRepo{
		byID: map[int64]*video.Video{},
		all:  []video.Video{{ID: 1, Path: mediaPath, FileName: "sample.mp4", FileSize: 5, Status: video.StatusAvailable}},
	}
	vrepo.byID[1] = &vrepo.all[0]
	svc := video.NewService(vrepo, nil, dir, log, nil)
	gsvc := group.NewService(&stubGroupRepo{}, log, nil)
	lib := api.New(svc, gsvc, log)
	javSvc := jav.NewService(repo, vrepo, nil, filepath.Join(dir, "covers", "jav"), nil, log, 1)
	t.Cleanup(javSvc.Close)
	lib.SetJavService(javSvc)

	srv := New(lib, dir, log)
	listenLoopback(srv)
	info, err := srv.Start(0)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return fmt.Sprintf("http://127.0.0.1:%d", info.Port)
}

func TestAPIGetVideo(t *testing.T) {
	_, _, base := newTestServer(t)
	res, err := http.Get(base + "/api/videos/1")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var v video.Video
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	if v.ID != 1 || v.FileName != "sample.mp4" {
		t.Errorf("video = %+v", v)
	}

	// Unknown id → 404, distinct from the JAV null case below.
	res, err = http.Get(base + "/api/videos/999")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown video status = %d, want 404", res.StatusCode)
	}
}

// The LAN API exposes the same display name rule as the desktop: a favorite
// with a saved scrape result carries its authoritative 番号 for clients to show.
func TestAPIListVideosCarriesScrapedCode(t *testing.T) {
	base := newJavTestServer(t, &stubJavRepo{
		details: map[int64]*jav.Detail{1: {VideoID: 1, Code: "ABC-123"}},
	})
	res, err := http.Get(base + "/api/videos")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var videos []video.Video
	if err := json.NewDecoder(res.Body).Decode(&videos); err != nil {
		t.Fatal(err)
	}
	if len(videos) != 1 || videos[0].JavCode != "ABC-123" {
		t.Fatalf("videos = %+v, want one carrying ABC-123", videos)
	}
}

func TestAPIJavDetailAndTask(t *testing.T) {
	repo := &stubJavRepo{
		details: map[int64]*jav.Detail{
			1: {VideoID: 1, Code: "ABC-123", Title: "示例", Actors: []string{"葵つかさ"}, Tags: []string{"高清"}},
		},
		task: &jav.Task{ID: 5, VideoID: 1, Status: jav.StatusSucceeded},
	}
	base := newJavTestServer(t, repo)

	res, err := http.Get(base + "/api/videos/1/jav")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("jav status = %d, want 200", res.StatusCode)
	}
	var detail jav.Detail
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Code != "ABC-123" || detail.Title != "示例" {
		t.Errorf("detail = %+v", detail)
	}

	res, err = http.Get(base + "/api/videos/1/jav/task")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("jav task status = %d, want 200", res.StatusCode)
	}
	var task jav.Task
	if err := json.NewDecoder(res.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.Status != jav.StatusSucceeded {
		t.Errorf("task = %+v", task)
	}
}

// An unscraped favorite must answer 200 with a null body rather than an error,
// so the mobile detail screen can render "not scraped yet" instead of treating
// it as a failure.
func TestAPIJavDetailNullWhenUnscraped(t *testing.T) {
	base := newJavTestServer(t, &stubJavRepo{})
	for _, path := range []string{"/api/videos/1/jav", "/api/videos/1/jav/task"} {
		res, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s status = %d, want 200 (body=%s)", path, res.StatusCode, body)
			continue
		}
		if trimmed := strings.TrimSpace(string(body)); trimmed != "null" {
			t.Errorf("%s body = %q, want null", path, trimmed)
		}
	}
}

func TestAPIJavTasks(t *testing.T) {
	repo := &stubJavRepo{task: &jav.Task{ID: 5, VideoID: 1, Status: jav.StatusQueued}}
	base := newJavTestServer(t, repo)

	res, err := http.Get(base + "/api/jav/tasks")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var overviews []jav.TaskOverview
	if err := json.NewDecoder(res.Body).Decode(&overviews); err != nil {
		t.Fatal(err)
	}
	if len(overviews) != 1 || overviews[0].Category != jav.TaskCategoryQueued {
		t.Errorf("overviews = %+v", overviews)
	}
}

func TestAPIActorsAndFilms(t *testing.T) {
	repo := &stubJavRepo{
		refs:   []jav.ActorFilmRef{{Name: "葵つかさ", VideoID: 1}},
		actors: []int64{1},
	}
	base := newJavTestServer(t, repo)

	res, err := http.Get(base + "/api/actors")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var actors []jav.ActorSummary
	if err := json.NewDecoder(res.Body).Decode(&actors); err != nil {
		t.Fatal(err)
	}
	if len(actors) != 1 || actors[0].Name != "葵つかさ" || actors[0].Count != 1 {
		t.Errorf("actors = %+v", actors)
	}

	// The actor name travels as a query parameter so CJK names (and names with
	// slashes) survive URL parsing.
	res, err = http.Get(base + "/api/actors/videos?name=" + url.QueryEscape("葵つかさ"))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("actor films status = %d, want 200", res.StatusCode)
	}
	var films []video.Video
	if err := json.NewDecoder(res.Body).Decode(&films); err != nil {
		t.Fatal(err)
	}
	if len(films) != 1 || films[0].ID != 1 {
		t.Errorf("films = %+v", films)
	}
}

func TestAPIEditInfoAndLikes(t *testing.T) {
	_, _, base := newTestServer(t)

	res, body := doJSON(t, http.MethodPut, base+"/api/videos/1/info", `{"title":"新标题","notes":"备注"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update info status = %d, want 200 (body=%s)", res.StatusCode, body)
	}
	var v video.Video
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("update info JSON: %v (%s)", err, body)
	}
	if v.ID != 1 {
		t.Errorf("updated video id = %d, want 1", v.ID)
	}

	// Unknown video → 400 with the façade's user-facing message.
	res, body = doJSON(t, http.MethodPut, base+"/api/videos/999/info", `{"title":"x"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("update missing video status = %d, want 400 (body=%s)", res.StatusCode, body)
	}

	res, body = doJSON(t, http.MethodPost, base+"/api/videos/1/likes", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("like status = %d, want 200 (body=%s)", res.StatusCode, body)
	}
	res, body = doJSON(t, http.MethodDelete, base+"/api/videos/1/likes", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("clear likes status = %d, want 200 (body=%s)", res.StatusCode, body)
	}

	// Invalid body → 400.
	res, body = doJSON(t, http.MethodPut, base+"/api/videos/1/info", `{bad json`)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("bad info body status = %d, want 400 (body=%s)", res.StatusCode, body)
	}
}

// newContentDedupServer starts a LAN server holding two favorites whose files
// are byte-identical, so the content-duplicate endpoints are exercised the way
// the mobile client calls them.
func newContentDedupServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	payload := bytes.Repeat([]byte("abcdefghij"), 100) // 1000 bytes, twice
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	vrepo := &stubVideoRepo{byID: map[int64]*video.Video{}}
	for i, name := range []string{"a.mp4", "b.mp4"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		v := video.Video{
			ID: int64(i + 1), Path: path, FileName: name,
			FileSize: info.Size(), Status: video.StatusAvailable,
			CreatedAt:   int64(i + 1),
			UpdatedAt:   int64(i + 1),
			ContentHash: "digest-one", HashVersion: video.FingerprintVersion,
			HashState: video.FingerprintState{Size: info.Size(), Mtime: info.ModTime().UnixNano()},
		}
		vrepo.all = append(vrepo.all, v)
	}
	for i := range vrepo.all {
		vrepo.byID[vrepo.all[i].ID] = &vrepo.all[i]
	}

	svc := video.NewService(vrepo, nil, dir, log, nil)
	fp := video.NewFingerprinter(vrepo, nil, log, 1)
	t.Cleanup(fp.Close)
	svc.SetFingerprinter(fp)

	lib := api.New(svc, group.NewService(&stubGroupRepo{}, log, nil), log)
	srv := New(lib, dir, log)
	listenLoopback(srv)
	info, err := srv.Start(0)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return fmt.Sprintf("http://127.0.0.1:%d", info.Port)
}

// TestAPIContentDuplicates covers the content worklist and its dismissal over
// HTTP, which is how the mobile client uses it.
func TestAPIContentDuplicates(t *testing.T) {
	base := newContentDedupServer(t)

	res, err := http.Get(base + "/api/duplicates/content")
	if err != nil {
		t.Fatal(err)
	}
	var groups []video.ContentDuplicateGroup
	if err := json.NewDecoder(res.Body).Decode(&groups); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(groups) != 1 {
		t.Fatalf("groups = %+v, want one", groups)
	}
	if groups[0].Hash != "digest-one" || len(groups[0].Members) != 2 {
		t.Fatalf("group = %+v, want digest-one with two members", groups[0])
	}
	if groups[0].Members[0].Video.FileName != "b.mp4" {
		t.Errorf("first member = %q, want the newest copy", groups[0].Members[0].Video.FileName)
	}

	res, body := doJSON(t, http.MethodPost, base+"/api/duplicates/content/ignore", `{"hash":"digest-one"}`)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("ignore status = %d, want 204 (body=%s)", res.StatusCode, body)
	}
	res, err = http.Get(base + "/api/duplicates/content")
	if err != nil {
		t.Fatal(err)
	}
	var visible []video.ContentDuplicateGroup
	if err := json.NewDecoder(res.Body).Decode(&visible); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(visible) != 0 {
		t.Fatalf("groups after dismissal = %+v, want none", visible)
	}
	res, err = http.Get(base + "/api/duplicates/content?includeIgnored=1")
	if err != nil {
		t.Fatal(err)
	}
	var withIgnored []video.ContentDuplicateGroup
	if err := json.NewDecoder(res.Body).Decode(&withIgnored); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(withIgnored) != 1 || !withIgnored[0].Ignored {
		t.Fatalf("groups with ignored = %+v, want the dismissed group", withIgnored)
	}

	// Re-verification and the passing scan both answer without a body.
	res, body = doJSON(t, http.MethodPost, base+"/api/duplicates/content/reverify", `{"hash":"digest-one"}`)
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("reverify status = %d, want 204 (body=%s)", res.StatusCode, body)
	}

	// Full-file confirmation is scheduled in the background (it reads whole
	// files), so the endpoint answers immediately and the group reports the
	// result once both copies were read.
	res, body = doJSON(t, http.MethodPost, base+"/api/duplicates/content/confirm", `{"hash":"digest-one"}`)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("confirm status = %d, want 204 (body=%s)", res.StatusCode, body)
	}
	if !waitForConfirmed(t, base) {
		t.Error("the group was not confirmed after the full read")
	}

	res, body = doJSON(t, http.MethodPost, base+"/api/duplicates/content/scan", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("scan status = %d, want 200 (body=%s)", res.StatusCode, body)
	}
	if res, err = http.Get(base + "/api/duplicates/content/scan"); err != nil {
		t.Fatal(err)
	} else if res.StatusCode != http.StatusOK {
		t.Errorf("scan status route = %d, want 200", res.StatusCode)
	}
	res.Body.Close()

	// A request without a digest is a client error, not a silent success.
	for _, path := range []string{
		"/api/duplicates/content/ignore",
		"/api/duplicates/content/reverify",
		"/api/duplicates/content/confirm",
	} {
		res, body = doJSON(t, http.MethodPost, base+path, `{}`)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s without hash status = %d, want 400 (body=%s)", path, res.StatusCode, body)
		}
	}
}

// waitForConfirmed polls the worklist until the group reports itself confirmed.
func waitForConfirmed(t *testing.T, base string) bool {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(base + "/api/duplicates/content?includeIgnored=1")
		if err != nil {
			t.Fatal(err)
		}
		var groups []video.ContentDuplicateGroup
		if err := json.NewDecoder(res.Body).Decode(&groups); err != nil {
			res.Body.Close()
			t.Fatal(err)
		}
		res.Body.Close()
		if len(groups) == 1 && groups[0].Confirmed {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// TestStreamOriginalCodec is the core contract after playback moved to native
// clients: even an MKV/HEVC source is delivered unchanged and with Range
// support, because the client owns decoding.
func TestStreamOriginalCodec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.mkv")
	want := []byte("original-mkv-bytes")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	base := newMetadataServer(t, dir, video.Video{
		ID: 1, Path: path, FileName: "movie.mkv", FileSize: int64(len(want)),
		Status: video.StatusAvailable, VideoCodec: "hevc", AudioCodec: "aac",
	})

	req, _ := http.NewRequest(http.MethodGet, base+"/stream/video/1", nil)
	req.Header.Set("Range", "bytes=0-8")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "video/x-matroska" {
		t.Errorf("content-type = %q, want video/x-matroska", ct)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, want[:9]) {
		t.Errorf("range body = %q, want original bytes %q", body, want[:9])
	}
}

// TestStreamStatusRouteRemoved ensures the old browser-preparation endpoint
// cannot be mistaken for an SPA route after the API deletion.
func TestStreamStatusRouteRemoved(t *testing.T) {
	_, _, base := newTestServer(t)
	res, err := http.Get(base + "/stream/video/1/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
}
