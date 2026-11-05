package group

import (
	"database/sql"
	"fmt"
	"time"
)

// GroupRepository isolates persistence from business logic, matching the
// video.VideoRepository pattern.
type GroupRepository interface {
	Create(name string, position int, parentID *int64, at int64) (*Group, error)
	Get(id int64) (*Group, error)
	FindByName(name string) (*Group, error) // case-insensitive, first by display order
	List() ([]Group, error)                 // ordered by position, then id (recursive union counts)
	Rename(id int64, name string, at int64) error
	Delete(id int64) error
	SetPositions(ids []int64) error                      // assign 0.. len-1 in the given order
	SetParent(id int64, parentID *int64, at int64) error // nil parent detaches a group
	Children(parentID int64) ([]int64, error)            // immediate child group ids
	Members(videoID int64) ([]int64, error)              // group ids a video belongs to
	SetMembers(videoID int64, groupIDs []int64) error
	AddVideoToGroup(videoID int64, groupID int64) error
	VideosInGroup(groupID int64) ([]int64, error)     // direct member video ids, ordered by created_at desc
	VideosInGroups(groupIDs []int64) ([]int64, error) // union across groups, distinct, newest first
	ClearVideoMembers(videoID int64) error
	// GroupNamesForVideos returns the group names each video belongs to, keyed
	// by video id. The recycle bin uses it to show where a trashed favorite
	// came from; memberships are retained while trashed.
	GroupNamesForVideos(videoIDs []int64) (map[int64][]string, error)
}

type sqliteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) GroupRepository {
	return &sqliteRepository{db: db}
}

// scanGroup scans a group row produced by groupListSQL, including the nullable
// parent_id column and the member count.
func scanGroup(scan func(dest ...any) error) (*Group, error) {
	var g Group
	var parentID sql.NullInt64
	err := scan(&g.ID, &g.Name, &g.Position, &g.CreatedAt, &g.UpdatedAt, &g.VideoCount, &parentID)
	if err != nil {
		return nil, err
	}
	if parentID.Valid {
		p := parentID.Int64
		g.ParentID = &p
	}
	return &g, nil
}

// groupRow selects a group plus its direct (non-recursive) member count. The
// recursive parent/child union count is computed in List() from the full tree.
const groupListSQL = `
SELECT g.id, g.name, g.position, g.created_at, g.updated_at,
       (SELECT COUNT(*) FROM video_group_members m
          JOIN videos v ON v.id = m.video_id AND v.deleted_at IS NULL
         WHERE m.group_id = g.id) AS video_count,
       g.parent_id
FROM video_groups g`

// nullableInt converts a *int64 into a value acceptable by database/sql as a
// nullable bound parameter (nil → NULL, otherwise the integer value).
func nullableInt(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func (r *sqliteRepository) Create(name string, position int, parentID *int64, at int64) (*Group, error) {
	res, err := r.db.Exec(
		`INSERT INTO video_groups (name, position, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		name, position, nullableInt(parentID), at, at,
	)
	if err != nil {
		return nil, fmt.Errorf("insert group: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return &Group{ID: id, Name: name, Position: position, ParentID: parentID, CreatedAt: at, UpdatedAt: at}, nil
}

func (r *sqliteRepository) Get(id int64) (*Group, error) {
	row := r.db.QueryRow(groupListSQL+` WHERE g.id = ?`, id)
	g, err := scanGroup(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get group: %w", err)
	}
	return g, nil
}

// FindByName returns the first group whose name equals the given name,
// ignoring case (Windows group names are compared case-insensitively). It
// returns ErrNotFound when no group matches. Used for get-or-create semantics.
func (r *sqliteRepository) FindByName(name string) (*Group, error) {
	row := r.db.QueryRow(groupListSQL+` WHERE g.name = ? COLLATE NOCASE ORDER BY g.position ASC, g.id ASC LIMIT 1`, name)
	g, err := scanGroup(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find group by name: %w", err)
	}
	return g, nil
}

func (r *sqliteRepository) List() ([]Group, error) {
	rows, err := r.db.Query(groupListSQL + ` ORDER BY g.position ASC, g.id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	groups := make([]Group, 0, 8)
	for rows.Next() {
		g, err := scanGroup(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		groups = append(groups, *g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	r.applyUnionCounts(groups)
	return groups, nil
}

// applyUnionCounts replaces each group's VideoCount with the recursive number
// of videos in the group and all of its descendants (self ∪ children ∪
// grandchildren …), reflecting the parent/child union rule.
func (r *sqliteRepository) applyUnionCounts(groups []Group) {
	children := make(map[int64][]int64, len(groups))
	for i := range groups {
		if p := groups[i].ParentID; p != nil {
			children[*p] = append(children[*p], groups[i].ID)
		}
	}
	// Load every distinct group's direct member count in one query.
	direct := make(map[int64]int, len(groups))
	args := make([]any, len(groups))
	for i := range groups {
		args[i] = groups[i].ID
	}
	rows, err := r.db.Query(
		`SELECT m.group_id, COUNT(*) FROM video_group_members m
		 JOIN videos v ON v.id = m.video_id AND v.deleted_at IS NULL
		 WHERE m.group_id IN (`+placeholders(len(groups))+`) GROUP BY m.group_id`,
		args...,
	)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var gid int64
			var n int
			if err := rows.Scan(&gid, &n); err == nil {
				direct[gid] = n
			}
		}
	}
	var union func(id int64) int64
	union = func(id int64) int64 {
		total := int64(direct[id])
		for _, c := range children[id] {
			total += union(c)
		}
		return total
	}
	for i := range groups {
		groups[i].VideoCount = union(groups[i].ID)
	}
}

// placeholders returns "?,?,…" with n question marks for an IN clause.
func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	b := make([]byte, 0, n*2)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '?')
	}
	return string(b)
}

func (r *sqliteRepository) Rename(id int64, name string, at int64) error {
	_, err := r.db.Exec(`UPDATE video_groups SET name = ?, updated_at = ? WHERE id = ?`, name, at, id)
	if err != nil {
		return fmt.Errorf("rename group: %w", err)
	}
	return nil
}

func (r *sqliteRepository) Delete(id int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete group: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Detach the group's own children first so they become top-level instead of
	// being orphaned (they keep their videos and their own sub-tree).
	if _, err := tx.Exec(`UPDATE video_groups SET parent_id = NULL, updated_at = ? WHERE parent_id = ?`, time.Now().UnixMilli(), id); err != nil {
		return fmt.Errorf("detach child groups: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM video_group_members WHERE group_id = ?`, id); err != nil {
		return fmt.Errorf("delete group members: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM video_groups WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete group: %w", err)
	}
	return nil
}

// SetParent assigns a group a parent, or detaches it (parentID nil) so it
// becomes a top-level group.
func (r *sqliteRepository) SetParent(id int64, parentID *int64, at int64) error {
	if _, err := r.db.Exec(`UPDATE video_groups SET parent_id = ?, updated_at = ? WHERE id = ?`, nullableInt(parentID), at, id); err != nil {
		return fmt.Errorf("set parent: %w", err)
	}
	return nil
}

// Children returns the immediate child group ids of a group, in display order.
func (r *sqliteRepository) Children(parentID int64) ([]int64, error) {
	rows, err := r.db.Query(
		`SELECT id FROM video_groups WHERE parent_id = ? ORDER BY position ASC, id ASC`, parentID,
	)
	if err != nil {
		return nil, fmt.Errorf("query children: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, 8)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan child id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetPositions renumbers the given groups to 0,1,2,… in list order so the
// sidebar order follows the array passed by the frontend.
func (r *sqliteRepository) SetPositions(ids []int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin reorder groups: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	at := time.Now().UnixMilli()
	for i, id := range ids {
		if _, err := tx.Exec(
			`UPDATE video_groups SET position = ?, updated_at = ? WHERE id = ?`,
			i, at, id,
		); err != nil {
			return fmt.Errorf("reorder group %d: %w", id, err)
		}
	}
	return tx.Commit()
}

func (r *sqliteRepository) Members(videoID int64) ([]int64, error) {
	rows, err := r.db.Query(
		`SELECT group_id FROM video_group_members WHERE video_id = ? ORDER BY group_id ASC`,
		videoID,
	)
	if err != nil {
		return nil, fmt.Errorf("query group memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, 4)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan group id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetMembers replaces a video's memberships with the given group ids. Empty
// input clears them all.
func (r *sqliteRepository) SetMembers(videoID int64, groupIDs []int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin set members: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM video_group_members WHERE video_id = ?`, videoID); err != nil {
		return fmt.Errorf("clear members: %w", err)
	}
	for _, gid := range groupIDs {
		if _, err := tx.Exec(
			`INSERT INTO video_group_members (video_id, group_id) VALUES (?, ?)`,
			videoID, gid,
		); err != nil {
			return fmt.Errorf("insert member: %w", err)
		}
	}
	return tx.Commit()
}

// AddVideoToGroup adds a single video to a group. It is idempotent: if the
// video is already a member, the insert is a no-op (ON CONFLICT ... DO
// NOTHING), so dragging an already-grouped video produces no side effect.
func (r *sqliteRepository) AddVideoToGroup(videoID int64, groupID int64) error {
	if _, err := r.db.Exec(
		`INSERT INTO video_group_members (video_id, group_id) VALUES (?, ?)
		 ON CONFLICT(video_id, group_id) DO NOTHING`,
		videoID, groupID,
	); err != nil {
		return fmt.Errorf("insert member: %w", err)
	}
	return nil
}

func (r *sqliteRepository) VideosInGroup(groupID int64) ([]int64, error) {
	return r.VideosInGroups([]int64{groupID})
}

// VideosInGroups returns the distinct video ids across the given groups
// (recursive-union call site), newest first. Videos are deduplicated across
// overlapping group membership, matching the "videos can belong to both a
// parent and a child group" rule.
func (r *sqliteRepository) VideosInGroups(groupIDs []int64) ([]int64, error) {
	if len(groupIDs) == 0 {
		return []int64{}, nil
	}
	query := `SELECT m.video_id FROM video_group_members m
		 JOIN videos v ON v.id = m.video_id
		 WHERE v.deleted_at IS NULL AND m.group_id IN (` + placeholders(len(groupIDs)) + `)
		 GROUP BY m.video_id
		 ORDER BY MAX(v.created_at) DESC, m.video_id DESC`
	args := make([]any, len(groupIDs))
	for i, id := range groupIDs {
		args[i] = id
	}
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query group videos: %w", err)
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

func (r *sqliteRepository) ClearVideoMembers(videoID int64) error {
	_, err := r.db.Exec(`DELETE FROM video_group_members WHERE video_id = ?`, videoID)
	if err != nil {
		return fmt.Errorf("clear video members: %w", err)
	}
	return nil
}

// GroupNamesForVideos returns each video's group names in sidebar display
// order. It is a single query for the whole recycle-bin page; an empty id list
// is a valid request and costs no query.
func (r *sqliteRepository) GroupNamesForVideos(videoIDs []int64) (map[int64][]string, error) {
	out := make(map[int64][]string, len(videoIDs))
	if len(videoIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(videoIDs))
	for i, id := range videoIDs {
		args[i] = id
	}
	rows, err := r.db.Query(
		`SELECT m.video_id, g.name FROM video_group_members m
		 JOIN video_groups g ON g.id = m.group_id
		 WHERE m.video_id IN (`+placeholders(len(videoIDs))+`)
		 ORDER BY g.position ASC, g.id ASC`, args...,
	)
	if err != nil {
		return nil, fmt.Errorf("query video group names: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var videoID int64
		var name string
		if err := rows.Scan(&videoID, &name); err != nil {
			return nil, fmt.Errorf("scan video group name: %w", err)
		}
		out[videoID] = append(out[videoID], name)
	}
	return out, rows.Err()
}
