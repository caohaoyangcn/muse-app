package watch

import (
	"database/sql"
	"fmt"
)

// Repository isolates persistence from business logic, mirroring the other
// domains (video, group).
type Repository interface {
	Create(dir *WatchedDir) error
	Get(id int64) (*WatchedDir, error)
	GetByPath(path string) (*WatchedDir, error)
	List() ([]WatchedDir, error)
	Delete(id int64) error
	MarkScanned(id int64, at int64) error
}

type sqliteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository builds the SQLite-backed repository.
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepository{db: db}
}

const dirColumns = "id, path, created_at, last_scan_at"

func scanDir(scan func(dest ...any) error) (*WatchedDir, error) {
	var d WatchedDir
	var lastScan sql.NullInt64
	if err := scan(&d.ID, &d.Path, &d.CreatedAt, &lastScan); err != nil {
		return nil, err
	}
	d.LastScanAt = lastScan.Int64
	return &d, nil
}

func (r *sqliteRepository) Create(d *WatchedDir) error {
	res, err := r.db.Exec(
		`INSERT INTO watched_dirs (path, created_at) VALUES (?, ?)`,
		d.Path, d.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert watched dir: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	d.ID = id
	return nil
}

func (r *sqliteRepository) Get(id int64) (*WatchedDir, error) {
	row := r.db.QueryRow(`SELECT `+dirColumns+` FROM watched_dirs WHERE id = ?`, id)
	d, err := scanDir(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get watched dir: %w", err)
	}
	return d, nil
}

func (r *sqliteRepository) GetByPath(path string) (*WatchedDir, error) {
	row := r.db.QueryRow(`SELECT `+dirColumns+` FROM watched_dirs WHERE path = ?`, path)
	d, err := scanDir(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get watched dir by path: %w", err)
	}
	return d, nil
}

func (r *sqliteRepository) List() ([]WatchedDir, error) {
	rows, err := r.db.Query(`SELECT ` + dirColumns + ` FROM watched_dirs ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list watched dirs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []WatchedDir{}
	for rows.Next() {
		d, err := scanDir(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan watched dir: %w", err)
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (r *sqliteRepository) Delete(id int64) error {
	res, err := r.db.Exec(`DELETE FROM watched_dirs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete watched dir: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteRepository) MarkScanned(id int64, at int64) error {
	if _, err := r.db.Exec(`UPDATE watched_dirs SET last_scan_at = ? WHERE id = ?`, at, id); err != nil {
		return fmt.Errorf("mark watched dir scanned: %w", err)
	}
	return nil
}
