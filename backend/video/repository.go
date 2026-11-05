package video

import (
	"database/sql"
	"fmt"
	"strings"
)

// VideoRepository isolates persistence from business logic (spec-v1.md §25).
type VideoRepository interface {
	Create(video *Video) error
	// Get returns an active favorite. A trashed row is not found; the trash
	// read paths use GetAny/ListTrashed.
	Get(id int64) (*Video, error)
	// GetAny returns a row in either state. Only the recycle bin and the
	// permanent-delete path use it, because they are about the record itself.
	GetAny(id int64) (*Video, error)
	// GetByPath includes trashed rows: the unique path index belongs to the
	// favorite while it sits in the trash, so an add has to see it and restore
	// the existing record instead of inserting a duplicate.
	GetByPath(path string) (*Video, error)
	// GetByRemote finds a favorite by its cloud connection and WebDAV path.
	// The comparison is BINARY: remote paths are case-sensitive. Like
	// GetByPath it includes trashed rows, so re-adding a removed remote file
	// restores the original favorite.
	GetByRemote(sourceID int64, remotePath string) (*Video, error)
	List() ([]Video, error)
	Search(keyword string) ([]Video, error)
	// Trash soft-deletes an active row: it sets deleted_at/delete_reason and
	// leaves the record, its path, its relations and its generated images in
	// place. Trashing an already-trashed row is a no-op.
	Trash(id int64, reason string, at int64) error
	// Restore clears the trash marker. The record becomes active again with
	// everything it owned still attached.
	Restore(id int64, at int64) error
	// ListTrashed returns the recycle bin, newest removal first.
	ListTrashed() ([]Video, error)
	// ListArchived returns the long-lived archived rows (reason archived).
	ListArchived() ([]Video, error)
	// Delete is the permanent delete used after the retention period or when
	// the user empties the trash.
	Delete(id int64) error
	// ArchiveBySource hides every active favorite of one cloud source in the
	// recycle bin with reason archived. It is one transaction so a disable
	// cannot leave half a source active.
	ArchiveBySource(sourceID int64, sourceName string, at int64) (int, error)
	UpdateStatus(id int64, status string) error
	UpdateMetadata(id int64, durationMs int64, width, height int, frameRate float64, bitrate int64, videoCodec, audioCodec string) error
	UpdateThumbnail(id int64, thumbnailPath string) error
	UpdateThumbnailIfAuto(id int64, thumbnailPath string) (bool, error)
	SetScrapedCover(id int64, path string) error
	SetScrapedCoverAllowed(id int64, allowed bool) error
	ApplyScrapedCoverIfAllowed(id int64, thumbnailPath string) (bool, error)
	SetCoverSource(id int64, source string) error
	UpdateLastOpened(id int64, at int64) error
	ListNeedingMetadata() ([]int64, error) // ids with no duration_ms, frame_rate or bit_rate
	UpdateInfo(id int64, title, notes string) error
	SetCustomCover(id int64, at int64) error
	ClearCustomCover(id int64) error
	// IncrementLikes adds one like to the counter and returns the new count.
	IncrementLikes(id int64) (int64, error)
	// ClearLikes resets the like counter to zero.
	ClearLikes(id int64) error
	// UpdateFingerprint stores a content fingerprint together with the on-disk
	// facts it was computed from, and clears any previous failure.
	UpdateFingerprint(id int64, hash string, version int, size, mtime, at int64) error
	// UpdateFingerprintFailure records that the file could not be read. Any
	// stored fingerprint is cleared, so an unreadable file can never be
	// presented as a confirmed duplicate of another one.
	UpdateFingerprintFailure(id int64, message string, at int64) error
	// UpdateFullHash stores the optional full-file confirmation.
	UpdateFullHash(id int64, hash string, at int64) error
	// ListNeedingFingerprint returns the ids of available videos that have no
	// fingerprint, or one produced by an older recipe.
	ListNeedingFingerprint(version int) ([]int64, error)
	// ListSameSizePeers returns the ids of available videos whose recorded size
	// equals size, excluding excludeID. It gives the same-size peers of a newly
	// added file their first fingerprint (the lazy T1 pass): a scan prunes
	// files whose size is unique, so their fingerprints only become necessary
	// once a second file of that size exists.
	ListSameSizePeers(size int64, excludeID int64) ([]int64, error)
	// ListRelocationCandidates returns the favorites recorded at the same size
	// as a file that was just handed to the app and carrying a fingerprint from
	// the given recipe. Size is the cheap filter behind relocation matching: a
	// moved file keeps its bytes, so only a favorite of the same size can be the
	// one that moved. Rows are not filtered by status — a favorite whose file is
	// gone is an obvious candidate, and one the status refresh has not caught up
	// with yet is decided by the caller, which can test the path.
	ListRelocationCandidates(size int64, version int) ([]Video, error)
	// RelinkPath re-points an existing favorite at the same bytes found under a
	// new path. The row keeps its id and everything the user owns — groups,
	// likes, scraped metadata, title, notes, cover — and gains the on-disk facts
	// the new file reports, so the stored fingerprint still describes it. A
	// previous read failure is cleared and the favorite becomes available again.
	RelinkPath(id int64, path, fileName, hash string, version int, size, mtime, at int64) error
	// ListFingerprinted returns the available favorites that carry a content
	// fingerprint, newest first. It is the read path behind the content dedup
	// worklist.
	ListFingerprinted() ([]Video, error)
	// ListByContentHash returns the available favorites carrying one digest.
	ListByContentHash(hash string) ([]Video, error)
	// ListContentDuplicateIgnores returns every persisted content-duplicate
	// dismissal.
	ListContentDuplicateIgnores() ([]IgnoredContentDuplicate, error)
	// UpsertContentDuplicateIgnore records that the user keeps every member of a
	// content-duplicate group.
	UpsertContentDuplicateIgnore(key, videoIDs string) error
	// DeleteContentDuplicateIgnore clears a dismissal, so the group is surfaced
	// again.
	DeleteContentDuplicateIgnore(key string) error
	// ListVideoIDs returns the ids of every favorite, for reconciling the
	// generated images on disk against the records that own them.
	ListVideoIDs() ([]int64, error)
}

type sqliteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) VideoRepository {
	return &sqliteRepository{db: db}
}

const videoColumns = "id, path, file_name, file_size, duration_ms, width, height, frame_rate, bit_rate, video_codec, audio_codec, thumbnail_path, created_at, updated_at, last_opened_at, status, title, notes, custom_cover_at, like_count, cover_source, scraped_cover_path, allow_scraped_cover, content_hash, hash_version, hash_size, hash_mtime, hash_at, hash_error, full_hash, full_hash_at, source_id, remote_path, deleted_at, delete_reason, archive_source_name"

func scanVideo(scan func(dest ...any) error) (*Video, error) {
	var v Video
	var durationMs, width, height, bitrate, lastOpened, customCoverAt, allowScrapedCover sql.NullInt64
	var hashVersion, hashSize, hashMtime, hashAt, fullHashAt sql.NullInt64
	var deletedAt sql.NullInt64
	var frameRate sql.NullFloat64
	var videoCodec, audioCodec, thumbnailPath, title, notes, coverSource, scrapedCoverPath sql.NullString
	var contentHash, hashError, fullHash, remotePath, deleteReason, archiveSource sql.NullString
	err := scan(&v.ID, &v.Path, &v.FileName, &v.FileSize, &durationMs, &width, &height,
		&frameRate, &bitrate, &videoCodec, &audioCodec, &thumbnailPath, &v.CreatedAt, &v.UpdatedAt, &lastOpened, &v.Status,
		&title, &notes, &customCoverAt, &v.LikeCount, &coverSource, &scrapedCoverPath, &allowScrapedCover,
		&contentHash, &hashVersion, &hashSize, &hashMtime, &hashAt, &hashError, &fullHash, &fullHashAt,
		&v.SourceID, &remotePath, &deletedAt, &deleteReason, &archiveSource)
	if err != nil {
		return nil, err
	}
	v.DurationMs = durationMs.Int64
	v.Width = int(width.Int64)
	v.Height = int(height.Int64)
	v.FrameRate = frameRate.Float64
	v.Bitrate = bitrate.Int64
	v.VideoCodec = videoCodec.String
	v.AudioCodec = audioCodec.String
	v.ThumbnailPath = thumbnailPath.String
	v.LastOpenedAt = lastOpened.Int64
	v.Title = title.String
	v.Notes = notes.String
	v.CustomCoverAt = customCoverAt.Int64
	v.CoverSource = coverSource.String
	v.ScrapedCoverPath = scrapedCoverPath.String
	v.AllowScrapedCover = allowScrapedCover.Int64 != 0
	v.ContentHash = contentHash.String
	v.RemotePath = remotePath.String
	v.DeletedAt = deletedAt.Int64
	v.DeleteReason = deleteReason.String
	v.ArchiveSourceName = archiveSource.String
	v.HashVersion = int(hashVersion.Int64)
	v.HashState = FingerprintState{
		Size:       hashSize.Int64,
		Mtime:      hashMtime.Int64,
		At:         hashAt.Int64,
		Error:      hashError.String,
		FullHash:   fullHash.String,
		FullHashAt: fullHashAt.Int64,
	}
	return &v, nil
}

func (r *sqliteRepository) Create(v *Video) error {
	res, err := r.db.Exec(
		`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status, source_id, remote_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		v.Path, v.FileName, v.FileSize, v.CreatedAt, v.UpdatedAt, v.Status, v.SourceID, v.RemotePath,
	)
	if err != nil {
		return fmt.Errorf("insert video: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	v.ID = id
	// Keep the returned in-memory model aligned with the database default.
	v.AllowScrapedCover = true
	return nil
}

func (r *sqliteRepository) Get(id int64) (*Video, error) {
	row := r.db.QueryRow(`SELECT `+videoColumns+` FROM videos WHERE id = ? AND deleted_at IS NULL`, id)
	v, err := scanVideo(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get video: %w", err)
	}
	return v, nil
}

// GetAny is Get without the active-row filter. The recycle bin and the
// permanent-delete path need to read a trashed row so they can restore or purge
// it; every other caller must not see it.
func (r *sqliteRepository) GetAny(id int64) (*Video, error) {
	row := r.db.QueryRow(`SELECT `+videoColumns+` FROM videos WHERE id = ?`, id)
	v, err := scanVideo(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get video: %w", err)
	}
	return v, nil
}

func (r *sqliteRepository) GetByPath(path string) (*Video, error) {
	row := r.db.QueryRow(`SELECT `+videoColumns+` FROM videos WHERE path = ?`, path)
	v, err := scanVideo(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get video by path: %w", err)
	}
	return v, nil
}

func (r *sqliteRepository) GetByRemote(sourceID int64, remotePath string) (*Video, error) {
	row := r.db.QueryRow(`SELECT `+videoColumns+` FROM videos WHERE source_id = ? AND remote_path = ?`, sourceID, remotePath)
	v, err := scanVideo(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get video by remote path: %w", err)
	}
	return v, nil
}

func (r *sqliteRepository) List() ([]Video, error) {
	return r.query(`SELECT ` + videoColumns + ` FROM videos WHERE deleted_at IS NULL ORDER BY created_at DESC, id DESC`)
}

func (r *sqliteRepository) Search(keyword string) ([]Video, error) {
	pattern := "%" + escapeLike(strings.TrimSpace(keyword)) + "%"
	return r.query(
		`SELECT `+videoColumns+` FROM videos
		 WHERE deleted_at IS NULL
		   AND (file_name LIKE ? ESCAPE '\' OR path LIKE ? ESCAPE '\' OR remote_path LIKE ? ESCAPE '\')
		 ORDER BY created_at DESC, id DESC`,
		pattern, pattern, pattern,
	)
}

func (r *sqliteRepository) query(sqlText string, args ...any) ([]Video, error) {
	rows, err := r.db.Query(sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("query videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	// Non-nil even for zero rows: a nil slice marshals to JSON null and the
	// frontend expects an array.
	videos := make([]Video, 0, 16)
	for rows.Next() {
		v, err := scanVideo(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan video: %w", err)
		}
		videos = append(videos, *v)
	}
	return videos, rows.Err()
}

// ArchiveBySource moves every active favorite of a source into the recycle bin
// with the archived reason. The conditional update makes it idempotent: rows
// already trashed keep their original reason and timestamp.
func (r *sqliteRepository) ArchiveBySource(sourceID int64, sourceName string, at int64) (int, error) {
	res, err := r.db.Exec(`
		UPDATE videos
		SET deleted_at = ?, delete_reason = ?, archive_source_name = ?, updated_at = ?
		WHERE source_id = ? AND deleted_at IS NULL`,
		at, TrashReasonArchived, sourceName, at, sourceID)
	if err != nil {
		return 0, fmt.Errorf("archive cloud source: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// Trash soft-deletes an active favorite. The condition keeps the operation
// idempotent: a row already in the trash keeps its original removal moment and
// reason, so the retention clock cannot be reset by a stale client.
func (r *sqliteRepository) Trash(id int64, reason string, at int64) error {
	_, err := r.db.Exec(
		`UPDATE videos SET deleted_at = ?, delete_reason = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		at, reason, at, id,
	)
	if err != nil {
		return fmt.Errorf("trash video: %w", err)
	}
	return nil
}

// Restore clears the recycle-bin marker and nothing else.
func (r *sqliteRepository) Restore(id int64, at int64) error {
	_, err := r.db.Exec(
		`UPDATE videos SET deleted_at = NULL, delete_reason = '', updated_at = ? WHERE id = ?`,
		at, id,
	)
	if err != nil {
		return fmt.Errorf("restore video: %w", err)
	}
	return nil
}

// ListTrashed reads the recycle bin. The newest removal comes first because
// that is the one a user who just changed their mind is looking for.
func (r *sqliteRepository) ListTrashed() ([]Video, error) {
	return r.query(`SELECT `+videoColumns+` FROM videos WHERE deleted_at IS NOT NULL AND delete_reason <> ? ORDER BY deleted_at DESC, id DESC`, TrashReasonArchived)
}

func (r *sqliteRepository) ListArchived() ([]Video, error) {
	return r.query(`SELECT `+videoColumns+` FROM videos WHERE deleted_at IS NOT NULL AND delete_reason = ? ORDER BY deleted_at DESC, id DESC`, TrashReasonArchived)
}

func (r *sqliteRepository) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM videos WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete video: %w", err)
	}
	return nil
}

func (r *sqliteRepository) UpdateStatus(id int64, status string) error {
	_, err := r.db.Exec(`UPDATE videos SET status = ?, updated_at = ? WHERE id = ?`, status, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func (r *sqliteRepository) UpdateMetadata(id int64, durationMs int64, width, height int, frameRate float64, bitrate int64, videoCodec, audioCodec string) error {
	_, err := r.db.Exec(
		`UPDATE videos SET duration_ms = ?, width = ?, height = ?, frame_rate = ?, bit_rate = ?, video_codec = ?, audio_codec = ?, updated_at = ? WHERE id = ?`,
		durationMs, width, height, frameRate, bitrate, videoCodec, audioCodec, nowMillis(), id,
	)
	if err != nil {
		return fmt.Errorf("update metadata: %w", err)
	}
	return nil
}

func (r *sqliteRepository) UpdateThumbnail(id int64, thumbnailPath string) error {
	_, err := r.db.Exec(
		`UPDATE videos SET thumbnail_path = ?, updated_at = ? WHERE id = ?`,
		thumbnailPath, nowMillis(), id,
	)
	if err != nil {
		return fmt.Errorf("update thumbnail: %w", err)
	}
	return nil
}

func (r *sqliteRepository) UpdateThumbnailIfAuto(id int64, thumbnailPath string) (bool, error) {
	res, err := r.db.Exec(`UPDATE videos SET thumbnail_path = ?, cover_source = 'auto_frame', updated_at = ? WHERE id = ? AND cover_source = 'auto_frame' AND custom_cover_at IS NULL`, thumbnailPath, nowMillis(), id)
	if err != nil {
		return false, fmt.Errorf("update automatic thumbnail: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *sqliteRepository) SetScrapedCover(id int64, path string) error {
	_, err := r.db.Exec(`UPDATE videos SET scraped_cover_path = ?, updated_at = ? WHERE id = ?`, path, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("set scraped cover: %w", err)
	}
	return nil
}

// SetScrapedCoverAllowed stores the per-video cover preference. Enabling the
// preference immediately promotes an already-downloaded scraped cover when
// one exists; disabling it leaves the current cover untouched.
func (r *sqliteRepository) SetScrapedCoverAllowed(id int64, allowed bool) error {
	allow := 0
	if allowed {
		allow = 1
	}
	_, err := r.db.Exec(`
		UPDATE videos
		SET allow_scraped_cover = ?,
			thumbnail_path = CASE WHEN ? = 1 AND COALESCE(scraped_cover_path, '') <> '' THEN scraped_cover_path ELSE thumbnail_path END,
			cover_source = CASE WHEN ? = 1 AND COALESCE(scraped_cover_path, '') <> '' THEN 'scraped' ELSE cover_source END,
			custom_cover_at = CASE WHEN ? = 1 AND COALESCE(scraped_cover_path, '') <> '' THEN NULL ELSE custom_cover_at END,
			updated_at = ?
		WHERE id = ?`, allow, allow, allow, allow, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("set scraped cover preference: %w", err)
	}
	return nil
}

// ApplyScrapedCoverIfAllowed promotes a freshly downloaded cover only while
// the video's current preference still allows it. The conditional update also
// prevents a stale scrape worker from replacing a cover after the user opted
// out.
func (r *sqliteRepository) ApplyScrapedCoverIfAllowed(id int64, thumbnailPath string) (bool, error) {
	res, err := r.db.Exec(`
		UPDATE videos
		SET thumbnail_path = ?, cover_source = 'scraped', custom_cover_at = NULL, updated_at = ?
		WHERE id = ? AND allow_scraped_cover = 1`, thumbnailPath, nowMillis(), id)
	if err != nil {
		return false, fmt.Errorf("apply scraped cover: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *sqliteRepository) SetCoverSource(id int64, source string) error {
	_, err := r.db.Exec(`UPDATE videos SET cover_source = ?, updated_at = ? WHERE id = ?`, source, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("set cover source: %w", err)
	}
	return nil
}

func (r *sqliteRepository) UpdateLastOpened(id int64, at int64) error {
	_, err := r.db.Exec(`UPDATE videos SET last_opened_at = ? WHERE id = ?`, at, id)
	if err != nil {
		return fmt.Errorf("update last opened: %w", err)
	}
	return nil
}

// UpdateInfo persists the user's custom title/notes. file_name stays immutable;
// an empty title means "display the file name".
func (r *sqliteRepository) UpdateInfo(id int64, title, notes string) error {
	_, err := r.db.Exec(
		`UPDATE videos SET title = ?, notes = ?, updated_at = ? WHERE id = ?`,
		title, notes, nowMillis(), id,
	)
	if err != nil {
		return fmt.Errorf("update info: %w", err)
	}
	return nil
}

// SetCustomCover marks the cover as user-picked (at = unix millis).
func (r *sqliteRepository) SetCustomCover(id int64, at int64) error {
	_, err := r.db.Exec(`UPDATE videos SET custom_cover_at = ?, updated_at = ? WHERE id = ?`, at, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("set custom cover: %w", err)
	}
	return nil
}

// ClearCustomCover reverts the cover to the automatic one (the next processor
// run regenerates the image file).
func (r *sqliteRepository) ClearCustomCover(id int64) error {
	_, err := r.db.Exec(`UPDATE videos SET custom_cover_at = NULL, updated_at = ? WHERE id = ?`, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("clear custom cover: %w", err)
	}
	return nil
}

// IncrementLikes bumps the like counter by one and returns the new value. It
// reports ErrNotFound when no such video exists.
func (r *sqliteRepository) IncrementLikes(id int64) (int64, error) {
	var count int64
	err := r.db.QueryRow(
		`UPDATE videos SET like_count = like_count + 1, updated_at = ? WHERE id = ? RETURNING like_count`,
		nowMillis(), id,
	).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("increment likes: %w", err)
	}
	return count, nil
}

// ClearLikes resets the like counter to zero.
func (r *sqliteRepository) ClearLikes(id int64) error {
	res, err := r.db.Exec(`UPDATE videos SET like_count = 0, updated_at = ? WHERE id = ?`, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("clear likes: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListNeedingMetadata returns ids of video rows that are missing duration,
// frame rate, bit rate or the video codec — i.e. favorites recorded before those
// metadata fields existed. This powers the post-migration backfill that re-runs
// ffprobe on them.
func (r *sqliteRepository) ListNeedingMetadata() ([]int64, error) {
	rows, err := r.db.Query(
		`SELECT id FROM videos
		 WHERE deleted_at IS NULL
		   AND (duration_ms IS NULL OR duration_ms <= 0
		    OR frame_rate IS NULL OR frame_rate <= 0
		        OR bit_rate IS NULL OR bit_rate <= 0
		        OR video_codec IS NULL OR video_codec = '')`,
	)
	if err != nil {
		return nil, fmt.Errorf("query videos needing metadata: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, 16)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan video id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UpdateFingerprint stores a freshly computed content fingerprint. size and
// mtime are the on-disk facts the digest was taken from, not the values cached
// on the row: they are what a later verification compares against.
//
// Writing a fingerprint also repairs file_size from that same fresh fact and
// drops any full-file confirmation, which described the digest being replaced.
func (r *sqliteRepository) UpdateFingerprint(id int64, hash string, version int, size, mtime, at int64) error {
	_, err := r.db.Exec(`
		UPDATE videos
		SET content_hash = ?, hash_version = ?, hash_size = ?, hash_mtime = ?, hash_at = ?, hash_error = '',
		    file_size = ?, full_hash = NULL, full_hash_at = NULL, updated_at = ?
		WHERE id = ?`, hash, version, size, mtime, at, size, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("update fingerprint: %w", err)
	}
	return nil
}

// UpdateFingerprintFailure records an unreadable file (missing, detached drive,
// permission denied). The fingerprint is cleared rather than kept: a file the
// app cannot read must never be reported as a confirmed duplicate, and clearing
// it also schedules a retry on the next scan.
func (r *sqliteRepository) UpdateFingerprintFailure(id int64, message string, at int64) error {
	_, err := r.db.Exec(`
		UPDATE videos
		SET content_hash = NULL, hash_version = NULL, hash_error = ?, hash_at = ?, updated_at = ?
		WHERE id = ?`, message, at, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("update fingerprint failure: %w", err)
	}
	return nil
}

// UpdateFullHash stores the full-file confirmation for one video.
func (r *sqliteRepository) UpdateFullHash(id int64, hash string, at int64) error {
	_, err := r.db.Exec(
		`UPDATE videos SET full_hash = ?, full_hash_at = ?, updated_at = ? WHERE id = ?`,
		hash, at, nowMillis(), id,
	)
	if err != nil {
		return fmt.Errorf("update full hash: %w", err)
	}
	return nil
}

// ListNeedingFingerprint returns available favorites with no fingerprint or one
// taken with an older recipe. Missing files are skipped: hashing them can only
// fail.
func (r *sqliteRepository) ListNeedingFingerprint(version int) ([]int64, error) {
	rows, err := r.db.Query(
		`SELECT id FROM videos
		 WHERE deleted_at IS NULL AND source_id = 0 AND status = ?
		   AND (content_hash IS NULL OR content_hash = '' OR hash_version IS NULL OR hash_version <> ?)
		 ORDER BY id`, StatusAvailable, version,
	)
	if err != nil {
		return nil, fmt.Errorf("query videos needing fingerprint: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, 16)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan video id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListSameSizePeers returns available favorites recorded at the same size as a
// file that was just added. Peers that already carry a current fingerprint are
// returned too — the fingerprinter skips them without reading, which keeps this
// query simple and the skip decision in one place.
func (r *sqliteRepository) ListSameSizePeers(size int64, excludeID int64) ([]int64, error) {
	rows, err := r.db.Query(
		`SELECT id FROM videos WHERE deleted_at IS NULL AND source_id = 0 AND file_size = ? AND id <> ? AND status = ? ORDER BY id`,
		size, excludeID, StatusAvailable,
	)
	if err != nil {
		return nil, fmt.Errorf("query same-size peers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, 4)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan same-size peer id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListRelocationCandidates returns the favorites recorded at one size, oldest
// first, that carry a current fingerprint. Status is deliberately not part of
// the query: the caller decides which of these rows has really lost its file,
// which is a question about the disk and not about the row.
func (r *sqliteRepository) ListRelocationCandidates(size int64, version int) ([]Video, error) {
	return r.query(`SELECT `+videoColumns+` FROM videos
		WHERE source_id = 0 AND file_size = ? AND content_hash IS NOT NULL AND content_hash <> '' AND hash_version = ?
		ORDER BY created_at ASC, id ASC`, size, version)
}

// RelinkPath points an existing favorite at the bytes found under another path.
// It is a single statement, so a row can never be caught holding a new path with
// the vanished file's facts.
//
// content_hash and hash_version are written back unchanged on purpose: the digest
// describes exactly the bytes now sitting at path, which is the whole reason the
// favorite is being re-pointed. full_hash is kept for the same reason — it was
// the full-file confirmation of those same bytes — and so is every user-owned
// column, because none of them describes the path.
func (r *sqliteRepository) RelinkPath(id int64, path, fileName, hash string, version int, size, mtime, at int64) error {
	_, err := r.db.Exec(`
		UPDATE videos
		SET path = ?, file_name = ?, status = ?, file_size = ?,
		    content_hash = ?, hash_version = ?, hash_size = ?, hash_mtime = ?, hash_at = ?, hash_error = '',
		    deleted_at = NULL, delete_reason = '',
		    updated_at = ?
		WHERE id = ?`,
		path, fileName, StatusAvailable, size, hash, version, size, mtime, at, nowMillis(), id)
	if err != nil {
		return fmt.Errorf("relink video path: %w", err)
	}
	return nil
}

// ListFingerprinted returns the available favorites that carry a content
// fingerprint, newest first. It is the read path behind the content dedup
// worklist: the grouping itself happens in Go, because each group needs its
// members as full rows.
func (r *sqliteRepository) ListFingerprinted() ([]Video, error) {
	return r.query(`SELECT `+videoColumns+` FROM videos
		WHERE deleted_at IS NULL AND source_id = 0 AND status = ? AND content_hash IS NOT NULL AND content_hash <> ''
		ORDER BY created_at DESC, id DESC`, StatusAvailable)
}

// ListByContentHash returns the available favorites carrying one digest.
func (r *sqliteRepository) ListByContentHash(hash string) ([]Video, error) {
	return r.query(`SELECT `+videoColumns+` FROM videos
		WHERE deleted_at IS NULL AND source_id = 0 AND content_hash = ? AND status = ?
		ORDER BY created_at DESC, id DESC`, hash, StatusAvailable)
}

// ListContentDuplicateIgnores returns every persisted content-duplicate
// dismissal.
func (r *sqliteRepository) ListContentDuplicateIgnores() ([]IgnoredContentDuplicate, error) {
	rows, err := r.db.Query(`SELECT dup_key, video_ids FROM content_duplicate_ignores`)
	if err != nil {
		return nil, fmt.Errorf("list content duplicate ignores: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]IgnoredContentDuplicate, 0)
	for rows.Next() {
		var ignored IgnoredContentDuplicate
		if err := rows.Scan(&ignored.Key, &ignored.VideoIDs); err != nil {
			return nil, fmt.Errorf("scan content duplicate ignore: %w", err)
		}
		out = append(out, ignored)
	}
	return out, rows.Err()
}

// UpsertContentDuplicateIgnore records that the user keeps every member of a
// content-duplicate group (保留全部). Re-dismissing a group whose membership
// changed replaces the stored member list.
func (r *sqliteRepository) UpsertContentDuplicateIgnore(key, videoIDs string) error {
	_, err := r.db.Exec(`
		INSERT INTO content_duplicate_ignores (dup_key, video_ids, created_at) VALUES (?, ?, ?)
		ON CONFLICT(dup_key) DO UPDATE SET video_ids=excluded.video_ids, created_at=excluded.created_at`,
		key, videoIDs, nowMillis())
	if err != nil {
		return fmt.Errorf("upsert content duplicate ignore: %w", err)
	}
	return nil
}

// DeleteContentDuplicateIgnore clears a dismissal, so the group is surfaced
// again.
func (r *sqliteRepository) DeleteContentDuplicateIgnore(key string) error {
	if _, err := r.db.Exec(`DELETE FROM content_duplicate_ignores WHERE dup_key=?`, key); err != nil {
		return fmt.Errorf("delete content duplicate ignore: %w", err)
	}
	return nil
}

// ListVideoIDs returns the id of every favorite. It is deliberately column-only
// and unfiltered: the sweep that deletes orphaned images needs the whole set,
// and reading paths or metadata would make a startup pass touch far more than it
// has to.
func (r *sqliteRepository) ListVideoIDs() ([]int64, error) {
	rows, err := r.db.Query(`SELECT id FROM videos`)
	if err != nil {
		return nil, fmt.Errorf("list video ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, 1024)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan video id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// escapeLike escapes LIKE wildcards so user keywords match literally.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
