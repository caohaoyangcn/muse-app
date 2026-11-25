package video

import (
	"database/sql"
	"fmt"
)

// SQLite persistence for 精彩时刻 (docs/spec/highlight-moments.md §4.3).
//
// The queue in highlights.go is only a list of ids; these rows are what a restart
// reads. Two shapes repeat here on purpose:
//
//   - transitions are conditional UPDATEs whose rows-affected answer is returned
//     to the caller (Claim/Finish/Retry). A false means "the row moved or was
//     deleted under you", and the worker uses it to decide whether to throw away
//     a file it just generated.
//   - reads that feed a user-facing operation are scoped by video_id, so an id
//     belonging to another favorite can never be touched.

const highlightColumns = "id, video_id, kind, start_ms, end_ms, media_path, status, error, interruptions, created_at, updated_at, finished_at"

func scanHighlight(scan func(dest ...any) error) (*Highlight, error) {
	var h Highlight
	err := scan(&h.ID, &h.VideoID, &h.Kind, &h.StartMs, &h.EndMs, &h.MediaPath,
		&h.Status, &h.Error, &h.Interruptions, &h.CreatedAt, &h.UpdatedAt, &h.FinishedAt)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (r *sqliteRepository) CreateHighlight(h *Highlight) error {
	res, err := r.db.Exec(`
		INSERT INTO video_highlights
		    (video_id, kind, start_ms, end_ms, media_path, status, error, interruptions, created_at, updated_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.VideoID, h.Kind, h.StartMs, h.EndMs, h.MediaPath, h.Status, h.Error,
		h.Interruptions, h.CreatedAt, h.UpdatedAt, h.FinishedAt)
	if err != nil {
		return fmt.Errorf("insert highlight: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("highlight last insert id: %w", err)
	}
	h.ID = id
	return nil
}

func (r *sqliteRepository) GetHighlight(id int64) (*Highlight, error) {
	return r.queryHighlight(`SELECT `+highlightColumns+` FROM video_highlights WHERE id = ?`, id)
}

func (r *sqliteRepository) GetVideoHighlight(videoID, id int64) (*Highlight, error) {
	return r.queryHighlight(`SELECT `+highlightColumns+` FROM video_highlights WHERE id = ? AND video_id = ?`, id, videoID)
}

func (r *sqliteRepository) queryHighlight(query string, args ...any) (*Highlight, error) {
	h, err := scanHighlight(r.db.QueryRow(query, args...).Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get highlight: %w", err)
	}
	return h, nil
}

// ListHighlights returns a favorite's moments in the order they appear in the
// video, which is the only order the detail page can make sense of.
func (r *sqliteRepository) ListHighlights(videoID int64) ([]Highlight, error) {
	rows, err := r.db.Query(
		`SELECT `+highlightColumns+` FROM video_highlights WHERE video_id = ? ORDER BY start_ms, id`, videoID)
	if err != nil {
		return nil, fmt.Errorf("list highlights: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Non-nil even for zero rows: a nil slice marshals to JSON null and both
	// clients expect an array.
	list := make([]Highlight, 0, 8)
	for rows.Next() {
		h, err := scanHighlight(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan highlight: %w", err)
		}
		list = append(list, *h)
	}
	return list, rows.Err()
}

func (r *sqliteRepository) ListQueuedHighlightIDs() ([]int64, error) {
	rows, err := r.db.Query(
		`SELECT id FROM video_highlights WHERE status = ? ORDER BY start_ms, id`, HighlightStatusQueued)
	if err != nil {
		return nil, fmt.Errorf("list queued highlights: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ids := make([]int64, 0, 8)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan queued highlight id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListHighlightMediaPaths returns every recorded path, whatever the status: a
// running row's file is half written and must not be swept as an orphan.
func (r *sqliteRepository) ListHighlightMediaPaths() ([]string, error) {
	rows, err := r.db.Query(`SELECT media_path FROM video_highlights WHERE media_path <> ''`)
	if err != nil {
		return nil, fmt.Errorf("list highlight media paths: %w", err)
	}
	defer func() { _ = rows.Close() }()

	paths := make([]string, 0, 16)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("scan highlight media path: %w", err)
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

// ClaimHighlight is the queued → running transition. The status predicate is
// what makes it safe: only one claimant can ever see rows-affected = 1.
func (r *sqliteRepository) ClaimHighlight(id int64) (bool, error) {
	res, err := r.db.Exec(
		`UPDATE video_highlights SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
		HighlightStatusRunning, nowMillis(), id, HighlightStatusQueued)
	if err != nil {
		return false, fmt.Errorf("claim highlight: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *sqliteRepository) FinishHighlight(id int64, status, errMsg string, finishedAt int64) (bool, error) {
	res, err := r.db.Exec(
		`UPDATE video_highlights SET status = ?, error = ?, finished_at = ?, updated_at = ? WHERE id = ?`,
		status, errMsg, finishedAt, nowMillis(), id)
	if err != nil {
		return false, fmt.Errorf("finish highlight: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// SetHighlightMediaPath re-points one row at a new file name. rows-affected
// false means the row is gone, which the caller treats like any other lost row.
func (r *sqliteRepository) SetHighlightMediaPath(id int64, mediaPath string) (bool, error) {
	res, err := r.db.Exec(
		`UPDATE video_highlights SET media_path = ?, updated_at = ? WHERE id = ?`,
		mediaPath, nowMillis(), id)
	if err != nil {
		return false, fmt.Errorf("set highlight media path: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// RetryHighlight re-queues a failed row and clears the previous error. It only
// matches failed rows: a ready row has nothing to retry, and a queued one is
// already waiting.
func (r *sqliteRepository) RetryHighlight(videoID, id int64) (bool, error) {
	res, err := r.db.Exec(`
		UPDATE video_highlights
		SET status = ?, error = '', finished_at = 0, updated_at = ?
		WHERE id = ? AND video_id = ? AND status = ?`,
		HighlightStatusQueued, nowMillis(), id, videoID, HighlightStatusFailed)
	if err != nil {
		return false, fmt.Errorf("retry highlight: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// RecoverHighlights prepares the queue for a new process.
//
// A row left running means the app died while ffmpeg had it: it is bumped and
// re-queued, or failed once the interruptions pile up (a file that takes the app
// down every time must not be retried forever). Rows already queued are simply
// collected. The whole pass is one transaction so two instances cannot both
// claim the same recovery.
func (r *sqliteRepository) RecoverHighlights(maxInterruptions int) ([]int64, int, error) {
	if maxInterruptions < 1 {
		maxInterruptions = 1
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, 0, fmt.Errorf("recover highlights: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	interrupted, err := queryInterruptedHighlights(tx)
	if err != nil {
		return nil, 0, err
	}

	now := nowMillis()
	failed := 0
	for _, row := range interrupted {
		next := row.interruptions + 1
		if next >= maxInterruptions {
			if _, err := tx.Exec(`
				UPDATE video_highlights
				SET status = ?, error = ?, interruptions = ?, finished_at = ?, updated_at = ?
				WHERE id = ?`,
				HighlightStatusFailed, "生成被多次中断", next, now, now, row.id); err != nil {
				return nil, 0, fmt.Errorf("recover highlight %d: %w", row.id, err)
			}
			failed++
			continue
		}
		if _, err := tx.Exec(
			`UPDATE video_highlights SET status = ?, interruptions = ?, updated_at = ? WHERE id = ?`,
			HighlightStatusQueued, next, now, row.id); err != nil {
			return nil, 0, fmt.Errorf("recover highlight %d: %w", row.id, err)
		}
	}

	// Read the queue after the transitions above, so a row that just went back
	// to queued is included and the list is a single, ordered source of work.
	requeue, err := queryHighlightIDs(tx,
		`SELECT id FROM video_highlights WHERE status = ? ORDER BY start_ms, id`, HighlightStatusQueued)
	if err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, fmt.Errorf("recover highlights commit: %w", err)
	}
	return requeue, failed, nil
}

// queryHighlightIDs reads a column of ids inside a transaction.
func queryHighlightIDs(tx *sql.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query highlight ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ids := make([]int64, 0, 8)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan highlight id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// interruptedHighlight is one row that was still running when the process ended,
// with the number of interruptions it has already survived.
type interruptedHighlight struct {
	id            int64
	interruptions int
}

// queryInterruptedHighlights reads the rows left in the running state inside a
// transaction, so the recovery pass rewrites exactly the set it observed.
func queryInterruptedHighlights(tx *sql.Tx) ([]interruptedHighlight, error) {
	rows, err := tx.Query(
		`SELECT id, interruptions FROM video_highlights WHERE status = ?`, HighlightStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("query interrupted highlights: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]interruptedHighlight, 0, 4)
	for rows.Next() {
		var item interruptedHighlight
		if err := rows.Scan(&item.id, &item.interruptions); err != nil {
			return nil, fmt.Errorf("scan interrupted highlight: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// DeleteHighlight removes one row, scoped to its favorite. Deleting a row that
// is already gone is not an error: the client asked for an outcome, not for a
// row count.
func (r *sqliteRepository) DeleteHighlight(videoID, id int64) error {
	if _, err := r.db.Exec(
		`DELETE FROM video_highlights WHERE id = ? AND video_id = ?`, id, videoID,
	); err != nil {
		return fmt.Errorf("delete highlight: %w", err)
	}
	return nil
}
