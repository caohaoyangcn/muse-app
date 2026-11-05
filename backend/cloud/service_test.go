package cloud

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"

	"muse-app/backend/database"
)

func newCloudService(t *testing.T) (*Service, Repository, *sql.DB, *[]string) {
	t.Helper()
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewSQLiteRepository(db)
	events := &[]string{}
	var mu sync.Mutex
	emit := func(name string, _ ...any) {
		mu.Lock()
		*events = append(*events, name)
		mu.Unlock()
	}
	return NewService(repo, emit, nil), repo, db, events
}

func baseInput(endpoint string) ConnectionInput {
	return ConnectionInput{
		Name:      "OpenList",
		Endpoint:  endpoint,
		RootPath:  "/",
		VerifyTLS: true,
	}
}

func TestServiceSavePasswordLifecycle(t *testing.T) {
	svc, repo, db, _ := newCloudService(t)
	// A reversible fake stands in for DPAPI; the production code path is the
	// same, only the platform primitive differs.
	svc.protect = func(s string) (string, error) { return "enc:" + s, nil }
	svc.unprotect = func(s string) (string, error) {
		if !strings.HasPrefix(s, "enc:") {
			return "", fmt.Errorf("bad ciphertext")
		}
		return strings.TrimPrefix(s, "enc:"), nil
	}

	in := baseInput("https://dav.example.test/dav")
	in.Password = "s3cret"
	view, err := svc.SaveConnection(in)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !view.HasPassword || view.RootPath != "/" || !view.VerifyTLS || view.VideoCount != 0 {
		t.Errorf("view = %+v", view)
	}
	var stored string
	if err := db.QueryRow(`SELECT password_enc FROM cloud_connections WHERE id=?`, view.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "enc:s3cret" {
		t.Errorf("stored secret = %q", stored)
	}

	// Editing with an empty password keeps the stored secret.
	in.ID = view.ID
	in.Password = ""
	in.Name = "OpenList 改名"
	view, err = svc.SaveConnection(in)
	if err != nil {
		t.Fatalf("update without password: %v", err)
	}
	if !view.HasPassword || view.Name != "OpenList 改名" {
		t.Errorf("updated view = %+v", view)
	}

	// ClearPassword is the only way to remove it.
	in.ClearPassword = true
	view, err = svc.SaveConnection(in)
	if err != nil {
		t.Fatalf("clear password: %v", err)
	}
	if view.HasPassword {
		t.Error("ClearPassword did not remove the secret")
	}

	// A password typed on a platform without secret storage is refused.
	if runtime.GOOS != "windows" {
		plain := NewService(repo, nil, nil)
		in.ClearPassword = false
		in.Password = "cannot-store"
		if _, err := plain.SaveConnection(in); !errors.Is(err, ErrPasswordStorageUnavailable) {
			t.Errorf("unsupported platform err = %v, want ErrPasswordStorageUnavailable", err)
		}
	}
}

func TestServiceSaveValidatesEndpointAndRoot(t *testing.T) {
	svc, _, _, _ := newCloudService(t)
	if _, err := svc.SaveConnection(baseInput("ftp://host/dav")); err == nil || !strings.Contains(err.Error(), "WebDAV 地址无效") {
		t.Errorf("bad endpoint err = %v", err)
	}
	in := baseInput("https://host/dav")
	in.RootPath = "/../etc"
	if _, err := svc.SaveConnection(in); err == nil || !strings.Contains(err.Error(), "路径格式无效") {
		t.Errorf("bad root err = %v", err)
	}
	in = baseInput("https://host/dav")
	in.Name = "  "
	if _, err := svc.SaveConnection(in); err == nil {
		t.Error("blank name must be rejected")
	}
}

func TestServiceBrowseSortsAndMapsHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dav" && r.URL.Path != "/dav/" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, multistatusPrefix+
			davResponseXML("/dav", "/", "", "", "", true)+
			davResponseXML("/dav/b.mp4", "b.mp4", "20", "", "video/mp4", false)+
			davResponseXML("/dav/A%E7%9B%AE%E5%BD%95", "A目录", "", "", "", true)+
			davResponseXML("/dav/a.MP4", "a.MP4", "10", "", "video/mp4", false)+
			`</d:multistatus>`)
	}))
	defer srv.Close()
	svc, _, _, events := newCloudService(t)
	view, err := svc.SaveConnection(baseInput(srv.URL + "/dav"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := svc.Browse(view.ID, "/")
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name)
	}
	want := []string{"A目录", "a.MP4", "b.mp4"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", got, want)
	}
	// The first successful browse emits a change from "unknown" to online.
	if len(*events) == 0 || (*events)[0] != "cloud:status" {
		t.Errorf("events = %v, want cloud:status", *events)
	}
}

func TestServiceTestConnectionOfflineAndRecover(t *testing.T) {
	healthy := false
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !healthy {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, multistatusPrefix+
			davResponseXML("/dav", "/", "", "", "", true)+
			`</d:multistatus>`)
	}))
	defer srv.Close()
	svc, _, _, _ := newCloudService(t)
	view, err := svc.SaveConnection(baseInput(srv.URL + "/dav"))
	if err != nil {
		t.Fatal(err)
	}
	in := baseInput(srv.URL + "/dav")
	in.ID = view.ID
	result, err := svc.TestConnection(in)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if result.OK || !strings.Contains(result.Error, "认证失败") {
		t.Errorf("offline result = %+v", result)
	}
	got, _ := svc.Connection(view.ID)
	if got.Online || !strings.Contains(got.LastError, "认证失败") {
		t.Errorf("offline view = %+v", got)
	}

	healthy = true
	result, err = svc.TestConnection(in)
	if err != nil {
		t.Fatalf("test healthy: %v", err)
	}
	if !result.OK || result.Error != "" {
		t.Errorf("healthy result = %+v", result)
	}
	got, _ = svc.Connection(view.ID)
	if !got.Online || got.LastError != "" || got.LastOKAt == 0 {
		t.Errorf("healthy view = %+v", got)
	}
}

func TestServiceDeleteProtectsFavorites(t *testing.T) {
	svc, _, db, _ := newCloudService(t)
	view, err := svc.SaveConnection(baseInput("https://host/dav"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status, source_id, remote_path)
		VALUES ('cloud://1/x','x.mp4',10,1,1,'available',?, '/x.mp4')`, view.ID); err != nil {
		t.Fatal(err)
	}
	err = svc.DeleteConnection(view.ID)
	if err == nil || !strings.Contains(err.Error(), "还有 1 个收藏") {
		t.Fatalf("delete with favorites err = %v", err)
	}
	if _, err := db.Exec(`DELETE FROM videos WHERE source_id=?`, view.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteConnection(view.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Connection(view.ID); !errors.Is(err, ErrConnectionNotFound) {
		t.Errorf("get deleted err = %v", err)
	}
}

func TestServiceHealthCheckPersists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	svc, _, _, _ := newCloudService(t)
	view, err := svc.SaveConnection(baseInput(srv.URL + "/dav"))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := svc.repo.GetConnection(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	svc.checkOne(conn)
	got, _ := svc.Connection(view.ID)
	if got.Online {
		t.Errorf("404 health should mark offline for a bad root: %+v", got)
	}
	if !strings.Contains(got.LastError, "路径不存在") {
		t.Errorf("last error = %q", got.LastError)
	}
}
