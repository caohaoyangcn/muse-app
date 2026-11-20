package settings

import (
	"database/sql"
	"testing"

	"muse-app/backend/accesskey"
	"muse-app/backend/database"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open(database.DriverName, "file:"+t.TempDir()+"/app.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestLanAccessKeyGeneratedAndPersisted(t *testing.T) {
	db := openTestDB(t)
	s := New(db)

	key, err := s.LanAccessKey()
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if _, ok := accesskey.Normalize(key); !ok || len(key) != accesskey.Length {
		t.Fatalf("generated key %q is not a valid access key", key)
	}

	// A new store over the same database must reuse the stored key, otherwise
	// every launch would log every phone out.
	again, err := New(db).LanAccessKey()
	if err != nil {
		t.Fatalf("read key again: %v", err)
	}
	if again != key {
		t.Fatalf("key changed between reads: %q then %q", key, again)
	}
}

func TestLanAccessKeyRotate(t *testing.T) {
	db := openTestDB(t)
	s := New(db)

	first, err := s.LanAccessKey()
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	rotated, err := s.RotateLanAccessKey()
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if rotated == first {
		t.Fatal("rotate returned the previous key")
	}
	current, err := New(db).LanAccessKey()
	if err != nil {
		t.Fatalf("read rotated key: %v", err)
	}
	if current != rotated {
		t.Fatalf("stored key = %q, want rotated key %q", current, rotated)
	}
}

func TestLanAccessKeyRepairsCorruptValue(t *testing.T) {
	db := openTestDB(t)
	s := New(db)
	if err := s.set(keyLanAccessKey, "not a key!"); err != nil {
		t.Fatalf("seed corrupt value: %v", err)
	}

	key, err := s.LanAccessKey()
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if _, ok := accesskey.Normalize(key); !ok {
		t.Fatalf("corrupt value was not replaced: %q", key)
	}
	stored, err := s.get(keyLanAccessKey)
	if err != nil {
		t.Fatalf("read stored value: %v", err)
	}
	if stored != key {
		t.Fatalf("stored value = %q, want repaired key %q", stored, key)
	}
}

func TestLanAccessKeyAcceptsDisplayForm(t *testing.T) {
	db := openTestDB(t)
	s := New(db)
	if err := s.set(keyLanAccessKey, "ABCD-EFGH-IJKL-MNOP"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	key, err := s.LanAccessKey()
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if key != "abcdefghijklmnop" {
		t.Fatalf("key = %q, want normalized form", key)
	}
}
