//go:build integration

package cloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/net/webdav"

	"muse-app/backend/database"
	"muse-app/backend/thumbnail"
	"muse-app/backend/video"
)

// TestLocalWebDAVEndToEnd exercises the real stack the unit tests mock:
// x/net/webdav serves an ffmpeg-generated file, the loopback media proxy reads
// it with Range, ffprobe extracts metadata and ffmpeg renders a cover. It is
// the local stand-in for OpenList; real BitQiu still needs the OpenList fork.
//
// go test -tags integration ./backend/cloud
func TestLocalWebDAVEndToEnd(t *testing.T) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not in PATH")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "sample.mp4")
	cmd := exec.Command(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=24:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-shortest", mediaPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot generate fixture: %v: %s", err, out)
	}
	info, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}

	dav := &webdav.Handler{
		Prefix:     "/dav",
		FileSystem: webdav.Dir(dir),
		LockSystem: webdav.NewMemLS(),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" && r.URL.Path == "/dav" {
			r.URL.Path = "/dav/"
		}
		dav.ServeHTTP(w, r)
	}))
	defer srv.Close()

	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := NewService(NewSQLiteRepository(db), nil, nil)
	conn, err := svc.SaveConnection(ConnectionInput{
		Name: "local webdav", Endpoint: srv.URL + "/dav", RootPath: "/", VerifyTLS: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := svc.Browse(conn.ID, "/")
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "sample.mp4" || entries[0].Size != info.Size() {
		t.Fatalf("entries = %+v", entries)
	}

	proxy, err := NewMediaProxy(
		func(id int64) (MediaInfo, error) {
			return MediaInfo{SourceID: conn.ID, Path: "/sample.mp4", FileName: "sample.mp4", Size: info.Size()}, nil
		},
		svc.OpenRange,
		svc.StatContext,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proxy.Close() }()

	md, err := video.ProbeContextWithTimeout(context.Background(), ffprobe, proxy.URL(1), 30*time.Second)
	if err != nil {
		t.Fatalf("ffprobe through proxy: %v", err)
	}
	if md.Width != 320 || md.Height != 180 || md.DurationMs < 500 {
		t.Errorf("metadata = %+v", md)
	}

	thumbDir := t.TempDir()
	rel, err := thumbnail.GenerateContextWithTimeout(context.Background(), ffmpeg, proxy.URL(1), md.DurationMs, thumbDir, 1, 30*time.Second)
	if err != nil {
		t.Fatalf("ffmpeg cover through proxy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(thumbDir, filepath.Base(rel))); err != nil {
		t.Errorf("cover not written: %v", err)
	}

}
