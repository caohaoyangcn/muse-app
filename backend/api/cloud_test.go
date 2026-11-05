package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"muse-app/backend/cloud"
	"muse-app/backend/database"
	"muse-app/backend/group"
	"muse-app/backend/video"
)

func TestAddCloudVideosAndReadModel(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	videoRepo := video.NewSQLiteRepository(db)
	processor := video.NewProcessor(videoRepo, "", "", filepath.Join(t.TempDir(), "thumb"), nil, log, 1)
	t.Cleanup(processor.Close)
	videoSvc := video.NewService(videoRepo, processor, t.TempDir(), log, nil)
	groupSvc := group.NewService(group.NewSQLiteRepository(db), log, nil)
	lib := New(videoSvc, groupSvc, log)
	cloudSvc := cloud.NewService(cloud.NewSQLiteRepository(db), nil, log)
	lib.SetCloudService(cloudSvc)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		switch r.URL.Path {
		case "/dav/媒体/a.mp4":
			_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">`+
				`<d:response><d:href>/dav/%E5%AA%92%E4%BD%93/a.mp4</d:href><d:propstat><d:prop>`+
				`<d:displayname>a.mp4</d:displayname><d:getcontentlength>2048</d:getcontentlength>`+
				`<d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`+
				`</d:multistatus>`)
		case "/dav/媒体/readme.txt":
			_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">`+
				`<d:response><d:href>/dav/%E5%AA%92%E4%BD%93/readme.txt</d:href><d:propstat><d:prop>`+
				`<d:displayname>readme.txt</d:displayname><d:getcontentlength>10</d:getcontentlength>`+
				`<d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`+
				`</d:multistatus>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	view, err := cloudSvc.SaveConnection(cloud.ConnectionInput{
		Name: "测试网盘", Endpoint: srv.URL + "/dav", RootPath: "/", VerifyTLS: true,
	})
	if err != nil {
		t.Fatalf("save connection: %v", err)
	}

	paths := []string{"/媒体/a.mp4", "/媒体/readme.txt"}
	results := lib.AddCloudVideos(view.ID, paths)
	if len(results) != 2 {
		t.Fatalf("results = %+v", results)
	}
	if !results[0].Success || results[0].VideoID == 0 {
		t.Errorf("first result = %+v", results[0])
	}
	if results[1].Success || results[1].Error != "不支持的文件类型" {
		t.Errorf("unsupported result = %+v", results[1])
	}

	all, err := lib.ListVideos()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("videos = %+v", all)
	}
	got := all[0]
	if got.SourceID != view.ID || got.SourceName != "测试网盘" || got.SourceOffline {
		t.Errorf("read model = %+v", got)
	}
	if got.RemotePath != "/媒体/a.mp4" || got.Path == got.RemotePath {
		t.Errorf("remote path = %q synthetic = %q", got.RemotePath, got.Path)
	}

	// A second add is a duplicate with the familiar message.
	again := lib.AddCloudVideos(view.ID, []string{"/媒体/a.mp4"})
	if len(again) != 1 || again[0].Success || again[0].Error != "已在收藏中" {
		t.Errorf("duplicate result = %+v", again)
	}
}

func TestCloudSourceOfflineReadModel(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	videoRepo := video.NewSQLiteRepository(db)
	processor := video.NewProcessor(videoRepo, "", "", filepath.Join(t.TempDir(), "thumb"), nil, log, 1)
	t.Cleanup(processor.Close)
	videoSvc := video.NewService(videoRepo, processor, t.TempDir(), log, nil)
	groupSvc := group.NewService(group.NewSQLiteRepository(db), log, nil)
	lib := New(videoSvc, groupSvc, log)
	cloudSvc := cloud.NewService(cloud.NewSQLiteRepository(db), nil, log)
	lib.SetCloudService(cloudSvc)

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">`+
			`<d:response><d:href>/dav/%E5%AA%92%E4%BD%93/a.mp4</d:href><d:propstat><d:prop>`+
			`<d:displayname>a.mp4</d:displayname><d:getcontentlength>10</d:getcontentlength>`+
			`<d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`+
			`</d:multistatus>`)
	}))
	defer healthy.Close()
	conn, err := cloudSvc.SaveConnection(cloud.ConnectionInput{Name: "dav", Endpoint: healthy.URL + "/dav", RootPath: "/", VerifyTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	if res := lib.AddCloudVideos(conn.ID, []string{"/媒体/a.mp4"}); len(res) != 1 || !res[0].Success {
		t.Fatalf("add = %+v", res)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer bad.Close()
	// Point the saved connection at an auth failure and let an ordinary browse
	// observe it; the read model must then flag the remote favorite.
	if _, err := cloudSvc.SaveConnection(cloud.ConnectionInput{
		ID: conn.ID, Name: "dav", Endpoint: bad.URL + "/dav", RootPath: "/", VerifyTLS: true,
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = cloudSvc.Browse(conn.ID, "/")
	all, err := lib.ListVideos()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || !all[0].SourceOffline {
		t.Errorf("read model after auth failure = %+v", all)
	}
}

func TestArchiveCloudConnectionMovesFavoritesToTrashArchive(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	videoRepo := video.NewSQLiteRepository(db)
	processor := video.NewProcessor(videoRepo, "", "", filepath.Join(t.TempDir(), "thumb"), nil, log, 1)
	t.Cleanup(processor.Close)
	videoSvc := video.NewService(videoRepo, processor, t.TempDir(), log, nil)
	groupSvc := group.NewService(group.NewSQLiteRepository(db), log, nil)
	lib := New(videoSvc, groupSvc, log)
	cloudSvc := cloud.NewService(cloud.NewSQLiteRepository(db), nil, log)
	lib.SetCloudService(cloudSvc)

	dav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">`+
			`<d:response><d:href>/dav/%E7%94%B5%E5%BD%B1/a.mp4</d:href><d:propstat><d:prop>`+
			`<d:displayname>a.mp4</d:displayname><d:getcontentlength>10</d:getcontentlength>`+
			`<d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`+
			`</d:multistatus>`)
	}))
	defer dav.Close()
	conn, err := cloudSvc.SaveConnection(cloud.ConnectionInput{Name: "测试盘", Endpoint: dav.URL + "/dav", RootPath: "/", VerifyTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	if res := lib.AddCloudVideos(conn.ID, []string{"/电影/a.mp4"}); len(res) != 1 || !res[0].Success {
		t.Fatalf("add = %+v", res)
	}
	active, err := lib.ListVideos()
	if err != nil || len(active) != 1 {
		t.Fatalf("active before archive = %+v, err=%v", active, err)
	}

	count, err := lib.ArchiveCloudConnection(conn.ID)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if count != 1 {
		t.Errorf("archived count = %d, want 1", count)
	}
	if active, _ := lib.ListVideos(); len(active) != 0 {
		t.Errorf("active after archive = %+v", active)
	}
	if trash, _ := lib.ListTrash(); len(trash.Videos) != 0 {
		t.Errorf("archived rows leaked into normal trash: %+v", trash.Videos)
	}
	archived, err := lib.ListArchived()
	if err != nil || len(archived) != 1 {
		t.Fatalf("archived = %+v, err=%v", archived, err)
	}
	if archived[0].DeleteReason != video.TrashReasonArchived || archived[0].SourceName != "测试盘" || archived[0].SourceEnabled {
		t.Errorf("archived row = %+v", archived[0])
	}
	// Re-enabling the source does not restore the archive.
	if err := lib.EnableCloudConnection(conn.ID); err != nil {
		t.Fatal(err)
	}
	if archived, _ := lib.ListArchived(); len(archived) != 1 {
		t.Errorf("enable restored archive: %+v", archived)
	}
	if err := lib.DeleteCloudConnection(conn.ID); err != nil {
		t.Fatalf("delete retired connection: %v", err)
	}
	archived, _ = lib.ListArchived()
	if len(archived) != 1 || archived[0].SourceName != "测试盘" {
		t.Errorf("archive lost its source snapshot after connection deletion: %+v", archived)
	}
	// Archive is long-lived: normal retention/empty-trash paths must not touch it.
	if _, err := db.Exec(`UPDATE videos SET deleted_at = 1 WHERE id = ?`, archived[0].ID); err != nil {
		t.Fatal(err)
	}
	if n, err := lib.PurgeExpiredTrash(); err != nil || n != 0 {
		t.Errorf("purge removed archive: n=%d err=%v", n, err)
	}
	if n, err := lib.EmptyTrash(); err != nil || n != 0 {
		t.Errorf("empty trash removed archive: n=%d err=%v", n, err)
	}
	if still, _ := lib.ListArchived(); len(still) != 1 {
		t.Errorf("archive disappeared after trash maintenance: %+v", still)
	}
	// The explicit clear action removes it.
	if n, err := lib.ClearArchivedVideos(); err != nil || n != 1 {
		t.Errorf("clear archive = %d, %v", n, err)
	}
	if archived, _ := lib.ListArchived(); len(archived) != 0 {
		t.Errorf("archive not empty: %+v", archived)
	}
}
