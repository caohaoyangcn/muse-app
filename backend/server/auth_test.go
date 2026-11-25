package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"muse-app/backend/api"
	"muse-app/backend/database"
	"muse-app/backend/video"
)

const testAccessKey = "abcdefghijklmnop"

// newAuthTestServer starts a real library over a tiny on-disk video and arms
// the server with key. It is the shape production runs in; only the listener is
// loopback so the test never touches the LAN.
func newAuthTestServer(t *testing.T, key string) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "sample.mp4")
	payload := []byte("0123456789")
	if err := os.WriteFile(mediaPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open(database.DriverName, "file:"+filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.DiscardHandler)
	repo := video.NewSQLiteRepository(db)
	lib := api.New(video.NewService(repo, nil, dir, log, nil), log)
	v := &video.Video{
		Path: mediaPath, FileName: "sample.mp4",
		FileSize: int64(len(payload)), Status: video.StatusAvailable,
	}
	if err := repo.Create(v); err != nil {
		t.Fatal(err)
	}

	srv := New(lib, dir, log)
	srv.SetAccessKey(key)
	listenLoopback(srv)
	info, err := srv.Start(0)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv, "http://127.0.0.1:" + strconv.Itoa(info.Port)
}

func getStatusAndBody(t *testing.T, url string, mutate func(*http.Request)) (int, string, http.Header) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(req)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(body), res.Header.Clone()
}

func postJSON(t *testing.T, url, body string) (*http.Response, string) {
	t.Helper()
	res, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res, string(raw)
}

// Every data route is closed without the key, including the media and generated
// image routes the browser addresses with plain <img> tags.
func TestLANAuthRejectsMissingKey(t *testing.T) {
	_, base := newAuthTestServer(t, testAccessKey)
	paths := []string{
		"/api/videos",
		"/api/trash",
		"/stream/video/1",
		"/thumbnails/1.jpg",
		"/covers/jav/1.jpg",
		"/avatars/abc.jpg",
		"/highlights/h-1-2-3.jpg",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			status, body, headers := getStatusAndBody(t, base+path, nil)
			if status != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", status)
			}
			if !strings.Contains(body, "需要访问口令") {
				t.Fatalf("body = %q, want the missing-key message", body)
			}
			if got := headers.Get("Cache-Control"); strings.HasPrefix(path, "/api/") && got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
			if got := headers.Get("Referrer-Policy"); got != "no-referrer" {
				t.Fatalf("Referrer-Policy = %q, want no-referrer", got)
			}
		})
	}
}

// The shell and its assets stay public so the login screen can render; client
// routes are the shell too and must not 401.
func TestLANAuthLeavesShellPublic(t *testing.T) {
	_, base := newAuthTestServer(t, testAccessKey)
	for _, path := range []string{"/", "/videos/1", "/assets/app.js"} {
		status, body, _ := getStatusAndBody(t, base+path, nil)
		if status != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, status)
		}
		if body == "" {
			t.Fatalf("%s: empty body", path)
		}
	}
}

// The login handshake: no key asked initially, a wrong key is refused, the
// right key installs an HttpOnly session cookie and returns the key the SPA
// needs for external-player URLs.
func TestLANAuthSessionHandshake(t *testing.T) {
	_, base := newAuthTestServer(t, testAccessKey)

	status, _, _ := getStatusAndBody(t, base+sessionPath, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous session status = %d, want 401", status)
	}

	res, body := postJSON(t, base+sessionPath, `{"key":"wrong-key"}`)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong key status = %d, body = %s", res.StatusCode, body)
	}

	res, body = postJSON(t, base+sessionPath, `{"key":"ABCD-EFGH-IJKL-MNOP"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", res.StatusCode, body)
	}
	var login struct {
		Authenticated bool   `json:"authenticated"`
		Key           string `json:"key"`
	}
	if err := json.Unmarshal([]byte(body), &login); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if !login.Authenticated || login.Key != testAccessKey {
		t.Fatalf("login payload = %+v", login)
	}

	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == sessionCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("login did not set the session cookie")
	}
	if !cookie.HttpOnly {
		t.Error("session cookie is not HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", cookie.SameSite)
	}
	if cookie.MaxAge <= 0 {
		t.Errorf("MaxAge = %d, want a persisted session", cookie.MaxAge)
	}

	// The cookie authenticates a data route (the handler is unknown here, so a
	// 404 proves the middleware let the request through).
	status, _, _ = getStatusAndBody(t, base+"/api/unknown", func(r *http.Request) {
		r.AddCookie(cookie)
	})
	if status != http.StatusNotFound {
		t.Fatalf("authenticated unknown route = %d, want 404", status)
	}

	// The handshake itself returns the key to an authenticated caller.
	status, body, _ = getStatusAndBody(t, base+sessionPath, func(r *http.Request) {
		r.AddCookie(cookie)
	})
	if status != http.StatusOK {
		t.Fatalf("session status = %d", status)
	}
	if !strings.Contains(body, testAccessKey) {
		t.Fatalf("session body = %q, want the key", body)
	}
}

// Scripts can use a Bearer token; a data route accepts it exactly like a cookie.
func TestLANAuthAcceptsBearer(t *testing.T) {
	_, base := newAuthTestServer(t, testAccessKey)
	status, _, _ := getStatusAndBody(t, base+"/api/unknown", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+testAccessKey)
	})
	if status != http.StatusNotFound {
		t.Fatalf("bearer request = %d, want 404 (authenticated)", status)
	}
	status, _, _ = getStatusAndBody(t, base+"/api/unknown", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer nope")
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong bearer = %d, want 401", status)
	}
}

// A native player cannot send a cookie or a header, so /stream/ accepts the key
// in the query string. No other route does.
func TestLANAuthQueryKeyOnlyForStream(t *testing.T) {
	_, base := newAuthTestServer(t, testAccessKey)

	status, body, _ := getStatusAndBody(t, base+"/stream/video/1?key="+testAccessKey, nil)
	if status != http.StatusOK {
		t.Fatalf("keyed stream = %d, body = %q", status, body)
	}
	if body != "0123456789" {
		t.Fatalf("stream body = %q, want the source bytes", body)
	}

	status, _, _ = getStatusAndBody(t, base+"/api/videos?key="+testAccessKey, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("keyed API route = %d, want 401 (query keys only work for streams)", status)
	}
}

// The extension route stays reachable without the key: it is read-only and is
// already restricted to loopback/LAN Hosts and JavDB origins.
func TestLANAuthLeavesExtensionCheckPublic(t *testing.T) {
	_, base := newAuthTestServer(t, testAccessKey)
	res, body := postJSON(t, base+extensionPath, `{"codes":[]}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("extension check = %d, body = %s", res.StatusCode, body)
	}
}

// Five wrong keys lock the client out for the window, even if the right key
// arrives; a login from a fresh server is unaffected.
func TestLANAuthThrottlesFailedLogins(t *testing.T) {
	_, base := newAuthTestServer(t, testAccessKey)
	for i := 0; i < loginMaxFailures; i++ {
		res, _ := postJSON(t, base+sessionPath, `{"key":"wrong"}`)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, res.StatusCode)
		}
	}
	res, body := postJSON(t, base+sessionPath, `{"key":"wrong"}`)
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("after %d failures = %d, want 429 (body %s)", loginMaxFailures, res.StatusCode, body)
	}
	if got := res.Header.Get("Retry-After"); got == "" {
		t.Error("429 response has no Retry-After")
	}

	res, _ = postJSON(t, base+sessionPath, `{"key":`+strconv.Quote(testAccessKey)+`}`)
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("correct key while throttled = %d, want 429", res.StatusCode)
	}
}

// Rotating the key invalidates every existing cookie immediately.
func TestLANAuthRotationInvalidatesCookies(t *testing.T) {
	srv, base := newAuthTestServer(t, testAccessKey)

	res, _ := postJSON(t, base+sessionPath, `{"key":`+strconv.Quote(testAccessKey)+`}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login = %d", res.StatusCode)
	}
	cookie := res.Cookies()[0]

	srv.SetAccessKey("zyxwvutsrqponmlk")

	status, _, _ := getStatusAndBody(t, base+"/api/unknown", func(r *http.Request) {
		r.AddCookie(cookie)
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("old cookie after rotation = %d, want 401", status)
	}
	status, _, _ = getStatusAndBody(t, base+"/api/unknown", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer zyxwvutsrqponmlk")
	})
	if status != http.StatusNotFound {
		t.Fatalf("new key after rotation = %d, want 404 (authenticated)", status)
	}
}

// A cross-site write is refused even when it carries a valid cookie: the Origin
// guard is the second lock behind SameSite=Lax.
func TestLANAuthRejectsCrossSiteWrites(t *testing.T) {
	_, base := newAuthTestServer(t, testAccessKey)
	req, err := http.NewRequest(http.MethodPost, base+"/api/maintenance/orphaned-images", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testAccessKey)
	req.Header.Set("Origin", "http://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site write = %d, want 403", res.StatusCode)
	}
}
