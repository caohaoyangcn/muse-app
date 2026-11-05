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
