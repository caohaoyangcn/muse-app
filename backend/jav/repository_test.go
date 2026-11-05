package jav

import (
	"path/filepath"
	"testing"

	"muse-app/backend/database"
	"muse-app/backend/video"
)

// ListCodes feeds the shared video read model, so it must return exactly the
// saved scrape results: a code is present only after a successful scrape and
// never for a video the user chose to skip.
func TestListCodesReturnsAuthoritativeResults(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewSQLiteRepository(db)
	videos := video.NewSQLiteRepository(db)
	create := func(name string) int64 {
		t.Helper()
		v := &video.Video{
			Path:      filepath.Join(t.TempDir(), name),
			FileName:  name,
			FileSize:  1,
			CreatedAt: 1,
			UpdatedAt: 1,
			Status:    video.StatusAvailable,
		}
		if err := videos.Create(v); err != nil {
			t.Fatalf("create favorite %q: %v", name, err)
		}
		if err := repo.PrepareTask(v.ID, false); err != nil {
			t.Fatalf("prepare task %q: %v", name, err)
		}
		return v.ID
	}

	scraped := create("a.mp4")
	if err := repo.SaveSuccess(scraped, Metadata{Code: "  IPX-633  "}, "filename", 2); err != nil {
		t.Fatalf("save success: %v", err)
	}
	unscraped := create("b.mp4")
	skipped := create("c.mp4")
	if err := repo.SaveSuccess(skipped, Metadata{Code: "ABW-180"}, "filename", 2); err != nil {
		t.Fatalf("save skipped code: %v", err)
	}
	if err := repo.SetOverride(skipped, OverrideSkip, ""); err != nil {
		t.Fatalf("skip video: %v", err)
	}

	codes, err := repo.ListCodes()
	if err != nil {
		t.Fatalf("list codes: %v", err)
	}
	if got := codes[scraped]; got != "IPX-633" {
		t.Errorf("scraped code = %q, want trimmed IPX-633", got)
	}
	if _, ok := codes[unscraped]; ok {
		t.Error("a video with no scrape result must not carry a code")
	}
	if _, ok := codes[skipped]; ok {
		t.Error("a skipped video must not carry a code")
	}
}
