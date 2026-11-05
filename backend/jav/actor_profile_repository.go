package jav

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// UpsertActorProfiles stores the scraped profiles, merging them into rows that
// already exist. A profile is an enrichment of a scrape result, so a later
// scrape that knows less about a performer (a provider that omits a field, or
// one that only carries the name) must not erase what an earlier one recorded.
// Empty strings and NULL numbers therefore keep the stored value; only facts the
// provider actually returned overwrite.
func (r *SQLiteRepository) UpsertActorProfiles(profiles []ActorProfile) error {
	if len(profiles) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin actor profile upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO actor_profiles (
			name, provider, star_id, avatar_url, avatar_path, birthday,
			height_cm, bust_cm, cup, waist_cm, hip_cm, blood_type, hometown,
			hobby, movie_count, last_release_unix, aliases_json, fetched_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			provider          = CASE WHEN excluded.provider <> '' THEN excluded.provider ELSE actor_profiles.provider END,
			star_id           = CASE WHEN excluded.star_id <> '' THEN excluded.star_id ELSE actor_profiles.star_id END,
			avatar_url        = CASE WHEN excluded.avatar_url <> '' THEN excluded.avatar_url ELSE actor_profiles.avatar_url END,
			avatar_path       = CASE WHEN excluded.avatar_path <> '' THEN excluded.avatar_path ELSE actor_profiles.avatar_path END,
			birthday          = CASE WHEN excluded.birthday <> '' THEN excluded.birthday ELSE actor_profiles.birthday END,
			height_cm         = COALESCE(excluded.height_cm, actor_profiles.height_cm),
			bust_cm           = COALESCE(excluded.bust_cm, actor_profiles.bust_cm),
			cup               = CASE WHEN excluded.cup <> '' THEN excluded.cup ELSE actor_profiles.cup END,
			waist_cm          = COALESCE(excluded.waist_cm, actor_profiles.waist_cm),
			hip_cm            = COALESCE(excluded.hip_cm, actor_profiles.hip_cm),
			blood_type        = CASE WHEN excluded.blood_type <> '' THEN excluded.blood_type ELSE actor_profiles.blood_type END,
			hometown          = CASE WHEN excluded.hometown <> '' THEN excluded.hometown ELSE actor_profiles.hometown END,
			hobby             = CASE WHEN excluded.hobby <> '' THEN excluded.hobby ELSE actor_profiles.hobby END,
			movie_count       = COALESCE(excluded.movie_count, actor_profiles.movie_count),
			last_release_unix = COALESCE(excluded.last_release_unix, actor_profiles.last_release_unix),
			aliases_json      = CASE WHEN excluded.aliases_json <> '[]' THEN excluded.aliases_json ELSE actor_profiles.aliases_json END,
			fetched_at        = COALESCE(excluded.fetched_at, actor_profiles.fetched_at),
			updated_at        = excluded.updated_at`)
	if err != nil {
		return fmt.Errorf("prepare actor profile upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, profile := range profiles {
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			continue
		}
		aliases, err := json.Marshal(stringList(profile.Aliases))
		if err != nil {
			return fmt.Errorf("encode actor aliases: %w", err)
		}
		if _, err := stmt.Exec(
			name, profile.Provider, profile.StarID, profile.AvatarURL,
			profile.AvatarPath, profile.Birthday, profile.HeightCM,
			profile.BustCM, profile.Cup, profile.WaistCM, profile.HipCM,
			profile.BloodType, profile.Hometown, profile.Hobby,
			profile.MovieCount, profile.LastReleaseUnix, aliases,
			profile.FetchedAt, now, now,
		); err != nil {
			return fmt.Errorf("upsert actor profile %q: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit actor profile upsert: %w", err)
	}
	return nil
}

// ListActorProfiles returns every stored profile, ordered by name. Rows whose
// alias list does not parse are returned without aliases rather than failing
// the read: a corrupt enrichment must not hide the actor index.
func (r *SQLiteRepository) ListActorProfiles() ([]ActorProfile, error) {
	rows, err := r.db.Query(`
		SELECT name, provider, star_id, avatar_url, avatar_path, birthday,
		       height_cm, bust_cm, cup, waist_cm, hip_cm, blood_type, hometown,
		       hobby, movie_count, last_release_unix, aliases_json, fetched_at
		FROM actor_profiles
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list actor profiles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ActorProfile, 0)
	for rows.Next() {
		var (
			profile      ActorProfile
			height, bust sql.NullInt64
			waist, hip   sql.NullInt64
			movieCount   sql.NullInt64
			lastRelease  sql.NullInt64
			fetchedAt    sql.NullInt64
			aliasesJSON  string
		)
		if err := rows.Scan(
			&profile.Name, &profile.Provider, &profile.StarID,
			&profile.AvatarURL, &profile.AvatarPath, &profile.Birthday,
			&height, &bust, &profile.Cup, &waist, &hip, &profile.BloodType,
			&profile.Hometown, &profile.Hobby, &movieCount, &lastRelease,
			&aliasesJSON, &fetchedAt,
		); err != nil {
			return nil, fmt.Errorf("scan actor profile: %w", err)
		}
		profile.HeightCM = nullableInt(height)
		profile.BustCM = nullableInt(bust)
		profile.WaistCM = nullableInt(waist)
		profile.HipCM = nullableInt(hip)
		profile.MovieCount = nullableInt(movieCount)
		if lastRelease.Valid {
			unix := lastRelease.Int64
			profile.LastReleaseUnix = &unix
		}
		if fetchedAt.Valid {
			unix := fetchedAt.Int64
			profile.FetchedAt = &unix
		}
		profile.Aliases = parseStringList(aliasesJSON)
		out = append(out, profile)
	}
	return out, rows.Err()
}

// SetActorAvatarPath records where the downloaded avatar of one profile lives.
// It is a separate write because the avatar download happens after the profile
// row exists, and a failed download must not roll the profile back.
func (r *SQLiteRepository) SetActorAvatarPath(name, avatarPath string) error {
	if _, err := r.db.Exec(
		`UPDATE actor_profiles SET avatar_path=?, updated_at=? WHERE name=?`,
		avatarPath, time.Now().UnixMilli(), name,
	); err != nil {
		return fmt.Errorf("set actor avatar path: %w", err)
	}
	return nil
}

// EnqueueActorFills adds the names to the profile backfill queue. A name that
// is already queued, running or terminally not found is left alone (re-running
// the queue must not resurrect a performer the provider does not know); a name
// that previously failed is retried, because the usual cause was a transient
// network or provider error.
func (r *SQLiteRepository) EnqueueActorFills(names []string) (int, error) {
	if len(names) == 0 {
		return 0, nil
	}
	now := time.Now().UnixMilli()
	tx, err := r.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin actor fill enqueue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO actor_fill_tasks (name, status, attempts, max_attempts, created_at, updated_at)
		VALUES (?, ?, 0, 3, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			status        = 'queued',
			attempts      = 0,
			error_code    = '',
			last_error    = '',
			next_retry_at = NULL,
			finished_at   = NULL,
			updated_at    = excluded.updated_at
		WHERE actor_fill_tasks.status = 'failed'`)
	if err != nil {
		return 0, fmt.Errorf("prepare actor fill enqueue: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	queued := 0
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		res, err := stmt.Exec(name, StatusQueued, now, now)
		if err != nil {
			return 0, fmt.Errorf("enqueue actor fill %q: %w", name, err)
		}
		if affected, err := res.RowsAffected(); err == nil {
			queued += int(affected)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit actor fill enqueue: %w", err)
	}
	return queued, nil
}

// ClaimActorFill claims the oldest due actor name and marks it running. The
// conditional UPDATE is what makes the claim atomic even though the backfill
// run is single-threaded today: a second caller sees zero rows affected.
func (r *SQLiteRepository) ClaimActorFill(now int64) (*ActorFillTask, bool, error) {
	var task ActorFillTask
	err := r.db.QueryRow(`
		SELECT name, attempts, max_attempts
		FROM actor_fill_tasks
		WHERE status = 'queued' AND (next_retry_at IS NULL OR next_retry_at <= ?)
		ORDER BY created_at, name
		LIMIT 1`, now).Scan(&task.Name, &task.Attempts, &task.MaxAttempts)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find actor fill: %w", err)
	}
	res, err := r.db.Exec(`
		UPDATE actor_fill_tasks
		SET status='running', attempts=attempts+1, updated_at=?
		WHERE name=? AND status='queued'`, now, task.Name)
	if err != nil {
		return nil, false, fmt.Errorf("claim actor fill: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("claim actor fill: %w", err)
	}
	if affected == 0 {
		return nil, false, nil
	}
	task.Status = StatusRunning
	task.Attempts++
	return &task, true, nil
}

// SetActorFillResult records a claimed name's outcome. A queued status means
// "retry later": nextRetryAt carries the cooldown, exactly like a scrape task.
func (r *SQLiteRepository) SetActorFillResult(name, status, errorCode, lastError string, nextRetryAt, finishedAt *int64) error {
	if len(lastError) > maxErrorLength {
		lastError = lastError[:maxErrorLength]
	}
	_, err := r.db.Exec(`
		UPDATE actor_fill_tasks
		SET status=?, error_code=?, last_error=?, next_retry_at=?, finished_at=?, updated_at=?
		WHERE name=?`,
		status, errorCode, lastError, nextRetryAt, finishedAt, time.Now().UnixMilli(), name)
	if err != nil {
		return fmt.Errorf("set actor fill result: %w", err)
	}
	return nil
}

// ListActorFillTasks returns the whole queue, newest activity first.
func (r *SQLiteRepository) ListActorFillTasks() ([]ActorFillTask, error) {
	rows, err := r.db.Query(`
		SELECT name, status, attempts, max_attempts, error_code, last_error,
		       next_retry_at, created_at, updated_at, finished_at
		FROM actor_fill_tasks
		ORDER BY updated_at DESC, name`)
	if err != nil {
		return nil, fmt.Errorf("list actor fill tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ActorFillTask, 0)
	for rows.Next() {
		var (
			task       ActorFillTask
			nextRetry  sql.NullInt64
			finishedAt sql.NullInt64
		)
		if err := rows.Scan(
			&task.Name, &task.Status, &task.Attempts, &task.MaxAttempts,
			&task.ErrorCode, &task.LastError, &nextRetry, &task.CreatedAt,
			&task.UpdatedAt, &finishedAt,
		); err != nil {
			return nil, fmt.Errorf("scan actor fill task: %w", err)
		}
		if nextRetry.Valid {
			unix := nextRetry.Int64
			task.NextRetryAt = &unix
		}
		if finishedAt.Valid {
			unix := finishedAt.Int64
			task.FinishedAt = &unix
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

// ResetActorFills requeues the names a previous process left running. The
// backfill run has no lease: a crash is the only way a row stays running, and
// the work is idempotent, so requeueing is safe.
func (r *SQLiteRepository) ResetActorFills() error {
	if _, err := r.db.Exec(
		`UPDATE actor_fill_tasks SET status='queued', updated_at=? WHERE status='running'`,
		time.Now().UnixMilli(),
	); err != nil {
		return fmt.Errorf("reset actor fills: %w", err)
	}
	return nil
}

func nullableInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	n := int(value.Int64)
	return &n
}

// parseStringList decodes a JSON string array, treating a malformed value as an
// empty list. Stored lists are written by this package only, but a partial write
// must not break the read path (the actor aggregation follows the same rule).
func parseStringList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}
