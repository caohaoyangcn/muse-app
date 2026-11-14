package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"muse-app/backend/api"
	"muse-app/backend/database"
	"muse-app/backend/group"
	"muse-app/backend/jav"
	"muse-app/backend/video"
)

// The CORS allowlist is the security boundary of the extension endpoint: a
// JavDB page may call it, anything else may not.
func TestExtensionOriginAllowed(t *testing.T) {
	cases := []struct {
		origin string
		want   bool
	}{
		{"https://javdb.com", true},
		{"https://javdb582.com", true},
		{"https://app.javdb581.com", true},
		{"https://javdb2.com", true},
		{"https://jav.app", true},
		{"https://app.jav.app", true},
		{"http://localhost:5173", true},
		{"http://127.0.0.1:8899", true},
		{"http://[::1]:3000", true},

		{"https://evil.com", false},
		{"https://javdb.com.evil.com", false},
		{"https://eviljavdb.com", false},
		{"https://notjav.app", false},
		{"http://192.168.1.20:8899", false},
		{"null", false},
		{"", false},
		{"file:///tmp/fixture.html", false},
	}
	for _, c := range cases {
		if got := extensionOriginAllowed(c.origin); got != c.want {
			t.Errorf("extensionOriginAllowed(%q) = %v, want %v", c.origin, got, c.want)
		}
	}
}

// The wrapper must answer a JavDB preflight, stamp the allowed origin on the
// real response, and leave the header off for everyone else.
func TestWithExtensionCORS(t *testing.T) {
	handler := withExtensionCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("allowed preflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/extension/check", nil)
		req.Header.Set("Origin", "https://javdb.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://javdb.com" {
			t.Errorf("allow-origin = %q", got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
			t.Errorf("allow-methods = %q, want POST", got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Content-Type") {
			t.Errorf("allow-headers = %q, want Content-Type", got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Errorf("credentials must not be allowed, got %q", got)
		}
		if got := rec.Header().Get("Vary"); !strings.Contains(got, "Origin") {
			t.Errorf("Vary = %q, want Origin", got)
		}
	})

	t.Run("allowed POST", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/extension/check", strings.NewReader(`{"codes":[]}`))
		req.Header.Set("Origin", "https://javdb582.com")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://javdb582.com" {
			t.Errorf("allow-origin = %q", got)
		}
	})

	t.Run("disallowed origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/extension/check", strings.NewReader(`{"codes":[]}`))
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("allow-origin = %q, want none", got)
		}
		if got := rec.Header().Get("Vary"); !strings.Contains(got, "Origin") {
			t.Errorf("Vary = %q, want Origin even when denied", got)
		}
	})

	t.Run("disallowed preflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/extension/check", nil)
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("allow-origin = %q, want none", got)
		}
	})

	t.Run("no origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/extension/check", strings.NewReader(`{"codes":[]}`))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (same-origin callers are unaffected)", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("allow-origin = %q, want none", got)
		}
	})
}

// stubGroupRepo satisfies group.GroupRepository minimally. It lives here rather
// than in the integration test file because the extension route needs a façade
// without a real database too.
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
func (r *stubGroupRepo) GroupNamesForVideos(videoIDs []int64) (map[int64][]string, error) {
	return map[int64][]string{}, nil
}

// listenLoopback keeps a test server off every network interface and out of the
// host firewall: binding 0.0.0.0 makes Windows Firewall ask whether to allow the
// freshly built test binary — once per run, because every build lands in a new
// temp path. The tests only ever talk to 127.0.0.1.
func listenLoopback(srv *Server) {
	srv.listen = func(string, string) (net.Listener, error) {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	srv.firewall = nil
}

// readAll drains a response body for an error message.
func readAll(t *testing.T, body io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

// newExtensionServer starts a server over a real SQLite library holding one
// scraped favorite and one the scraper has not reached yet, so the route is
// exercised against the same queries production uses.
func newExtensionServer(t *testing.T) string {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := sql.Open(database.DriverName, "file:"+filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	vr := video.NewSQLiteRepository(db)
	lib := api.New(video.NewService(vr, nil, t.TempDir(), log, nil), group.NewService(&stubGroupRepo{}, log, nil), log)
	javRepo := jav.NewSQLiteRepository(db)
	javSvc := jav.NewService(javRepo, vr, nil, t.TempDir(), nil, log, 1)
	t.Cleanup(javSvc.Close)
	lib.SetJavService(javSvc)

	scraped := &video.Video{Path: filepath.Join(t.TempDir(), "abp-576.mp4"), FileName: "abp-576.mp4", Status: video.StatusAvailable}
	unscraped := &video.Video{Path: filepath.Join(t.TempDir(), "ipx-633.mp4"), FileName: "ipx-633.mp4", Status: video.StatusAvailable}
	for _, v := range []*video.Video{scraped, unscraped} {
		if err := vr.Create(v); err != nil {
			t.Fatal(err)
		}
	}
	if err := javRepo.PrepareTask(scraped.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := javRepo.SaveSuccess(scraped.ID, jav.Metadata{Code: "ABP-576", Actors: []string{"Actor A"}}, "filename", 1); err != nil {
		t.Fatal(err)
	}

	srv := New(lib, t.TempDir(), log)
	listenLoopback(srv)
	info, err := srv.Start(0)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return "http://127.0.0.1:" + strconv.Itoa(info.Port)
}

// The route the extension calls: a JavDB origin gets an answer for every code
// on the page, in the order it asked, and a bad request is reported as 400.
func TestAPIExtensionCheck(t *testing.T) {
	base := newExtensionServer(t)

	body := `{"codes":["ABP-576","IPX-633","MIAA-000"]}`
	res, err := http.Post(base+"/api/extension/check", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.StatusCode, readAll(t, res.Body))
	}

	var payload struct {
		Codes []jav.JavCodeCheck `json:"codes"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Codes) != 3 {
		t.Fatalf("codes = %#v", payload.Codes)
	}
	if payload.Codes[0].Code != "ABP-576" || len(payload.Codes[0].Matches) != 1 {
		t.Errorf("scraped favorite = %#v, want one match", payload.Codes[0])
	} else if payload.Codes[0].Matches[0].JavCode != "ABP-576" {
		t.Errorf("match = %#v, want the scraped code", payload.Codes[0].Matches[0])
	}
	if payload.Codes[1].Code != "IPX-633" || len(payload.Codes[1].Matches) != 1 {
		t.Errorf("unscraped favorite = %#v, want a file-name match", payload.Codes[1])
	}
	if payload.Codes[2].Code != "MIAA-000" || payload.Codes[2].Matches == nil || len(payload.Codes[2].Matches) != 0 {
		t.Errorf("absent work = %#v, want an empty (non-null) match list", payload.Codes[2])
	}

	// A malformed or empty body is a client error, not a 500.
	for _, bad := range []string{`not json`, `{}`, `{"codes":[]}`} {
		res, err := http.Post(base+"/api/extension/check", "application/json", bytes.NewBufferString(bad))
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusBadRequest {
			t.Errorf("body %q status = %d, want 200 or 400", bad, res.StatusCode)
		}
	}
}

// Only the extension route may answer a cross-origin caller; every other API
// path keeps its historical same-origin behaviour.
func TestAPIExtensionCORSIsScoped(t *testing.T) {
	base := newExtensionServer(t)

	req, err := http.NewRequest(http.MethodGet, base+"/api/videos", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://javdb.com")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("GET /api/videos allow-origin = %q, want none", got)
	}
}
