package database

import (
	"database/sql"
	"fmt"
	"testing"
)

func openMigrated(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open(DriverName, "file:"+t.TempDir()+"/app.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestFreshMigrate ensures a brand-new database lands on the current version and
// requests a metadata backfill (it finds nothing on a fresh library, but the
// migration itself was newly applied).
func TestFreshMigrate(t *testing.T) {
	db := openMigrated(t)
	res, err := Migrate(db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.SchemaVersion != currentSchemaVersion {
		t.Errorf("schema version = %d, want %d", res.SchemaVersion, currentSchemaVersion)
	}
	// A fresh DB is created at v0 and migrated forward, so backfill is reported.
	if !res.BackfillVideoMetadata {
		t.Error("expected backfill on a fresh migrate (v0 -> current)")
	}
	// The content-fingerprint columns are created by the same run, so the fresh
	// database also asks for one content-duplicate scan.
	if !res.BackfillFingerprints {
		t.Error("expected a fingerprint backfill on a fresh migrate (v0 -> current)")
	}
	// Migrating again must be a no-op with no further backfill.
	res2, err := Migrate(db)
	if err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if res2.BackfillVideoMetadata {
		t.Error("second migrate should not request backfill again")
	}
	if res2.BackfillFingerprints {
		t.Error("second migrate should not request a fingerprint backfill again")
	}
	if res2.SchemaVersion != currentSchemaVersion {
		t.Errorf("schema version after re-migrate = %d", res2.SchemaVersion)
	}
}

func TestV14CreatesJavTaskAndCoverState(t *testing.T) {
	db := openMigrated(t)
	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='video_jav'`).Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("video_jav table count = %d, err=%v", tables, err)
	}
	for _, col := range []string{"cover_source", "scraped_cover_path", "allow_scraped_cover"} {
		if !hasColumn(db, "videos", col) {
			t.Errorf("videos.%s missing", col)
		}
	}
	var taskTable int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='jav_scrape_tasks'`).Scan(&taskTable); err != nil || taskTable != 1 {
		t.Fatalf("jav_scrape_tasks table count = %d, err=%v", taskTable, err)
	}
}

func TestV16DefaultsScrapedCoverReplacementToEnabled(t *testing.T) {
	db := openMigrated(t)
	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status) VALUES (?, ?, ?, ?, ?, ?)`, `C:\v\a.mp4`, "a.mp4", 1, 1, 1, "available"); err != nil {
		t.Fatalf("insert video: %v", err)
	}
	var allowed int
	if err := db.QueryRow(`SELECT allow_scraped_cover FROM videos WHERE path=?`, `C:\v\a.mp4`).Scan(&allowed); err != nil {
		t.Fatalf("read default: %v", err)
	}
	if allowed != 1 {
		t.Fatalf("allow_scraped_cover default = %d, want 1", allowed)
	}
}

// TestUpgradeFromV2 simulates a database that already has V1+V2 structure but
// predates the frame_rate column (as if user_version < 3), and verifies the
// upgrade adds the column and flags a metadata backfill.
func TestUpgradeFromV2(t *testing.T) {
	db := openMigrated(t)
	// Build the V1+V2 structure and set version to 2, no frame_rate column.
	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("exec v1: %v", err)
	}
	if _, err := db.Exec(schemaV2); err != nil {
		t.Fatalf("exec v2: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatalf("set version 2: %v", err)
	}
	// Insert a video recorded before frame rate existed (duration present).
	if _, err := db.Exec(
		`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status, duration_ms, width, height)
		 VALUES ('C:\\v\\a.mp4','a.mp4',10,1,1,'available',5000,1920,1080)`); err != nil {
		t.Fatalf("insert video: %v", err)
	}

	res, err := Migrate(db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !res.BackfillVideoMetadata {
		t.Error("upgrade from v2 must request metadata backfill")
	}
	if res.SchemaVersion != currentSchemaVersion {
		t.Errorf("schema version = %d, want %d", res.SchemaVersion, currentSchemaVersion)
	}
	// The column now exists and the pre-existing row still has it NULL.
	if !hasColumn(db, "videos", "frame_rate") {
		t.Error("videos.frame_rate column not added")
	}
	if !hasColumn(db, "videos", "bit_rate") {
		t.Error("videos.bit_rate column not added")
	}
	var fr sql.NullFloat64
	if err := db.QueryRow(`SELECT frame_rate FROM videos WHERE file_name='a.mp4'`).Scan(&fr); err != nil {
		t.Fatalf("read frame_rate: %v", err)
	}
	if fr.Valid {
		t.Errorf("pre-existing video frame_rate should be NULL, got %v", fr.Float64)
	}
	var br sql.NullInt64
	if err := db.QueryRow(`SELECT bit_rate FROM videos WHERE file_name='a.mp4'`).Scan(&br); err != nil {
		t.Fatalf("read bit_rate: %v", err)
	}
	if br.Valid {
		t.Errorf("pre-existing video bit_rate should be NULL, got %v", br.Int64)
	}
	// Re-running migrate must not re-request backfill.
	res2, err := Migrate(db)
	if err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if res2.BackfillVideoMetadata {
		t.Error("no backfill expected on a settled database")
	}
}

// legacyActorSchema recreates the tables the removed Actor domain (演员) created
// in schema v7–v9. It stands in for a pre-v17 database in the drop test, since
// the schema constants themselves are gone.
const legacyActorSchema = `
CREATE TABLE actors (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, avatar_path TEXT,
    created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
CREATE TABLE video_actors (video_id INTEGER NOT NULL, actor_id INTEGER NOT NULL, confidence REAL,
    source TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'confirmed', created_at INTEGER NOT NULL,
    PRIMARY KEY (video_id, actor_id));
CREATE TABLE video_faces (id INTEGER PRIMARY KEY AUTOINCREMENT, video_id INTEGER NOT NULL,
    image_path TEXT, bbox TEXT, embedding BLOB, quality REAL, actor_id INTEGER, created_at INTEGER NOT NULL);
CREATE TABLE unknown_faces (id INTEGER PRIMARY KEY AUTOINCREMENT, video_face_id INTEGER NOT NULL UNIQUE,
    cluster_id TEXT, created_at INTEGER NOT NULL);
CREATE TABLE actor_candidates (id INTEGER PRIMARY KEY AUTOINCREMENT, cluster_id TEXT NOT NULL,
    face_count INTEGER NOT NULL, video_count INTEGER NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
    created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
CREATE TABLE face_scan_runs (id INTEGER PRIMARY KEY AUTOINCREMENT, video_id INTEGER NOT NULL,
    status TEXT NOT NULL, created_at INTEGER NOT NULL);
CREATE TABLE face_scan_run_faces (id INTEGER PRIMARY KEY AUTOINCREMENT, run_id INTEGER NOT NULL,
    image_path TEXT, quality REAL);
CREATE TABLE face_scan_matches (id INTEGER PRIMARY KEY AUTOINCREMENT, run_face_id INTEGER NOT NULL,
    similar_image_path TEXT, actor_id INTEGER, score REAL NOT NULL);
CREATE TABLE faces (id INTEGER PRIMARY KEY AUTOINCREMENT, video_id INTEGER NOT NULL, image_path TEXT,
    bbox TEXT, embedding BLOB, quality REAL, actor_id INTEGER NOT NULL, created_at INTEGER NOT NULL);
CREATE TABLE face_recognize_history (id INTEGER PRIMARY KEY AUTOINCREMENT, video_id INTEGER NOT NULL,
    status TEXT NOT NULL, created_at INTEGER NOT NULL);
CREATE TABLE face_recognize_history_faces (id INTEGER PRIMARY KEY AUTOINCREMENT, run_id INTEGER NOT NULL,
    image_path TEXT, quality REAL, decision TEXT NOT NULL DEFAULT 'pending');
CREATE TABLE face_recognize_history_matches (id INTEGER PRIMARY KEY AUTOINCREMENT, run_face_id INTEGER NOT NULL,
    similar_image_path TEXT, actor_id INTEGER, score REAL NOT NULL);
`

// legacyActorTables lists every table the Actor domain created and which v17
// must drop.
var legacyActorTables = []string{
	"actors", "video_actors", "video_faces", "unknown_faces", "actor_candidates",
	"face_scan_runs", "face_scan_run_faces", "face_scan_matches",
	"faces", "face_recognize_history", "face_recognize_history_faces",
	"face_recognize_history_matches",
}

// TestUpgradeDropsActorDomain simulates an old database (at the last actor-era
// stamp, v9, or just before the drop, v16) that still carries the Actor tables
// and its concurrency preference, then verifies v17 removes all of it while
// leaving media rows and unrelated settings untouched. The v9 case also proves
// the removed v7–v9 create steps are safely skipped.
func TestUpgradeDropsActorDomain(t *testing.T) {
	for _, startVersion := range []int{9, 16} {
		t.Run(fmt.Sprintf("from_v%d", startVersion), func(t *testing.T) {
			db := openMigrated(t)
			// Build the base tables a real install at that version would have.
			if _, err := db.Exec(schemaV1); err != nil {
				t.Fatalf("exec v1: %v", err)
			}
			if _, err := db.Exec(schemaV2); err != nil {
				t.Fatalf("exec v2: %v", err)
			}
			if _, err := db.Exec(schemaV5); err != nil {
				t.Fatalf("exec v5: %v", err)
			}
			if err := addColumnIfMissing(db, "video_groups", "parent_id", schemaV6); err != nil {
				t.Fatalf("exec v6: %v", err)
			}
			if _, err := db.Exec(legacyActorSchema); err != nil {
				t.Fatalf("create legacy actor schema: %v", err)
			}
			if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('actor_concurrency','5')`); err != nil {
				t.Fatalf("seed actor setting: %v", err)
			}
			if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('server_port','8899')`); err != nil {
				t.Fatalf("seed unrelated setting: %v", err)
			}
			if _, err := db.Exec(
				`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status)
				 VALUES ('C:\v\a.mp4','a.mp4',10,1,1,'available')`); err != nil {
				t.Fatalf("insert video: %v", err)
			}
			if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, startVersion)); err != nil {
				t.Fatalf("set version %d: %v", startVersion, err)
			}

			res, err := Migrate(db)
			if err != nil {
				t.Fatalf("migrate v%d -> v17: %v", startVersion, err)
			}
			if res.SchemaVersion != currentSchemaVersion {
				t.Errorf("schema version = %d, want %d", res.SchemaVersion, currentSchemaVersion)
			}
			for _, table := range legacyActorTables {
				var n int
				if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil {
					t.Fatalf("check dropped table %s: %v", table, err)
				}
				if n != 0 {
					t.Errorf("actor table %s should be dropped by v17", table)
				}
			}
			// The actor preference is gone; unrelated settings survive.
			var actorSetting int
			if err := db.QueryRow(`SELECT COUNT(*) FROM settings WHERE key='actor_concurrency'`).Scan(&actorSetting); err != nil {
				t.Fatalf("count actor setting: %v", err)
			}
			if actorSetting != 0 {
				t.Error("actor_concurrency setting should be deleted")
			}
			var portSetting int
			if err := db.QueryRow(`SELECT COUNT(*) FROM settings WHERE key='server_port'`).Scan(&portSetting); err != nil {
				t.Fatalf("count server port: %v", err)
			}
			if portSetting != 1 {
				t.Error("unrelated settings must survive v17")
			}
			// Media data is untouched.
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM videos WHERE file_name='a.mp4'`).Scan(&count); err != nil {
				t.Fatalf("count videos: %v", err)
			}
			if count != 1 {
				t.Errorf("pre-existing video lost during v17")
			}
			// Re-running migrate is a no-op.
			res2, err := Migrate(db)
			if err != nil {
				t.Fatalf("re-migrate: %v", err)
			}
			if res2.SchemaVersion != currentSchemaVersion {
				t.Errorf("re-migrate version = %d", res2.SchemaVersion)
			}
		})
	}
}

// TestV18CreatesActorAliasTable verifies the actor merge table exists with the
// shape the merge service expects, and that a pre-existing database at v17 is
// upgraded to it additively.
func TestV18CreatesActorAliasTable(t *testing.T) {
	db := openMigrated(t)
	// A real database stamped v17 already has the base schema; v19 (later than
	// v18) alters videos, so the base table must exist for the upgrade path.
	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("exec v1: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 17`); err != nil {
		t.Fatalf("stamp v17: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE IF EXISTS actor_aliases`); err != nil {
		t.Fatalf("drop alias table: %v", err)
	}
	res, err := Migrate(db)
	if err != nil {
		t.Fatalf("migrate v17 -> v18: %v", err)
	}
	if res.SchemaVersion != currentSchemaVersion {
		t.Errorf("schema version = %d, want %d", res.SchemaVersion, currentSchemaVersion)
	}
	for _, col := range []string{"alias", "canonical", "created_at"} {
		if !hasColumn(db, "actor_aliases", col) {
			t.Errorf("actor_aliases.%s missing", col)
		}
	}
	// Re-running migrate keeps the table (idempotent, and it must not be
	// dropped like the removed face-recognition tables were).
	if _, err := Migrate(db); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='actor_aliases'`).Scan(&n); err != nil {
		t.Fatalf("check alias table: %v", err)
	}
	if n != 1 {
		t.Errorf("actor_aliases table count = %d, want 1", n)
	}
}

// TestV22AddsContentFingerprintStorage verifies the content-duplicate storage
// (部分哈希去重): the fingerprint columns on videos, the dismissal table, the
// index that makes grouping cheap, and the one-off backfill request that fills
// in the fingerprints for rows recorded before the columns existed.
func TestV22AddsContentFingerprintStorage(t *testing.T) {
	db := openMigrated(t)
	// A database stamped v21 already has the base schema; v22 only alters
	// videos and creates its own table, so the V1 shape is enough to upgrade.
	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("exec v1: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 21`); err != nil {
		t.Fatalf("stamp v21: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE IF EXISTS content_duplicate_ignores`); err != nil {
		t.Fatalf("drop dismissals table: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status)
		 VALUES ('C:\v\a.mp4','a.mp4',2048,1,1,'available')`); err != nil {
		t.Fatalf("insert video: %v", err)
	}

	res, err := Migrate(db)
	if err != nil {
		t.Fatalf("migrate v21 -> v22: %v", err)
	}
	if res.SchemaVersion != currentSchemaVersion {
		t.Errorf("schema version = %d, want %d", res.SchemaVersion, currentSchemaVersion)
	}
	if !res.BackfillFingerprints {
		t.Error("upgrade to v22 must request a fingerprint backfill")
	}
	for _, col := range []string{
		"content_hash", "hash_version", "hash_size", "hash_mtime",
		"hash_at", "hash_error", "full_hash", "full_hash_at",
	} {
		if !hasColumn(db, "videos", col) {
			t.Errorf("videos.%s missing", col)
		}
	}
	// A pre-existing favorite has no fingerprint: nothing is computed during the
	// migration, the scheduled scan fills it in.
	var hash sql.NullString
	if err := db.QueryRow(`SELECT content_hash FROM videos WHERE file_name='a.mp4'`).Scan(&hash); err != nil {
		t.Fatalf("read content_hash: %v", err)
	}
	if hash.Valid {
		t.Errorf("pre-existing video content_hash should be NULL, got %q", hash.String)
	}

	for _, col := range []string{"dup_key", "video_ids", "created_at"} {
		if !hasColumn(db, "content_duplicate_ignores", col) {
			t.Errorf("content_duplicate_ignores.%s missing", col)
		}
	}
	var idx int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_videos_content_hash'`).Scan(&idx); err != nil {
		t.Fatalf("check index: %v", err)
	}
	if idx != 1 {
		t.Errorf("idx_videos_content_hash count = %d, want 1", idx)
	}

	// Re-running migrate keeps the table and does not re-request the backfill.
	res2, err := Migrate(db)
	if err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if res2.BackfillFingerprints {
		t.Error("re-migrate must not request the fingerprint backfill again")
	}
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='content_duplicate_ignores'`).Scan(&n); err != nil {
		t.Fatalf("check dismissals table: %v", err)
	}
	if n != 1 {
		t.Errorf("content_duplicate_ignores table count = %d, want 1", n)
	}
}

func hasColumn(db *sql.DB, table, column string) bool {
	_, ok := declaredDefault(db, table, column)
	return ok
}

// declaredDefault reports the DEFAULT SQLite has stored for a column, and
// whether the column exists at all. Reading the *stored* default is the point:
// a migration that edits an already-applied schema constant changes nothing for
// existing installs, so tests must assert on the table, not the constant.
func declaredDefault(db *sql.DB, table, column string) (string, bool) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return "", false
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return "", false
		}
		if name == column {
			if dflt == nil {
				return "", true
			}
			return fmt.Sprint(dflt), true
		}
	}
	return "", false
}

// TestUpgradeFromV11 verifies a v11 database (pre-like) gains the like_count
// column, existing rows start at 0, and no metadata backfill is requested.
func TestUpgradeFromV11(t *testing.T) {
	db := openMigrated(t)
	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("exec v1: %v", err)
	}
	if _, err := db.Exec(schemaV2); err != nil {
		t.Fatalf("exec v2: %v", err)
	}
	// v5 always precedes v11 in real installs, so the settings table exists.
	if _, err := db.Exec(schemaV5); err != nil {
		t.Fatalf("exec v5: %v", err)
	}
	for _, c := range []struct{ name, sql string }{
		{"frame_rate", `ALTER TABLE videos ADD COLUMN frame_rate REAL`},
		{"bit_rate", `ALTER TABLE videos ADD COLUMN bit_rate INTEGER`},
		{"title", `ALTER TABLE videos ADD COLUMN title TEXT`},
		{"notes", `ALTER TABLE videos ADD COLUMN notes TEXT`},
		{"custom_cover_at", `ALTER TABLE videos ADD COLUMN custom_cover_at INTEGER`},
	} {
		if err := addColumnIfMissing(db, "videos", c.name, c.sql); err != nil {
			t.Fatalf("add %s: %v", c.name, err)
		}
	}
	if _, err := db.Exec(`PRAGMA user_version = 11`); err != nil {
		t.Fatalf("set version 11: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status)
		 VALUES ('C:\v\a.mp4','a.mp4',10,1,1,'available')`); err != nil {
		t.Fatalf("insert video: %v", err)
	}

	res, err := Migrate(db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.SchemaVersion != currentSchemaVersion {
		t.Errorf("schema version = %d, want %d", res.SchemaVersion, currentSchemaVersion)
	}
	// Migrating from v11 reaches v19, which introduces the codec columns and so
	// legitimately requires a metadata backfill. (The v12 like-count step on its
	// own does not — it only adds a defaulted column.)
	if !res.BackfillVideoMetadata {
		t.Error("v11 -> current upgrade must request a codec metadata backfill")
	}

	var likeCount int
	if err := db.QueryRow(`SELECT like_count FROM videos WHERE file_name='a.mp4'`).Scan(&likeCount); err != nil {
		t.Fatalf("read like_count: %v", err)
	}
	if likeCount != 0 {
		t.Errorf("existing row like_count = %d, want 0", likeCount)
	}

	// Re-running migrate is a no-op.
	res2, err := Migrate(db)
	if err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if res2.SchemaVersion != currentSchemaVersion || res2.BackfillVideoMetadata {
		t.Errorf("re-migrate settlement mismatch: %+v", res2)
	}
}

// TestV19AddsCodecColumns verifies the codec columns exist and that the upgrade
// flags a metadata backfill, since pre-existing rows have no codec recorded.
func TestV19AddsCodecColumns(t *testing.T) {
	db := openMigrated(t)
	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("exec v1: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 18`); err != nil {
		t.Fatalf("stamp v18: %v", err)
	}
	res, err := Migrate(db)
	if err != nil {
		t.Fatalf("migrate v18 -> v19: %v", err)
	}
	if res.SchemaVersion != currentSchemaVersion {
		t.Errorf("schema version = %d, want %d", res.SchemaVersion, currentSchemaVersion)
	}
	if !res.BackfillVideoMetadata {
		t.Error("v18 -> v19 must request a metadata backfill for codecs")
	}
	for _, col := range []string{"video_codec", "audio_codec"} {
		if !hasColumn(db, "videos", col) {
			t.Errorf("videos.%s missing", col)
		}
	}
}

// legacyJavTaskSchemaV14 is jav_scrape_tasks exactly as v14 first created it,
// before v15 raised max_attempts to 4. It stands in for a production table:
// v15 changed the schemaV14 constant and back-filled the rows that existed
// then, but CREATE TABLE IF NOT EXISTS never runs again, so the stored default
// stayed 3 for every task created afterwards.
const legacyJavTaskSchemaV14 = `
CREATE TABLE jav_scrape_tasks (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    video_id      INTEGER NOT NULL UNIQUE,
    status        TEXT NOT NULL DEFAULT 'queued',
    force         INTEGER NOT NULL DEFAULT 0,
    attempts      INTEGER NOT NULL DEFAULT 0,
    max_attempts  INTEGER NOT NULL DEFAULT 3,
    next_retry_at INTEGER,
    lease_token   TEXT,
    lease_until   INTEGER,
    error_code    TEXT NOT NULL DEFAULT '',
    last_error    TEXT NOT NULL DEFAULT '',
    error_at      INTEGER,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    started_at    INTEGER,
    finished_at   INTEGER,
    FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_jav_scrape_tasks_status ON jav_scrape_tasks(status);
`

// TestV21RepairsStaleJavTaskAttemptBudget reproduces the production defect: a
// table created when v14 declared DEFAULT 3, then upgraded past v15. Tasks keep
// getting a three-attempt budget, and a task whose third attempt failed can
// never be claimed again (ClaimTask needs attempts < max_attempts) while the
// recovery scan keeps re-enqueueing it as retryable. v21 must repair both the
// stored default and the rows already affected.
func TestV21RepairsStaleJavTaskAttemptBudget(t *testing.T) {
	db := openMigrated(t)
	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("exec v1: %v", err)
	}
	if _, err := db.Exec(legacyJavTaskSchemaV14); err != nil {
		t.Fatalf("create legacy jav task table: %v", err)
	}
	// The fixture must actually reproduce the defect, or the assertions below
	// would pass without the migration doing anything.
	if got, ok := declaredDefault(db, "jav_scrape_tasks", "max_attempts"); !ok || got != "3" {
		t.Fatalf("fixture max_attempts default = %q (found=%t), want the stale 3", got, ok)
	}
	for i, name := range []string{"stuck.mp4", "past-budget.mp4", "pending.mp4", "fresh.mp4"} {
		if _, err := db.Exec(
			`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status) VALUES (?,?,?,?,?,?)`,
			fmt.Sprintf(`C:\v\%s`, name), name, 10, 1, 1, "available"); err != nil {
			t.Fatalf("insert video %d: %v", i, err)
		}
	}
	// Video 1: the reported state — three attempts used, no budget left.
	if _, err := db.Exec(`INSERT INTO jav_scrape_tasks (video_id, status, attempts, max_attempts, next_retry_at, error_code, last_error, created_at, updated_at)
		VALUES (1, 'retryable', 3, 3, 1, 'rate_limited', 'retryable: Avmoo temporarily unavailable', 1, 1)`); err != nil {
		t.Fatalf("seed stuck task: %v", err)
	}
	// Video 2: already at a four-attempt limit under a newer table, so raising
	// the stored default must not revive it.
	if _, err := db.Exec(`INSERT INTO jav_scrape_tasks (video_id, status, attempts, max_attempts, next_retry_at, error_code, last_error, created_at, updated_at)
		VALUES (2, 'retryable', 4, 4, 1, 'rate_limited', 'retryable', 1, 1)`); err != nil {
		t.Fatalf("seed past-budget task: %v", err)
	}
	// Video 3: ordinary queued work that must survive untouched.
	if _, err := db.Exec(`INSERT INTO jav_scrape_tasks (video_id, status, created_at, updated_at) VALUES (3, 'queued', 1, 1)`); err != nil {
		t.Fatalf("seed queued task: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 20`); err != nil {
		t.Fatalf("stamp v20: %v", err)
	}

	res, err := Migrate(db)
	if err != nil {
		t.Fatalf("migrate v20 -> v21: %v", err)
	}
	if res.SchemaVersion != currentSchemaVersion {
		t.Errorf("schema version = %d, want %d", res.SchemaVersion, currentSchemaVersion)
	}
	if res.BackfillVideoMetadata {
		t.Error("v21 only rebuilds the scrape queue, so it must not request a metadata backfill")
	}

	// The stored default now matches the code, so tasks created from here on
	// get the intended budget.
	if got, ok := declaredDefault(db, "jav_scrape_tasks", "max_attempts"); !ok || got != "4" {
		t.Errorf("stored max_attempts default = %q (found=%t), want 4", got, ok)
	}
	if _, err := db.Exec(`INSERT INTO jav_scrape_tasks (video_id, created_at, updated_at) VALUES (4, 1, 1)`); err != nil {
		t.Fatalf("insert fresh task: %v", err)
	}
	var freshMax int
	if err := db.QueryRow(`SELECT max_attempts FROM jav_scrape_tasks WHERE video_id=4`).Scan(&freshMax); err != nil {
		t.Fatalf("read fresh task: %v", err)
	}
	if freshMax != 4 {
		t.Errorf("new task max_attempts = %d, want 4", freshMax)
	}

	// The stuck row gains the retry v15 intended and stays claimable.
	var status string
	var attempts, maxAttempts int
	var nextRetry sql.NullInt64
	if err := db.QueryRow(`SELECT status, attempts, max_attempts, next_retry_at FROM jav_scrape_tasks WHERE video_id=1`).
		Scan(&status, &attempts, &maxAttempts, &nextRetry); err != nil {
		t.Fatalf("read stuck task: %v", err)
	}
	if status != "retryable" || attempts != 3 || maxAttempts != 4 {
		t.Errorf("stuck task = %s %d/%d, want retryable 3/4", status, attempts, maxAttempts)
	}
	if !nextRetry.Valid {
		t.Error("revived task lost its retry schedule")
	}

	// A row that has truly used up its budget becomes terminal instead of
	// staying in the queue as pending work.
	var pastStatus string
	var pastRetry sql.NullInt64
	if err := db.QueryRow(`SELECT status, next_retry_at FROM jav_scrape_tasks WHERE video_id=2`).Scan(&pastStatus, &pastRetry); err != nil {
		t.Fatalf("read past-budget task: %v", err)
	}
	if pastStatus != "failed" {
		t.Errorf("exhausted task status = %q, want failed", pastStatus)
	}
	if pastRetry.Valid {
		t.Errorf("exhausted task still scheduled for retry at %d", pastRetry.Int64)
	}

	// Queued work is untouched, and the rebuild preserved the row identity.
	var pendingStatus string
	var pendingID int64
	if err := db.QueryRow(`SELECT id, status FROM jav_scrape_tasks WHERE video_id=3`).Scan(&pendingID, &pendingStatus); err != nil {
		t.Fatalf("read pending task: %v", err)
	}
	if pendingStatus != "queued" || pendingID != 3 {
		t.Errorf("pending task = id %d %q, want id 3 queued", pendingID, pendingStatus)
	}

	// Re-running migrate must be a no-op.
	res2, err := Migrate(db)
	if err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if res2.SchemaVersion != currentSchemaVersion {
		t.Errorf("re-migrate version = %d", res2.SchemaVersion)
	}
	if got, _ := declaredDefault(db, "jav_scrape_tasks", "max_attempts"); got != "4" {
		t.Errorf("re-migrate changed the stored default to %q", got)
	}
}

// TestV24AddsCloudSourceSchema verifies the WebDAV source storage: the
// connection table, the two remote-reference columns, their indexes, and the
// local-row defaults that keep every existing favorite local. It also proves
// the partial unique index is BINARY (remote paths are case-sensitive).
func TestV24AddsCloudSourceSchema(t *testing.T) {
	db := openMigrated(t)
	if _, err := db.Exec(schemaV1); err != nil {
		t.Fatalf("exec v1: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 23`); err != nil {
		t.Fatalf("stamp v23: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status)
		 VALUES ('C:\v\a.mp4','a.mp4',10,1,1,'available')`); err != nil {
		t.Fatalf("insert video: %v", err)
	}

	res, err := Migrate(db)
	if err != nil {
		t.Fatalf("migrate v23 -> v24: %v", err)
	}
	if res.SchemaVersion != currentSchemaVersion {
		t.Errorf("schema version = %d, want %d", res.SchemaVersion, currentSchemaVersion)
	}
	if res.BackfillVideoMetadata || res.BackfillFingerprints {
		t.Errorf("v24 is additive and must not request a backfill: %+v", res)
	}
	for _, col := range []string{
		"id", "name", "kind", "endpoint", "web_url", "username", "password_enc",
		"root_path", "verify_tls", "created_at", "updated_at", "last_ok_at", "last_error",
	} {
		if !hasColumn(db, "cloud_connections", col) {
			t.Errorf("cloud_connections.%s missing", col)
		}
	}
	for _, col := range []string{"source_id", "remote_path"} {
		if !hasColumn(db, "videos", col) {
			t.Errorf("videos.%s missing", col)
		}
	}
	var sourceID int
	var remotePath string
	if err := db.QueryRow(`SELECT source_id, remote_path FROM videos WHERE file_name='a.mp4'`).Scan(&sourceID, &remotePath); err != nil {
		t.Fatalf("read local row defaults: %v", err)
	}
	if sourceID != 0 || remotePath != "" {
		t.Errorf("local row defaults = (%d, %q), want (0, \"\")", sourceID, remotePath)
	}
	for _, idx := range []string{"idx_videos_remote", "idx_videos_source"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil {
			t.Fatalf("check index %s: %v", idx, err)
		}
		if n != 1 {
			t.Errorf("%s count = %d, want 1", idx, n)
		}
	}

	// The remote key is BINARY: two paths differing only in case coexist.
	insert := `INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status, source_id, remote_path)
		VALUES (?, ?, 1, 1, 1, 'available', 7, ?)`
	if _, err := db.Exec(insert, "cloud://7/a", "a.mp4", "/Media/A.mp4"); err != nil {
		t.Fatalf("insert first remote row: %v", err)
	}
	if _, err := db.Exec(insert, "cloud://7/b", "b.mp4", "/Media/a.mp4"); err != nil {
		t.Fatalf("case-different remote path must be allowed: %v", err)
	}
	if _, err := db.Exec(insert, "cloud://7/c", "c.mp4", "/Media/A.mp4"); err == nil {
		t.Error("exact duplicate (source_id, remote_path) must violate the unique index")
	}

	// Re-running migrate is a no-op.
	res2, err := Migrate(db)
	if err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if res2.SchemaVersion != currentSchemaVersion {
		t.Errorf("re-migrate version = %d", res2.SchemaVersion)
	}
}

// TestV25AddsTrashSchema verifies the recycle-bin columns: an existing library
// is upgraded without backfill, every pre-existing row stays active
// (deleted_at NULL), and the new index supports the active-row reads.
func TestV25AddsTrashSchema(t *testing.T) {
	db := openMigrated(t)
	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Rewind to v24 by removing the v25 additions, which is what an install
	// from the previous release looks like.
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_videos_delete_reason`,
		`DROP INDEX IF EXISTS idx_videos_deleted_at`,
		`ALTER TABLE videos DROP COLUMN archive_source_name`,
		`ALTER TABLE videos DROP COLUMN deleted_at`,
		`ALTER TABLE videos DROP COLUMN delete_reason`,
		`ALTER TABLE cloud_connections DROP COLUMN disabled_at`,
		`ALTER TABLE cloud_connections DROP COLUMN enabled`,
		`PRAGMA user_version = 24`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("rewind to v24 (%s): %v", stmt, err)
		}
	}
	if _, err := db.Exec(
		`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status)
		 VALUES ('C:\v\a.mp4','a.mp4',10,1,1,'available')`); err != nil {
		t.Fatalf("insert video: %v", err)
	}

	res, err := Migrate(db)
	if err != nil {
		t.Fatalf("migrate v24 -> v25: %v", err)
	}
	if res.SchemaVersion != currentSchemaVersion {
		t.Errorf("schema version = %d, want %d", res.SchemaVersion, currentSchemaVersion)
	}
	if res.BackfillVideoMetadata || res.BackfillFingerprints {
		t.Errorf("v25 is additive and must not request a backfill: %+v", res)
	}
	for _, col := range []string{"deleted_at", "delete_reason"} {
		if !hasColumn(db, "videos", col) {
			t.Errorf("videos.%s missing", col)
		}
	}
	var deletedAt sql.NullInt64
	var reason string
	if err := db.QueryRow(`SELECT deleted_at, delete_reason FROM videos WHERE file_name='a.mp4'`).Scan(&deletedAt, &reason); err != nil {
		t.Fatalf("read trash defaults: %v", err)
	}
	if deletedAt.Valid || reason != "" {
		t.Errorf("pre-existing row = (deleted_at=%v, reason=%q), want active", deletedAt, reason)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_videos_deleted_at'`).Scan(&n); err != nil {
		t.Fatalf("check index: %v", err)
	}
	if n != 1 {
		t.Errorf("idx_videos_deleted_at count = %d, want 1", n)
	}

	// Re-running migrate is a no-op.
	res2, err := Migrate(db)
	if err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if res2.SchemaVersion != currentSchemaVersion {
		t.Errorf("re-migrate version = %d", res2.SchemaVersion)
	}
}

// TestV26AddsCloudArchiveSchema verifies the source lifecycle and archive
// snapshot columns, and that an existing v25 database upgrades additively.
func TestV26AddsCloudArchiveSchema(t *testing.T) {
	db := openMigrated(t)
	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Rewind to v25, which is what the previous release looks like.
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_videos_delete_reason`,
		`ALTER TABLE videos DROP COLUMN archive_source_name`,
		`ALTER TABLE cloud_connections DROP COLUMN disabled_at`,
		`ALTER TABLE cloud_connections DROP COLUMN enabled`,
		`PRAGMA user_version = 25`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("rewind to v25 (%s): %v", stmt, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO cloud_connections (name, endpoint, created_at, updated_at) VALUES ('dav','http://host/dav',1,1)`); err != nil {
		t.Fatalf("insert connection: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status) VALUES ('C:\v\a.mp4','a.mp4',1,1,1,'available')`); err != nil {
		t.Fatalf("insert video: %v", err)
	}
	res, err := Migrate(db)
	if err != nil {
		t.Fatalf("migrate v25 -> v26: %v", err)
	}
	if res.SchemaVersion != currentSchemaVersion {
		t.Errorf("schema version = %d", res.SchemaVersion)
	}
	if !hasColumn(db, "cloud_connections", "enabled") || !hasColumn(db, "cloud_connections", "disabled_at") {
		t.Error("cloud connection lifecycle columns missing")
	}
	if !hasColumn(db, "videos", "archive_source_name") {
		t.Error("videos.archive_source_name missing")
	}
	var enabled int
	if err := db.QueryRow(`SELECT enabled FROM cloud_connections WHERE name='dav'`).Scan(&enabled); err != nil || enabled != 1 {
		t.Errorf("existing connection enabled = %d, err=%v, want 1", enabled, err)
	}
	var archiveName string
	if err := db.QueryRow(`SELECT archive_source_name FROM videos WHERE file_name='a.mp4'`).Scan(&archiveName); err != nil || archiveName != "" {
		t.Errorf("archive source default = %q, err=%v", archiveName, err)
	}
	var idx int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_videos_delete_reason'`).Scan(&idx); err != nil || idx != 1 {
		t.Errorf("idx_videos_delete_reason = %d, err=%v", idx, err)
	}
}
