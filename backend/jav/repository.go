package jav

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const maxErrorLength = 4096

type Repository interface {
	Get(videoID int64) (*Detail, error)
	GetTask(videoID int64) (*Task, error)
	ListTaskOverviews() ([]TaskOverview, error)
	// ListCodes returns every saved 番号 keyed by favorite id. It feeds the
	// shared video read model, which shows a favorite under its authoritative
	// code once a scrape has produced one.
	ListCodes() (map[int64]string, error)
	PrepareTask(videoID int64, force bool) error
	ClaimTask(videoID int64, token string, now, leaseUntil int64) (*Task, bool, error)
	SetTaskStatus(videoID int64, token, status, errorCode, lastError string, nextRetryAt *int64) error
	RecoverPending(now int64) ([]Task, error)
	SaveSuccess(videoID int64, metadata Metadata, codeSource string, fetchedAt int64) error
	SetCover(videoID int64, status string) error
	SetOverride(videoID int64, mode, code string) error
	ListActorFilmRefs() ([]ActorFilmRef, error)
	ListActorMerges() ([]ActorMerge, error)
	UpsertActorMerge(alias, canonical string) error
	DeleteActorMerge(alias string) error
	VideoIDsByActorNames(names []string) ([]int64, error)
	UpsertActorProfiles(profiles []ActorProfile) error
	ListActorProfiles() ([]ActorProfile, error)
	SetActorAvatarPath(name, avatarPath string) error
	EnqueueActorFills(names []string) (int, error)
	ClaimActorFill(now int64) (*ActorFillTask, bool, error)
	SetActorFillResult(name, status, errorCode, lastError string, nextRetryAt, finishedAt *int64) error
	ListActorFillTasks() ([]ActorFillTask, error)
	ResetActorFills() error
	ListDuplicateCandidates() ([]DuplicateCandidate, error)
	ListDuplicateIgnores() ([]IgnoredDuplicate, error)
	UpsertDuplicateIgnore(key, videoIDs string) error
	DeleteDuplicateIgnore(key string) error
	Delete(videoID int64) error
}

type SQLiteRepository struct{ db *sql.DB }

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository { return &SQLiteRepository{db: db} }

func (r *SQLiteRepository) PrepareTask(videoID int64, force bool) error {
	now := time.Now().UnixMilli()
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO video_jav (video_id, created_at, updated_at) VALUES (?, ?, ?)`, videoID, now, now); err != nil {
		return fmt.Errorf("create jav result row: %w", err)
	}
	if _, err := r.db.Exec(`INSERT OR IGNORE INTO jav_scrape_tasks (video_id, created_at, updated_at) VALUES (?, ?, ?)`, videoID, now, now); err != nil {
		return fmt.Errorf("create jav task: %w", err)
	}
	if !force {
		return nil
	}
	_, err := r.db.Exec(`UPDATE jav_scrape_tasks SET force=1, status='queued', attempts=0, next_retry_at=NULL, lease_token=NULL, lease_until=NULL, error_code='', last_error='', error_at=NULL, started_at=NULL, finished_at=NULL, created_at=?, updated_at=? WHERE video_id=?`, now, now, videoID)
	if err != nil {
		return fmt.Errorf("reset jav task: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ListTaskOverviews() ([]TaskOverview, error) {
	rows, err := r.db.Query(`
		SELECT
			t.id, t.video_id, t.status, t.force, t.attempts, t.max_attempts,
			t.next_retry_at, t.lease_until, t.error_code, t.last_error, t.error_at,
			t.created_at, t.updated_at, t.started_at, t.finished_at,
			v.id, v.path, v.file_name, v.file_size, v.duration_ms, v.width, v.height,
			v.frame_rate, v.bit_rate, v.thumbnail_path, v.created_at, v.updated_at,
			v.last_opened_at, v.status, v.title, v.notes, v.custom_cover_at,
			v.like_count, v.cover_source, v.scraped_cover_path, v.allow_scraped_cover
		FROM jav_scrape_tasks t
		JOIN videos v ON v.id=t.video_id
		ORDER BY t.created_at DESC, t.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list jav task overviews: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]TaskOverview, 0)
	for rows.Next() {
		var overview TaskOverview
		var force int
		var nextRetry, leaseUntil, errorAt, startedAt, finishedAt sql.NullInt64
		var videoDuration, videoWidth, videoHeight, videoLastOpened, videoCustomCover sql.NullInt64
		var videoBitrate sql.NullInt64
		var videoFrameRate sql.NullFloat64
		var videoThumbnail, videoTitle, videoNotes, videoCoverSource, videoScrapedCover sql.NullString
		var videoAllowScrapedCover sql.NullInt64
		if err := rows.Scan(
			&overview.ID, &overview.VideoID, &overview.Status, &force, &overview.Attempts, &overview.MaxAttempts,
			&nextRetry, &leaseUntil, &overview.ErrorCode, &overview.LastError, &errorAt,
			&overview.CreatedAt, &overview.UpdatedAt, &startedAt, &finishedAt,
			&overview.Video.ID, &overview.Video.Path, &overview.Video.FileName, &overview.Video.FileSize,
			&videoDuration, &videoWidth, &videoHeight, &videoFrameRate, &videoBitrate,
			&videoThumbnail, &overview.Video.CreatedAt, &overview.Video.UpdatedAt, &videoLastOpened,
			&overview.Video.Status, &videoTitle, &videoNotes, &videoCustomCover, &overview.Video.LikeCount,
			&videoCoverSource, &videoScrapedCover, &videoAllowScrapedCover,
		); err != nil {
			return nil, fmt.Errorf("scan jav task overview: %w", err)
		}
		overview.Force = force != 0
		overview.NextRetryAt = nullableInt64(nextRetry)
		overview.LeaseUntil = nullableInt64(leaseUntil)
		overview.ErrorAt = nullableInt64(errorAt)
		overview.StartedAt = nullableInt64(startedAt)
		overview.FinishedAt = nullableInt64(finishedAt)
		overview.Category = TaskCategory(overview.Status)
		overview.Video.DurationMs = videoDuration.Int64
		overview.Video.Width = int(videoWidth.Int64)
		overview.Video.Height = int(videoHeight.Int64)
		overview.Video.FrameRate = videoFrameRate.Float64
		overview.Video.Bitrate = videoBitrate.Int64
		overview.Video.ThumbnailPath = videoThumbnail.String
		overview.Video.LastOpenedAt = videoLastOpened.Int64
		overview.Video.Title = videoTitle.String
		overview.Video.Notes = videoNotes.String
		overview.Video.CustomCoverAt = videoCustomCover.Int64
		overview.Video.CoverSource = videoCoverSource.String
		overview.Video.ScrapedCoverPath = videoScrapedCover.String
		overview.Video.AllowScrapedCover = videoAllowScrapedCover.Int64 != 0
		out = append(out, overview)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate jav task overviews: %w", err)
	}
	return out, nil
}

// ListCodes returns the authoritative 番号 recorded for every favorite,
// keyed by video id. Rows without a code and videos the user chose to skip are
// omitted, so the caller can treat "present" as "safe to show in place of the
// file name". The query is deliberately one scan of the small jav result table
// rather than a per-video lookup: the library lists every favorite at once.
func (r *SQLiteRepository) ListCodes() (map[int64]string, error) {
	rows, err := r.db.Query(`SELECT video_id, TRIM(code) FROM video_jav WHERE TRIM(code) <> '' AND override_mode <> 'skip'`)
	if err != nil {
		return nil, fmt.Errorf("list jav codes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]string)
	for rows.Next() {
		var videoID int64
		var code string
		if err := rows.Scan(&videoID, &code); err != nil {
			return nil, fmt.Errorf("scan jav code: %w", err)
		}
		out[videoID] = code
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) GetTask(videoID int64) (*Task, error) {
	var t Task
	var nextRetry, leaseUntil, errorAt, startedAt, finishedAt sql.NullInt64
	var leaseToken sql.NullString
	var force int
	err := r.db.QueryRow(`SELECT id, video_id, status, force, attempts, max_attempts, next_retry_at, lease_token, lease_until, error_code, last_error, error_at, created_at, updated_at, started_at, finished_at FROM jav_scrape_tasks WHERE video_id=?`, videoID).Scan(&t.ID, &t.VideoID, &t.Status, &force, &t.Attempts, &t.MaxAttempts, &nextRetry, &leaseToken, &leaseUntil, &t.ErrorCode, &t.LastError, &errorAt, &t.CreatedAt, &t.UpdatedAt, &startedAt, &finishedAt)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("get jav task: %w", err)
	}
	t.Force = force != 0
	t.LeaseToken = leaseToken.String
	t.NextRetryAt = nullableInt64(nextRetry)
	t.LeaseUntil = nullableInt64(leaseUntil)
	t.ErrorAt = nullableInt64(errorAt)
	t.StartedAt = nullableInt64(startedAt)
	t.FinishedAt = nullableInt64(finishedAt)
	return &t, nil
}

func (r *SQLiteRepository) Get(videoID int64) (*Detail, error) {
	var d Detail
	var release, duration, uncensored, fetched sql.NullInt64
	var actors, tags string
	var taskStatus, taskErrorCode, taskLastError sql.NullString
	var attempts sql.NullInt64
	var nextRetry, errorAt sql.NullInt64
	err := r.db.QueryRow(`SELECT v.video_id, v.code, v.title, v.studio, v.series, v.release_unix, v.duration_min, v.is_uncensored, v.actors_json, v.tags_json, v.cover_url, v.cover_status, v.provider, v.code_source, v.override_mode, COALESCE(t.status, v.scrape_status), COALESCE(t.attempts, v.attempts), COALESCE(t.last_error, v.last_error), COALESCE(t.next_retry_at, v.next_retry_at), v.fetched_at, t.error_code, t.error_at FROM video_jav v LEFT JOIN jav_scrape_tasks t ON t.video_id=v.video_id WHERE v.video_id=?`, videoID).Scan(&d.VideoID, &d.Code, &d.Title, &d.Studio, &d.Series, &release, &duration, &uncensored, &actors, &tags, &d.CoverURL, &d.CoverStatus, &d.Provider, &d.CodeSource, &d.OverrideMode, &taskStatus, &attempts, &taskLastError, &nextRetry, &fetched, &taskErrorCode, &errorAt)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("get jav detail: %w", err)
	}
	if release.Valid {
		d.ReleaseUnix = &release.Int64
	}
	if duration.Valid {
		n := int(duration.Int64)
		d.DurationMin = &n
	}
	if uncensored.Valid {
		b := uncensored.Int64 != 0
		d.IsUncensored = &b
	}
	if fetched.Valid {
		d.FetchedAt = &fetched.Int64
	}
	if nextRetry.Valid {
		d.NextRetryAt = &nextRetry.Int64
	}
	if errorAt.Valid {
		d.ErrorAt = &errorAt.Int64
	}
	d.ScrapeStatus, d.Attempts, d.LastError, d.ErrorCode = taskStatus.String, int(attempts.Int64), taskLastError.String, taskErrorCode.String
	_ = json.Unmarshal([]byte(actors), &d.Actors)
	_ = json.Unmarshal([]byte(tags), &d.Tags)
	// The desktop and web UIs read len(detail.actors) / len(detail.tags) while
	// rendering the detail panel. A nil slice marshals to json null and crashed
	// both panels, so the API contract is always an array — including for rows
	// written by older builds that stored "null".
	if d.Actors == nil {
		d.Actors = []string{}
	}
	if d.Tags == nil {
		d.Tags = []string{}
	}
	return &d, nil
}

func (r *SQLiteRepository) ClaimTask(videoID int64, token string, now, leaseUntil int64) (*Task, bool, error) {
	res, err := r.db.Exec(`UPDATE jav_scrape_tasks SET status='running', attempts=attempts+1, lease_token=?, lease_until=?, started_at=?, finished_at=NULL, error_code='', last_error='', error_at=NULL, updated_at=? WHERE video_id=? AND attempts < max_attempts AND (status='queued' OR (status='retryable' AND (next_retry_at IS NULL OR next_retry_at<=?)))`, token, leaseUntil, now, now, videoID, now)
	if err != nil {
		return nil, false, fmt.Errorf("claim jav task: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return nil, false, err
	}
	t, err := r.GetTask(videoID)
	return t, true, err
}

func (r *SQLiteRepository) SetTaskStatus(videoID int64, token, status, errorCode, lastError string, nextRetryAt *int64) error {
	lastError = strings.TrimSpace(lastError)
	if len(lastError) > maxErrorLength {
		lastError = lastError[:maxErrorLength]
	}
	now := time.Now().UnixMilli()
	finished := any(nil)
	if status != StatusRunning && status != StatusQueued {
		finished = now
	}
	res, err := r.db.Exec(`UPDATE jav_scrape_tasks SET status=?, error_code=?, last_error=?, error_at=CASE WHEN ?<>'' THEN ? ELSE NULL END, next_retry_at=?, lease_token=NULL, lease_until=NULL, finished_at=?, updated_at=? WHERE video_id=? AND (lease_token=? OR (?='' AND lease_token IS NULL))`, status, errorCode, lastError, lastError, now, nextRetryAt, finished, now, videoID, token, token)
	if err != nil {
		return fmt.Errorf("set jav task status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("jav task lease lost for video %d", videoID)
	}
	return nil
}

func (r *SQLiteRepository) RecoverPending(now int64) ([]Task, error) {
	if _, err := r.db.Exec(`UPDATE jav_scrape_tasks SET status='queued', lease_token=NULL, lease_until=NULL, updated_at=? WHERE status='running' AND lease_until IS NOT NULL AND lease_until<?`, now, now); err != nil {
		return nil, fmt.Errorf("recover expired jav tasks: %w", err)
	}
	// A retryable task is only recoverable while it still has an attempt left.
	// ClaimTask refuses to claim an exhausted task, so re-enqueueing one would
	// spin forever without ever making progress (and flood the log at the
	// retry scan interval). The service moves exhausted tasks to a terminal
	// status; this guard covers rows written before that existed and keeps a
	// future leak from turning into an unbounded loop.
	rows, err := r.db.Query(`SELECT id, video_id, status, force, attempts, max_attempts, next_retry_at, lease_token, lease_until, error_code, last_error, error_at, created_at, updated_at, started_at, finished_at FROM jav_scrape_tasks WHERE status='queued' OR (status='retryable' AND attempts < max_attempts AND next_retry_at IS NOT NULL AND next_retry_at<=?) ORDER BY updated_at, id`, now)
	if err != nil {
		return nil, fmt.Errorf("list recoverable jav tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]Task, 0)
	for rows.Next() {
		var t Task
		var force int
		var nextRetry, leaseUntil, errorAt, startedAt, finishedAt sql.NullInt64
		var leaseToken sql.NullString
		if err := rows.Scan(&t.ID, &t.VideoID, &t.Status, &force, &t.Attempts, &t.MaxAttempts, &nextRetry, &leaseToken, &leaseUntil, &t.ErrorCode, &t.LastError, &errorAt, &t.CreatedAt, &t.UpdatedAt, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		t.Force = force != 0
		t.LeaseToken = leaseToken.String
		t.NextRetryAt = nullableInt64(nextRetry)
		t.LeaseUntil = nullableInt64(leaseUntil)
		t.ErrorAt = nullableInt64(errorAt)
		t.StartedAt = nullableInt64(startedAt)
		t.FinishedAt = nullableInt64(finishedAt)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) SaveSuccess(videoID int64, m Metadata, codeSource string, fetchedAt int64) error {
	actors, _ := json.Marshal(stringList(m.Actors))
	tags, _ := json.Marshal(stringList(m.Tags))
	_, err := r.db.Exec(`UPDATE video_jav SET code=?, title=?, studio=?, series=?, release_unix=?, duration_min=?, is_uncensored=?, actors_json=?, tags_json=?, cover_url=?, provider=?, code_source=?, fetched_at=?, updated_at=? WHERE video_id=?`, m.Code, m.Title, m.Studio, m.Series, m.ReleaseUnix, m.DurationMin, boolInt(m.IsUncensored), actors, tags, m.CoverURL, m.Provider, codeSource, fetchedAt, fetchedAt, videoID)
	if err != nil {
		return fmt.Errorf("save jav metadata: %w", err)
	}
	return nil
}

// stringList returns a non-nil slice so metadata without actors or tags is
// stored as [] rather than null, mirroring the read path in Get.
func stringList(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (r *SQLiteRepository) SetCover(videoID int64, status string) error {
	_, err := r.db.Exec(`UPDATE video_jav SET cover_status=?, updated_at=? WHERE video_id=?`, status, time.Now().UnixMilli(), videoID)
	return err
}
func (r *SQLiteRepository) SetOverride(videoID int64, mode, code string) error {
	if mode != OverrideAuto && mode != OverrideManual && mode != OverrideSkip {
		return fmt.Errorf("invalid override mode %q", mode)
	}
	now := time.Now().UnixMilli()
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin jav override: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	storedCode := ""
	if mode == OverrideManual {
		storedCode = code
	}
	if _, err := tx.Exec(
		`UPDATE video_jav SET override_mode=?, code=?, updated_at=? WHERE video_id=?`,
		mode, storedCode, now, videoID,
	); err != nil {
		return fmt.Errorf("set jav override: %w", err)
	}

	status := StatusQueued
	if mode == OverrideSkip {
		status = StatusDisabled
	}
	if _, err := tx.Exec(
		`UPDATE jav_scrape_tasks SET status=?, force=0, attempts=0, next_retry_at=NULL, lease_token=NULL, lease_until=NULL, error_code='', last_error='', error_at=NULL, started_at=NULL, finished_at=NULL, created_at=?, updated_at=? WHERE video_id=?`,
		status, now, now, videoID,
	); err != nil {
		return fmt.Errorf("reset jav task for override: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit jav override: %w", err)
	}
	return nil
}

// ListActorFilmRefs returns every distinct (scraped name, favorite) pair.
// json_each unnests each cast array, and the GROUP BY collapses a name repeated
// within one cast list. The result is raw: alias merging is applied by the
// service, which owns the resolution rules. Pairs let the service count
// *distinct films* per actor, so a favorite listing two merged stage names is
// counted once. Rows whose actor list does not parse contribute nothing.
func (r *SQLiteRepository) ListActorFilmRefs() ([]ActorFilmRef, error) {
	rows, err := r.db.Query(`
		SELECT actor.value AS name, v.video_id AS video_id
		FROM video_jav v,
		     json_each(CASE WHEN json_valid(v.actors_json) THEN v.actors_json ELSE '[]' END) AS actor
		WHERE actor.value <> ''
		GROUP BY actor.value, v.video_id`)
	if err != nil {
		return nil, fmt.Errorf("list actor film refs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ActorFilmRef, 0)
	for rows.Next() {
		var ref ActorFilmRef
		if err := rows.Scan(&ref.Name, &ref.VideoID); err != nil {
			return nil, fmt.Errorf("scan actor film ref: %w", err)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// ListActorMerges returns every persisted alias → canonical mapping.
func (r *SQLiteRepository) ListActorMerges() ([]ActorMerge, error) {
	rows, err := r.db.Query(`SELECT alias, canonical FROM actor_aliases ORDER BY alias`)
	if err != nil {
		return nil, fmt.Errorf("list actor merges: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ActorMerge, 0)
	for rows.Next() {
		var m ActorMerge
		if err := rows.Scan(&m.Alias, &m.Canonical); err != nil {
			return nil, fmt.Errorf("scan actor merge: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpsertActorMerge records that alias should be shown as canonical. Re-merging
// an alias simply repoints it.
func (r *SQLiteRepository) UpsertActorMerge(alias, canonical string) error {
	_, err := r.db.Exec(`
		INSERT INTO actor_aliases (alias, canonical, created_at) VALUES (?, ?, ?)
		ON CONFLICT(alias) DO UPDATE SET canonical=excluded.canonical`,
		alias, canonical, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("upsert actor merge: %w", err)
	}
	return nil
}

// DeleteActorMerge removes one alias mapping (解除合并).
func (r *SQLiteRepository) DeleteActorMerge(alias string) error {
	if _, err := r.db.Exec(`DELETE FROM actor_aliases WHERE alias = ?`, alias); err != nil {
		return fmt.Errorf("delete actor merge: %w", err)
	}
	return nil
}

// VideoIDsByActorNames returns the ids of every favorite whose scraped cast
// includes any of names, newest first (same order as the library list). The
// comparison is exact, matching how the names were stored.
func (r *SQLiteRepository) VideoIDsByActorNames(names []string) ([]int64, error) {
	if len(names) == 0 {
		return []int64{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	args := make([]any, len(names))
	for i, name := range names {
		args[i] = name
	}
	rows, err := r.db.Query(`
		SELECT vj.video_id
		FROM video_jav vj,
		     json_each(CASE WHEN json_valid(vj.actors_json) THEN vj.actors_json ELSE '[]' END) AS actor,
		     videos v
		WHERE v.id = vj.video_id AND actor.value IN (`+placeholders+`)
		GROUP BY vj.video_id
		ORDER BY v.created_at DESC, v.id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("list videos by actor names: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan actor video id: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListDuplicateCandidates returns every favorite whose 番号 was recognized,
// newest first (library order). Grouping them into duplicate sets is the
// service's job: it owns the file-name part-marker rules, and the pairs here
// stay raw so a row whose name does not spell its code contributes nothing.
func (r *SQLiteRepository) ListDuplicateCandidates() ([]DuplicateCandidate, error) {
	rows, err := r.db.Query(`
		SELECT
			v.id, v.path, v.file_name, v.file_size, v.duration_ms, v.width, v.height,
			v.frame_rate, v.bit_rate, v.video_codec, v.audio_codec, v.thumbnail_path,
			v.created_at, v.updated_at, v.last_opened_at, v.status, v.title, v.notes,
			v.custom_cover_at, v.like_count, v.cover_source, v.scraped_cover_path,
			v.allow_scraped_cover, TRIM(j.code)
		FROM video_jav j
		JOIN videos v ON v.id = j.video_id
		WHERE TRIM(j.code) <> '' AND j.override_mode <> 'skip'
		ORDER BY v.created_at DESC, v.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list duplicate candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]DuplicateCandidate, 0)
	for rows.Next() {
		candidate, err := scanDuplicateCandidate(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan duplicate candidate: %w", err)
		}
		out = append(out, candidate)
	}
	return out, rows.Err()
}

// scanDuplicateCandidate reads one favorite joined with its recognized code,
// mirroring the column order used by ListTaskOverviews.
func scanDuplicateCandidate(scan func(dest ...any) error) (DuplicateCandidate, error) {
	var c DuplicateCandidate
	var durationMs, width, height, bitrate, lastOpened, customCoverAt, allowScrapedCover sql.NullInt64
	var frameRate sql.NullFloat64
	var videoCodec, audioCodec, thumbnailPath, title, notes, coverSource, scrapedCoverPath sql.NullString
	if err := scan(
		&c.Video.ID, &c.Video.Path, &c.Video.FileName, &c.Video.FileSize, &durationMs, &width, &height,
		&frameRate, &bitrate, &videoCodec, &audioCodec, &thumbnailPath,
		&c.Video.CreatedAt, &c.Video.UpdatedAt, &lastOpened, &c.Video.Status, &title, &notes,
		&customCoverAt, &c.Video.LikeCount, &coverSource, &scrapedCoverPath, &allowScrapedCover, &c.Code,
	); err != nil {
		return c, err
	}
	c.Video.DurationMs = durationMs.Int64
	c.Video.Width = int(width.Int64)
	c.Video.Height = int(height.Int64)
	c.Video.FrameRate = frameRate.Float64
	c.Video.Bitrate = bitrate.Int64
	c.Video.VideoCodec = videoCodec.String
	c.Video.AudioCodec = audioCodec.String
	c.Video.ThumbnailPath = thumbnailPath.String
	c.Video.LastOpenedAt = lastOpened.Int64
	c.Video.Title = title.String
	c.Video.Notes = notes.String
	c.Video.CustomCoverAt = customCoverAt.Int64
	c.Video.CoverSource = coverSource.String
	c.Video.ScrapedCoverPath = scrapedCoverPath.String
	c.Video.AllowScrapedCover = allowScrapedCover.Int64 != 0
	return c, nil
}

// ListDuplicateIgnores returns every persisted 查重 dismissal.
func (r *SQLiteRepository) ListDuplicateIgnores() ([]IgnoredDuplicate, error) {
	rows, err := r.db.Query(`SELECT dup_key, video_ids FROM jav_duplicate_ignores`)
	if err != nil {
		return nil, fmt.Errorf("list duplicate ignores: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]IgnoredDuplicate, 0)
	for rows.Next() {
		var ignored IgnoredDuplicate
		if err := rows.Scan(&ignored.Key, &ignored.VideoIDs); err != nil {
			return nil, fmt.Errorf("scan duplicate ignore: %w", err)
		}
		out = append(out, ignored)
	}
	return out, rows.Err()
}

// UpsertDuplicateIgnore records that the user chose to keep every copy in a
// duplicate group (等价于忽略重复提示). Re-dismissing a group whose membership
// changed simply replaces the stored member list.
func (r *SQLiteRepository) UpsertDuplicateIgnore(key, videoIDs string) error {
	_, err := r.db.Exec(`
		INSERT INTO jav_duplicate_ignores (dup_key, video_ids, created_at) VALUES (?, ?, ?)
		ON CONFLICT(dup_key) DO UPDATE SET video_ids=excluded.video_ids, created_at=excluded.created_at`,
		key, videoIDs, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("upsert duplicate ignore: %w", err)
	}
	return nil
}

// DeleteDuplicateIgnore clears a dismissal, so the group is surfaced again.
func (r *SQLiteRepository) DeleteDuplicateIgnore(key string) error {
	if _, err := r.db.Exec(`DELETE FROM jav_duplicate_ignores WHERE dup_key=?`, key); err != nil {
		return fmt.Errorf("delete duplicate ignore: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) Delete(videoID int64) error {
	if _, err := r.db.Exec(`DELETE FROM jav_scrape_tasks WHERE video_id=?`, videoID); err != nil {
		return err
	}
	_, err := r.db.Exec(`DELETE FROM video_jav WHERE video_id=?`, videoID)
	return err
}
func nullableInt64(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}
func boolInt(v *bool) any {
	if v == nil {
		return nil
	}
	if *v {
		return 1
	}
	return 0
}
