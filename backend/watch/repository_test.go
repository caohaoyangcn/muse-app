package watch

import (
	"testing"
	"time"

	"muse-app/backend/database"
)

func newTestRepo(t *testing.T) Repository {
	t.Helper()
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewSQLiteRepository(db)
}

func TestWatchDirRepositoryCRUD(t *testing.T) {
	repo := newTestRepo(t)

	d := &WatchedDir{Path: `D:\Videos`, CreatedAt: 1000}
	if err := repo.Create(d); err != nil {
		t.Fatalf("create: %v", err)
	}
	if d.ID == 0 {
		t.Fatal("expected id to be set")
	}

	got, err := repo.Get(d.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Path != `D:\Videos` || got.CreatedAt != 1000 || got.LastScanAt != 0 {
		t.Errorf("got %+v", got)
	}

	if _, err := repo.Get(999); err != ErrNotFound {
		t.Errorf("get missing: err = %v, want ErrNotFound", err)
	}

	// Path lookup is the duplicate check for Add; NOCASE must collapse
	// case-variant spellings of the same Windows folder.
	byPath, err := repo.GetByPath(`d:\videos`)
	if err != nil {
		t.Fatalf("get by path (case variant): %v", err)
	}
	if byPath.ID != d.ID {
		t.Errorf("get by path: got id %d, want %d", byPath.ID, d.ID)
	}
	if _, err := repo.GetByPath(`D:\Other`); err != ErrNotFound {
		t.Errorf("get by missing path: err = %v, want ErrNotFound", err)
	}

	d2 := &WatchedDir{Path: `D:\More`, CreatedAt: 1001}
	if err := repo.Create(d2); err != nil {
		t.Fatalf("create second: %v", err)
	}
	list, err := repo.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].ID != d.ID || list[1].ID != d2.ID {
		t.Errorf("list: got %+v", list)
	}

	if err := repo.MarkScanned(d.ID, 2000); err != nil {
		t.Fatalf("mark scanned: %v", err)
	}
	got, _ = repo.Get(d.ID)
	if got.LastScanAt != 2000 {
		t.Errorf("mark scanned not persisted: %+v", got)
	}

	if err := repo.Delete(d.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.Delete(d.ID); err != ErrNotFound {
		t.Errorf("delete missing: err = %v, want ErrNotFound", err)
	}
}

func TestWatchDirRepositoryCreateDuplicatePath(t *testing.T) {
	repo := newTestRepo(t)
	if err := repo.Create(&WatchedDir{Path: `D:\Videos`, CreatedAt: time.Now().UnixMilli()}); err != nil {
		t.Fatalf("create: %v", err)
	}
	// The UNIQUE COLLATE NOCASE constraint backs up the service-level dedup.
	if err := repo.Create(&WatchedDir{Path: `d:\videos`, CreatedAt: time.Now().UnixMilli()}); err == nil {
		t.Fatal("expected duplicate path insert to fail")
	}
}
