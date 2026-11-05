package database

import (
	"database/sql"
	"fmt"
)

// currentSchemaVersion is the highest applied schema version. It is tracked in
// PRAGMA user_version. Bump it whenever a migration adds a column/table that
// requires (or could benefit from) re-extracting data for pre-existing rows.
const currentSchemaVersion = 23

// MigrationResult reports what happened during Migrate so callers can trigger
// one-off backfills (e.g. re-probing ffprobe for videos created before a new
// metadata field existed).
type MigrationResult struct {
	// SchemaVersion is the user_version after migration.
	SchemaVersion int
	// BackfillVideoMetadata is true when the frame rate (or any metadata added
	// by a later migration) is newly available for already-stored videos,
	// meaning existing rows should be re-probed.
	BackfillVideoMetadata bool
	// BackfillFingerprints is true when the content-fingerprint columns were
	// just created, so no existing row has one. The caller should schedule one
	// background content-duplicate scan to fill them in. Unlike the metadata
	// backfill this is not run on every startup: routine startups find a library
	// that incremental hashing already covers, and a scan touches every file on
	// disk (which may be an external drive that has to spin up).
	BackfillFingerprints bool
}

// schemaV1 matches spec-v1.md §5 with two additions:
//   - width/height columns so the app can show resolution (DoD "获取 resolution")
//   - COLLATE NOCASE on path, because Windows paths differing only in case
//     point to the same file.
//
// V1 only needs "create if missing"; idempotent so it is safe to run even when
// the tables already exist (e.g. pre-versioning installs).
const schemaV1 = `
CREATE TABLE IF NOT EXISTS videos (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    path            TEXT NOT NULL UNIQUE COLLATE NOCASE,
    file_name       TEXT NOT NULL,
    file_size       INTEGER NOT NULL,
    duration_ms     INTEGER,
    width           INTEGER,
    height          INTEGER,
    thumbnail_path  TEXT,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    last_opened_at  INTEGER,
    status          TEXT NOT NULL DEFAULT 'available'
);
CREATE INDEX IF NOT EXISTS idx_videos_created_at ON videos(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_videos_status ON videos(status);
`

// schemaV2 adds grouping of favorites (分组). Favorites can be assigned to
// zero or more named, reorderable groups.
const schemaV2 = `
CREATE TABLE IF NOT EXISTS video_groups (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    position   INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_video_groups_position ON video_groups(position);

CREATE TABLE IF NOT EXISTS video_group_members (
    video_id INTEGER NOT NULL,
    group_id INTEGER NOT NULL,
    PRIMARY KEY (video_id, group_id)
);
CREATE INDEX IF NOT EXISTS idx_video_group_members_group ON video_group_members(group_id);
`

// schemaV5 adds a generic key/value settings table. It holds user preferences
// that must survive restarts (currently only the LAN server's fixed port). Each
// value is stored as text; the settings.Store parses it into the right type.
const schemaV5 = `
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

// schemaV6 adds parent/child grouping (分组父子关系). A nullable parent_id on
// video_groups lets one group be nested under another; a parent's video set is
// the recursive union of its own and every descendant's members. NULL means the
// group is top-level. Adding a column is idempotent-safe for existing installs.
const schemaV6 = `
ALTER TABLE video_groups ADD COLUMN parent_id INTEGER;
CREATE INDEX IF NOT EXISTS idx_video_groups_parent ON video_groups(parent_id);
`

// A note on version 7–9: those steps were the Actor domain (演员) — actors,
// video_actors, video_faces, the clustering tables and the face-scan history.
// Face recognition was removed once metadata scraping covered the same "who is
// in this video" need, so Migrate no longer creates any of it; schemaV17 drops
// the legacy tables on the next migration. Their numbers are gone from this
// file, and a database stamped 7–9 simply skips forward.

// Migrate brings the database structure up to currentSchemaVersion and returns
// what happened so the caller can run any required backfill. It is idempotent:
// every step is safe to run against an already-migrated database, and the
// applied version is tracked in PRAGMA user_version. Pre-versioning installs
// (user_version 0 with V1/V2 already present) are detected structurally and
// upgraded forward without re-running destructive changes.
func Migrate(db *sql.DB) (MigrationResult, error) {
	res := MigrationResult{}
	version, err := userVersion(db)
	if err != nil {
		return res, err
	}
	res.SchemaVersion = version

	if version < 1 {
		if _, err := db.Exec(schemaV1); err != nil {
			return res, fmt.Errorf("migrate database v1: %w", err)
		}
		if _, err := db.Exec(schemaV2); err != nil {
			return res, fmt.Errorf("migrate database v2: %w", err)
		}
		version = 2
	} else if version < 2 {
		if _, err := db.Exec(schemaV2); err != nil {
			return res, fmt.Errorf("migrate database v2: %w", err)
		}
		version = 2
	}

	// v3 adds the frame_rate column and needs a backfill of existing rows.
	if version < 3 {
		if err := addColumnIfMissing(db, "videos", "frame_rate", "ALTER TABLE videos ADD COLUMN frame_rate REAL"); err != nil {
			return res, err
		}
		res.BackfillVideoMetadata = true
		version = 3
	}

	// v4 adds the bit_rate column (overall bit rate in bits per second), which
	// also needs a backfill of existing rows.
	if version < 4 {
		if err := addColumnIfMissing(db, "videos", "bit_rate", "ALTER TABLE videos ADD COLUMN bit_rate INTEGER"); err != nil {
			return res, err
		}
		res.BackfillVideoMetadata = true
		version = 4
	}

	// v5 adds the settings table (LAN server fixed port, etc.).
	if version < 5 {
		if _, err := db.Exec(schemaV5); err != nil {
			return res, fmt.Errorf("migrate database v5: %w", err)
		}
		version = 5
	}

	// v6 adds the nullable parent_id column for parent/child grouping.
	if version < 6 {
		if err := addColumnIfMissing(db, "video_groups", "parent_id", schemaV6); err != nil {
			return res, fmt.Errorf("migrate database v6: %w", err)
		}
		version = 6
	}

	// v7–v9 (the removed Actor domain) are intentionally absent — see the note
	// above schemaV17. A database at one of those versions falls straight
	// through to v10, and v17 drops the tables it inherited.

	// v10 adds the watched_dirs table for folder watching (目录监控): a list of
	// directories whose video files are auto-imported, including files that
	// appear later. Pure table creation; additive, no backfill.
	if version < 10 {
		if _, err := db.Exec(schemaV10); err != nil {
			return res, fmt.Errorf("migrate database v10: %w", err)
		}
		version = 10
	}

	// v11 adds video-info editing (编辑影片信息): an optional custom title and
	// notes (display falls back to file_name when title is empty), plus the
	// custom-cover marker. custom_cover_at is the timestamp at which the user
	// picked a cover frame (NULL = auto thumbnail); it protects that cover from
	// the automatic frame processor.
	if version < 11 {
		if err := addColumnIfMissing(db, "videos", "title", `ALTER TABLE videos ADD COLUMN title TEXT`); err != nil {
			return res, fmt.Errorf("migrate database v11: %w", err)
		}
		if err := addColumnIfMissing(db, "videos", "notes", `ALTER TABLE videos ADD COLUMN notes TEXT`); err != nil {
			return res, fmt.Errorf("migrate database v11: %w", err)
		}
		if err := addColumnIfMissing(db, "videos", "custom_cover_at", `ALTER TABLE videos ADD COLUMN custom_cover_at INTEGER`); err != nil {
			return res, fmt.Errorf("migrate database v11: %w", err)
		}
		version = 11
	}

	// v12 adds the like counter (点赞数). It is a plain incrementing count owned
	// by the favorite itself (spec: favorites reference files, and a like is a
	// property of the favorite, not of the file), so no backfill is needed.
	if version < 12 {
		if err := addColumnIfMissing(db, "videos", "like_count", `ALTER TABLE videos ADD COLUMN like_count INTEGER NOT NULL DEFAULT 0`); err != nil {
			return res, fmt.Errorf("migrate database v12: %w", err)
		}
		version = 12
	}
	if version < 13 {
		if _, err := db.Exec(schemaV13); err != nil {
			return res, fmt.Errorf("migrate database v13: %w", err)
		}
		if err := addColumnIfMissing(db, "videos", "cover_source", `ALTER TABLE videos ADD COLUMN cover_source TEXT NOT NULL DEFAULT 'auto_frame'`); err != nil {
			return res, fmt.Errorf("migrate database v13 cover source: %w", err)
		}
		if err := addColumnIfMissing(db, "videos", "scraped_cover_path", `ALTER TABLE videos ADD COLUMN scraped_cover_path TEXT`); err != nil {
			return res, fmt.Errorf("migrate database v13 scraped cover: %w", err)
		}
		version = 13
	}
	if version < 14 {
		// v14 creates the persistent task table. Existing installs have no
		// scraped task history to migrate, so recovery starts with new imports.
		if _, err := db.Exec(schemaV14); err != nil {
			return res, fmt.Errorf("migrate database v14: %w", err)
		}
		version = 14
	}
	if version < 15 {
		// v15 allows the initial scrape plus three retry cooldowns.
		if _, err := db.Exec(schemaV15); err != nil {
			return res, fmt.Errorf("migrate database v15: %w", err)
		}
		version = 15
	}
	if version < 16 {
		// v16 stores the per-video preference for replacing the current cover
		// with the downloaded scraped cover. Existing videos opt in by default.
		if err := addColumnIfMissing(db, "videos", "allow_scraped_cover", `ALTER TABLE videos ADD COLUMN allow_scraped_cover INTEGER NOT NULL DEFAULT 1`); err != nil {
			return res, fmt.Errorf("migrate database v16: %w", err)
		}
		version = 16
	}
	if version < 17 {
		// v17 removes the Actor domain (演员 / face recognition). Feature dropped
		// in favour of metadata scraping, which supplies cast info directly.
		if _, err := db.Exec(schemaV17); err != nil {
			return res, fmt.Errorf("migrate database v17: %w", err)
		}
		version = 17
	}
	if version < 18 {
		// v18 stores actor alias merges (演员合并): one actor scraped under two
		// stage names. Pure table creation; the aggregate is computed at read
		// time, so no backfill is needed.
		if _, err := db.Exec(schemaV18); err != nil {
			return res, fmt.Errorf("migrate database v18: %w", err)
		}
		version = 18
	}
	if version < 19 {
		// v19 stores the container's video/audio codec names (ffprobe codec_name)
		// so the LAN client can tell direct-playable files from ones needing a
		// and explain file details precisely. Pre-existing rows have no
		// codec, so a metadata backfill is required.
		if err := addColumnIfMissing(db, "videos", "video_codec", `ALTER TABLE videos ADD COLUMN video_codec TEXT`); err != nil {
			return res, fmt.Errorf("migrate database v19 video codec: %w", err)
		}
		if err := addColumnIfMissing(db, "videos", "audio_codec", `ALTER TABLE videos ADD COLUMN audio_codec TEXT`); err != nil {
			return res, fmt.Errorf("migrate database v19 audio codec: %w", err)
		}
		res.BackfillVideoMetadata = true
		version = 19
	}

	if version < 20 {
		// v20 stores the 查重 (duplicate) dismissals: the user has seen that a
		// 番号 has several copies and chose to keep them all. Pure table
		// creation — the duplicate groups themselves are derived at read time
		// from video_jav.code, so nothing needs backfilling.
		if _, err := db.Exec(schemaV20); err != nil {
			return res, fmt.Errorf("migrate database v20: %w", err)
		}
		version = 20
	}

	if version < 21 {
		// v21 rebuilds jav_scrape_tasks because its max_attempts DEFAULT
		// drifted away from the code: v15 raised the constant to 4 and
		// back-filled the rows that existed then, but CREATE TABLE IF NOT
		// EXISTS never runs again, so the live table kept DEFAULT 3 and every
		// task created afterwards silently got a shorter retry budget than
		// intended. SQLite cannot alter a column default, hence the rebuild.
		// Wrapped in a transaction because the step drops a table: a partial
		// application would otherwise lose the whole scrape queue.
		tx, err := db.Begin()
		if err != nil {
			return res, fmt.Errorf("migrate database v21: %w", err)
		}
		if _, err := tx.Exec(schemaV21); err != nil {
			_ = tx.Rollback()
			return res, fmt.Errorf("migrate database v21: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return res, fmt.Errorf("migrate database v21: %w", err)
		}
		version = 21
	}

	if version < 22 {
		// v22 stores the content fingerprint (部分哈希): the digest that lets the
		// library find the same bytes stored twice, plus the on-disk facts it was
		// computed from and the optional full-file confirmation. Every existing
		// row starts without a fingerprint, so the caller is asked to schedule
		// one background scan; the columns are additive and no media row is
		// touched.
		for _, col := range []struct{ name, alter string }{
			{"content_hash", `ALTER TABLE videos ADD COLUMN content_hash TEXT`},
			{"hash_version", `ALTER TABLE videos ADD COLUMN hash_version INTEGER`},
			{"hash_size", `ALTER TABLE videos ADD COLUMN hash_size INTEGER`},
			{"hash_mtime", `ALTER TABLE videos ADD COLUMN hash_mtime INTEGER`},
			{"hash_at", `ALTER TABLE videos ADD COLUMN hash_at INTEGER`},
			{"hash_error", `ALTER TABLE videos ADD COLUMN hash_error TEXT`},
			{"full_hash", `ALTER TABLE videos ADD COLUMN full_hash TEXT`},
			{"full_hash_at", `ALTER TABLE videos ADD COLUMN full_hash_at INTEGER`},
		} {
			if err := addColumnIfMissing(db, "videos", col.name, col.alter); err != nil {
				return res, fmt.Errorf("migrate database v22 %s: %w", col.name, err)
			}
		}
		// Runs after the columns exist: the index covers content_hash.
		if _, err := db.Exec(schemaV22); err != nil {
			return res, fmt.Errorf("migrate database v22: %w", err)
		}
		res.BackfillFingerprints = true
		version = 22
	}

	if version < 23 {
		// v23 stores actor profiles (演员档案): the avatars, birthdays and
		// measurements the metadata provider returns with a work's cast. The
		// scrape results themselves stay in video_jav.actors_json, which keeps
		// the cast order and the aggregation rules unchanged; this table is an
		// enrichment keyed by the scraped name. actor_fill_tasks is the queue
		// for the backfill run that resolves profiles for names scraped before
		// the profiles were recorded. Pure table creation, no backfill: an
		// existing library fills in on its next scrape or on demand.
		if _, err := db.Exec(schemaV23); err != nil {
			return res, fmt.Errorf("migrate database v23: %w", err)
		}
		version = 23
	}

	res.SchemaVersion = version
	if err := setUserVersion(db, version); err != nil {
		return res, err
	}
	return res, nil
}

const schemaV13 = `
CREATE TABLE IF NOT EXISTS video_jav (
    video_id      INTEGER PRIMARY KEY,
    code          TEXT NOT NULL DEFAULT '',
    title         TEXT NOT NULL DEFAULT '',
    studio        TEXT NOT NULL DEFAULT '',
    series        TEXT NOT NULL DEFAULT '',
    release_unix  INTEGER,
    duration_min  INTEGER,
    is_uncensored INTEGER,
    actors_json   TEXT NOT NULL DEFAULT '[]',
    tags_json     TEXT NOT NULL DEFAULT '[]',
    cover_url     TEXT NOT NULL DEFAULT '',
    cover_status  TEXT NOT NULL DEFAULT 'none',
    provider      TEXT NOT NULL DEFAULT '',
    code_source   TEXT NOT NULL DEFAULT 'filename',
    override_mode TEXT NOT NULL DEFAULT 'auto',
    scrape_status TEXT NOT NULL DEFAULT 'queued',
    attempts      INTEGER NOT NULL DEFAULT 0,
    last_error    TEXT NOT NULL DEFAULT '',
    next_retry_at INTEGER,
    fetched_at    INTEGER,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_video_jav_code ON video_jav(code);
CREATE INDEX IF NOT EXISTS idx_video_jav_scrape_status ON video_jav(scrape_status);
`

const schemaV14 = `
CREATE TABLE IF NOT EXISTS jav_scrape_tasks (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    video_id      INTEGER NOT NULL UNIQUE,
    status        TEXT NOT NULL DEFAULT 'queued',
    force         INTEGER NOT NULL DEFAULT 0,
    attempts      INTEGER NOT NULL DEFAULT 0,
    max_attempts  INTEGER NOT NULL DEFAULT 4,
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
CREATE INDEX IF NOT EXISTS idx_jav_scrape_tasks_retry ON jav_scrape_tasks(next_retry_at);
CREATE INDEX IF NOT EXISTS idx_jav_scrape_tasks_lease ON jav_scrape_tasks(lease_until);
`

const schemaV15 = `
UPDATE jav_scrape_tasks SET max_attempts=4 WHERE max_attempts < 4;
`

// schemaV10 creates watched_dirs: the folder-watch list (目录监控). Each row is
// one directory whose video files (recursively, including future additions) are
// auto-imported into the favorites. path is UNIQUE COLLATE NOCASE like
// videos.path, because Windows paths differing only in case are the same folder.
// last_scan_at records the most recent completed recursive scan (null until the
// first one finishes). Pure table creation; additive, no backfill.
const schemaV10 = `
CREATE TABLE IF NOT EXISTS watched_dirs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    path         TEXT NOT NULL UNIQUE COLLATE NOCASE,
    created_at   INTEGER NOT NULL,
    last_scan_at INTEGER
);
`

// userVersion reads PRAGMA user_version.
func userVersion(db *sql.DB) (int, error) {
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return 0, fmt.Errorf("read user_version: %w", err)
	}
	return v, nil
}

// setUserVersion writes PRAGMA user_version.
func setUserVersion(db *sql.DB, v int) error {
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v)); err != nil {
		return fmt.Errorf("write user_version %d: %w", v, err)
	}
	return nil
}

// addColumnIfMissing runs an ALTER TABLE ADD COLUMN only when the target table
// does not already have the named column. SQLite has no "ADD COLUMN IF NOT
// EXISTS", so we introspect PRAGMA table_info instead.
func addColumnIfMissing(db *sql.DB, table, column, alterSQL string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return fmt.Errorf("inspect columns: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan column info: %w", err)
		}
		if name == column {
			return nil // already present
		}
	}
	if _, err := db.Exec(alterSQL); err != nil {
		return fmt.Errorf("alter table %s add column %s: %w", table, column, err)
	}
	return nil
}

// schemaV17 removes the Actor domain (演员) — the face-recognition feature that
// was dropped once metadata scraping covered cast info. It drops every table
// the feature created across v7–v9 and clears its concurrency preference. The
// tables are self-contained (no FKs into videos), so no media row is affected.
const schemaV17 = `
DROP TABLE IF EXISTS face_recognize_history_matches;
DROP TABLE IF EXISTS face_recognize_history_faces;
DROP TABLE IF EXISTS face_recognize_history;
DROP TABLE IF EXISTS faces;
DROP TABLE IF EXISTS face_scan_matches;
DROP TABLE IF EXISTS face_scan_run_faces;
DROP TABLE IF EXISTS face_scan_runs;
DROP TABLE IF EXISTS unknown_faces;
DROP TABLE IF EXISTS actor_candidates;
DROP TABLE IF EXISTS video_faces;
DROP TABLE IF EXISTS video_actors;
DROP TABLE IF EXISTS actors;
DELETE FROM settings WHERE key = 'actor_concurrency';
`

// schemaV18 stores actor alias merges (演员合并): the same performer scraped
// under two stage names. Each row maps one scraped name (alias) to the
// canonical name it should be shown as. The actor list itself stays derived
// from video_jav.actors_json, so merging is a pure read-time reinterpretation
// and needs no backfill. Names are matched exactly, like the cast lookups.
const schemaV18 = `
CREATE TABLE IF NOT EXISTS actor_aliases (
    alias      TEXT PRIMARY KEY,
    canonical  TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_actor_aliases_canonical ON actor_aliases(canonical);
`

// schemaV20 stores the 查重 dismissals. One row per duplicate group the user
// chose to keep as-is ("保留两者"), keyed by the group's 番号 + part marker.
// video_ids holds the group's member ids at dismissal time (ascending, comma
// separated): the prompt comes back when the membership changes, so a newly
// added copy of the same work is still surfaced instead of silently inheriting
// an old dismissal. Nothing needs backfilling — the groups are derived at read
// time from video_jav.code, and the part marker from the file names.
const schemaV20 = `
CREATE TABLE IF NOT EXISTS jav_duplicate_ignores (
    dup_key    TEXT PRIMARY KEY,
    video_ids  TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
`

// javScrapeTasksSchema is the canonical column list of jav_scrape_tasks, held
// in one place so the v21 rebuild cannot drift from the table it replaces.
// schemaV14 keeps its own literal copy on purpose: editing a migration constant
// that has already been applied changes nothing for existing installs, which is
// exactly the mistake v21 exists to repair.
const javScrapeTasksSchema = `(
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    video_id      INTEGER NOT NULL UNIQUE,
    status        TEXT NOT NULL DEFAULT 'queued',
    force         INTEGER NOT NULL DEFAULT 0,
    attempts      INTEGER NOT NULL DEFAULT 0,
    max_attempts  INTEGER NOT NULL DEFAULT 4,
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
)`

// schemaV21 rebuilds jav_scrape_tasks so the stored max_attempts DEFAULT matches
// schemaV14's constant again (v15 changed the constant, which existing tables
// ignore), then repairs the rows the stale default already affected.
//
// The retry budget is what makes this more than cosmetic: ClaimTask only claims
// a task while attempts < max_attempts, so a task created with DEFAULT 3 whose
// third attempt failed could never be claimed again, while the recovery scan
// kept re-enqueueing it as retryable — a permanent queue entry that never made
// progress. Raising the old rows to 4 gives them the retry v15 intended, and
// any row already at its limit is normalised to a terminal status so it stops
// masquerading as pending work.
//
// The leading CREATE TABLE IF NOT EXISTS makes the step self-sufficient: a
// database stamped past v14 without the table would otherwise fail the INSERT
// and leave the app unable to start.
const schemaV21 = `
DROP TABLE IF EXISTS jav_scrape_tasks_v21;
CREATE TABLE IF NOT EXISTS jav_scrape_tasks ` + javScrapeTasksSchema + `;
CREATE TABLE jav_scrape_tasks_v21 ` + javScrapeTasksSchema + `;
INSERT INTO jav_scrape_tasks_v21 (id, video_id, status, force, attempts, max_attempts, next_retry_at, lease_token, lease_until, error_code, last_error, error_at, created_at, updated_at, started_at, finished_at)
    SELECT id, video_id, status, force, attempts, max_attempts, next_retry_at, lease_token, lease_until, error_code, last_error, error_at, created_at, updated_at, started_at, finished_at FROM jav_scrape_tasks;
DROP TABLE jav_scrape_tasks;
ALTER TABLE jav_scrape_tasks_v21 RENAME TO jav_scrape_tasks;
CREATE INDEX IF NOT EXISTS idx_jav_scrape_tasks_status ON jav_scrape_tasks(status);
CREATE INDEX IF NOT EXISTS idx_jav_scrape_tasks_retry ON jav_scrape_tasks(next_retry_at);
CREATE INDEX IF NOT EXISTS idx_jav_scrape_tasks_lease ON jav_scrape_tasks(lease_until);
UPDATE jav_scrape_tasks SET max_attempts=4 WHERE max_attempts < 4;
UPDATE jav_scrape_tasks SET status='failed', next_retry_at=NULL, lease_token=NULL, lease_until=NULL
    WHERE status='retryable' AND attempts >= max_attempts;
`

// schemaV22 stores the dismissals for content duplicates (内容重复): the user has
// seen that several favorites hold byte-identical files and chose to keep them
// all. The shape mirrors jav_duplicate_ignores on purpose — the same
// "member set changed means the decision expired" semantics apply — but the
// table is separate so each domain owns its own cleanup rules.
//
// dup_key is the content fingerprint the group was formed around, and video_ids
// is the ascending id list of its members at the moment of the dismissal.
const schemaV22 = `
CREATE TABLE IF NOT EXISTS content_duplicate_ignores (
    dup_key    TEXT PRIMARY KEY,
    video_ids  TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_videos_content_hash ON videos(content_hash);
`

// schemaV23 stores actor profiles (演员档案) and their backfill queue.
//
// actor_profiles is keyed by the scraped actor name — the same string that
// appears in video_jav.actors_json and that the actor index aggregates on — so
// a profile can never point at a name the library does not show. star_id is the
// provider's stable performer id; it is what lets a second scrape of the same
// performer land on the same row after a stage-name change. avatar_path is a
// data-dir-relative path (avatars/<star_id>.jpg), mirroring covers/jav/<id>.jpg.
//
// actor_fill_tasks is one row per actor name that the backfill run has to
// resolve. It is deliberately separate from actor_profiles: a name with a
// terminal failure (not found, no matching performer) must stay recorded so the
// run does not retry it forever on every start.
const schemaV23 = `
CREATE TABLE IF NOT EXISTS actor_profiles (
    name              TEXT PRIMARY KEY,
    provider          TEXT NOT NULL DEFAULT '',
    star_id           TEXT NOT NULL DEFAULT '',
    avatar_url        TEXT NOT NULL DEFAULT '',
    avatar_path       TEXT NOT NULL DEFAULT '',
    birthday          TEXT NOT NULL DEFAULT '',
    height_cm         INTEGER,
    bust_cm           INTEGER,
    cup               TEXT NOT NULL DEFAULT '',
    waist_cm          INTEGER,
    hip_cm            INTEGER,
    blood_type        TEXT NOT NULL DEFAULT '',
    hometown          TEXT NOT NULL DEFAULT '',
    hobby             TEXT NOT NULL DEFAULT '',
    movie_count       INTEGER,
    last_release_unix INTEGER,
    aliases_json      TEXT NOT NULL DEFAULT '[]',
    fetched_at        INTEGER,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_actor_profiles_star ON actor_profiles(star_id);

CREATE TABLE IF NOT EXISTS actor_fill_tasks (
    name          TEXT PRIMARY KEY,
    status        TEXT NOT NULL DEFAULT 'queued',
    attempts      INTEGER NOT NULL DEFAULT 0,
    max_attempts  INTEGER NOT NULL DEFAULT 3,
    error_code    TEXT NOT NULL DEFAULT '',
    last_error    TEXT NOT NULL DEFAULT '',
    next_retry_at INTEGER,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    finished_at   INTEGER
);
CREATE INDEX IF NOT EXISTS idx_actor_fill_tasks_status ON actor_fill_tasks(status);
CREATE INDEX IF NOT EXISTS idx_actor_fill_tasks_retry ON actor_fill_tasks(next_retry_at);
`
