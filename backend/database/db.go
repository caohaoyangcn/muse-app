// Package database owns the SQLite database lifecycle for Muse.
package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const DriverName = "sqlite"

// Open opens (creating if needed) the app database inside dataDir and runs any
// pending migrations. The returned MigrationResult tells the caller whether a
// one-off backfill should run (e.g. re-probing metadata for pre-existing rows).
func Open(dataDir string) (*sql.DB, MigrationResult, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, MigrationResult{}, fmt.Errorf("create data dir: %w", err)
	}
	db, err := sql.Open(DriverName, filepath.Join(dataDir, "app.db"))
	if err != nil {
		return nil, MigrationResult{}, fmt.Errorf("open database: %w", err)
	}
	// SQLite allows a single writer: one shared connection avoids SQLITE_BUSY
	// between UI reads and background thumbnail/metadata writes.
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, MigrationResult{}, fmt.Errorf("apply pragma: %w", err)
		}
	}
	res, err := Migrate(db)
	if err != nil {
		_ = db.Close()
		return nil, MigrationResult{}, err
	}
	return db, res, nil
}
