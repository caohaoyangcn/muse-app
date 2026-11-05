package cloud

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"muse-app/backend/database"
)

func TestServeVideoForwardsRange(t *testing.T) {
	dav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=2-4" {
			t.Errorf("upstream Range = %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 2-4/10")
		w.Header().Set("Content-Length", "3")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "234")
	}))
	defer dav.Close()

	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := NewService(NewSQLiteRepository(db), nil, nil)
	conn, err := svc.SaveConnection(ConnectionInput{Name: "dav", Endpoint: dav.URL + "/dav", RootPath: "/", VerifyTLS: true})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/stream/video/1", nil)
	req.Header.Set("Range", "bytes=2-4")
	rec := httptest.NewRecorder()
	svc.ServeVideo(rec, req, conn.ID, "/媒体/a.mp4", "a.mp4")
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "234" {
		t.Errorf("response = %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Range") != "bytes 2-4/10" || rec.Header().Get("Accept-Ranges") != "bytes" {
		t.Errorf("headers = %v", rec.Header())
	}
}

func TestServeVideoMapsUnreachable(t *testing.T) {
	dav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer dav.Close()
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := NewService(NewSQLiteRepository(db), nil, nil)
	conn, err := svc.SaveConnection(ConnectionInput{Name: "dav", Endpoint: dav.URL + "/dav", RootPath: "/", VerifyTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/stream/video/1", nil)
	rec := httptest.NewRecorder()
	svc.ServeVideo(rec, req, conn.ID, "/a.mp4", "a.mp4")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "网盘未连接") {
		t.Errorf("body = %q", rec.Body.String())
	}
}
