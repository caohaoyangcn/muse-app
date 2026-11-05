//go:build javbus_live

package jav_test

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"muse-app/backend/database"
	"muse-app/backend/jav"
	"muse-app/backend/jav/providers"
	"muse-app/backend/video"
)

func TestJavBusLiveScrape(t *testing.T) {
	cookie := strings.TrimSpace(os.Getenv("MUSE_JAVBUS_COOKIE"))
	if cookie == "" {
		t.Skip("MUSE_JAVBUS_COOKIE is not set; opt in to the live JavBus test")
	}

	code := strings.TrimSpace(os.Getenv("MUSE_JAVBUS_CODE"))
	if code == "" {
		code = "SMD-133"
	}
	code = strings.ToUpper(code)
	baseURL := strings.TrimSpace(os.Getenv("MUSE_JAVBUS_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://javbus.com"
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	provider := providers.NewJavBusWithConfig(client, baseURL, providers.JavBusConfig{Cookie: cookie})

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "live.db")
	db, err := sql.Open(database.DriverName, "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migration, err := database.Migrate(db)
	if err != nil {
		t.Fatal(err)
	}
	if migration.SchemaVersion != 14 {
		t.Fatalf("temporary database schema version = %d, want 14", migration.SchemaVersion)
	}

	videoPath := filepath.Join(tempDir, code+".mp4")
	if err := os.WriteFile(videoPath, []byte("live test placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	videos := video.NewSQLiteRepository(db)
	v := &video.Video{
		Path:      videoPath,
		FileName:  filepath.Base(videoPath),
		FileSize:  int64(len("live test placeholder")),
		Status:    video.StatusAvailable,
		CreatedAt: time.Now().UnixMilli(),
		UpdatedAt: time.Now().UnixMilli(),
	}
	if err := videos.Create(v); err != nil {
		t.Fatal(err)
	}

	coverDir := filepath.Join(tempDir, "covers", "jav")
	service := jav.NewService(
		jav.NewSQLiteRepository(db),
		videos,
		provider,
		coverDir,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		1,
	)
	defer service.Close()

	service.Enqueue(v.ID)
	deadline := time.Now().Add(90 * time.Second)
	var detail *jav.Detail
	for time.Now().Before(deadline) {
		detail, err = service.Get(v.ID)
		if err == nil && detail.ScrapeStatus == jav.StatusSucceeded {
			break
		}
		if err == nil && (detail.ScrapeStatus == jav.StatusBlocked || detail.ScrapeStatus == jav.StatusNotFound) {
			t.Fatalf("live JavBus scrape failed: status=%s errorCode=%s lastError=%q", detail.ScrapeStatus, detail.ErrorCode, detail.LastError)
		}
		time.Sleep(250 * time.Millisecond)
	}
	if detail == nil || detail.ScrapeStatus != jav.StatusSucceeded {
		t.Fatalf("live JavBus scrape did not succeed within timeout: detail=%+v err=%v", detail, err)
	}

	if detail.Code != code {
		t.Fatalf("saved code = %q, want %q", detail.Code, code)
	}
	if strings.TrimSpace(detail.Title) == "" {
		t.Fatal("saved title is empty")
	}
	if detail.Provider != provider.Name() {
		t.Fatalf("saved provider = %q, want %q", detail.Provider, provider.Name())
	}
	if detail.CoverStatus != jav.CoverSucceeded {
		t.Fatalf("cover status = %q, want %q (url=%q lastError=%q)", detail.CoverStatus, jav.CoverSucceeded, detail.CoverURL, detail.LastError)
	}
	if strings.TrimSpace(detail.CoverURL) == "" {
		t.Fatal("saved cover URL is empty")
	}

	coverPath := filepath.Join(coverDir, fmt.Sprintf("%d.jpg", v.ID))
	info, err := os.Stat(coverPath)
	if err != nil {
		t.Fatalf("temporary cover file %q: %v", coverPath, err)
	}
	if info.Size() == 0 {
		t.Fatalf("temporary cover file %q is empty", coverPath)
	}
}
