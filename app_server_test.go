package main

import (
	"database/sql"
	"log/slog"
	"testing"

	"muse-app/backend/accesskey"
	"muse-app/backend/database"
	"muse-app/backend/server"
	"muse-app/backend/settings"
)

// The desktop bindings must expose the same key the server enforces, and a
// rotation must update both the stored value and the running server atomically.
func TestLanAccessKeyBindings(t *testing.T) {
	db, err := sql.Open(database.DriverName, "file:"+t.TempDir()+"/app.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	app := NewApp("", nil)
	app.SetSettings(settings.New(db))
	srv := server.New(nil, t.TempDir(), slog.New(slog.DiscardHandler))
	app.SetServer(srv)

	first, err := app.GetLanAccessKey()
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if _, ok := accesskey.Normalize(first); !ok {
		t.Fatalf("exposed key %q is not a valid access key", first)
	}

	rotated, err := app.RotateLanAccessKey()
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if rotated == first {
		t.Fatal("rotation returned the previous key")
	}
	if got := srv.AccessKey(); got != rotated {
		t.Fatalf("server key = %q, want rotated key %q", got, rotated)
	}
	stored, err := app.GetLanAccessKey()
	if err != nil {
		t.Fatalf("get rotated key: %v", err)
	}
	if stored != rotated {
		t.Fatalf("stored key = %q, want %q", stored, rotated)
	}
}
