package cloud

import (
	"database/sql"
	"errors"
	"fmt"
)

// Repository is the persistence boundary for connections. It deliberately does
// not know about HTTP, secrets or the videos table's model: the service owns
// those rules, and the SQL here stays a thin mapping.
type Repository interface {
	ListConnections() ([]Connection, error)
	GetConnection(id int64) (*Connection, error)
	CreateConnection(*Connection) error
	UpdateConnection(*Connection) error
	DeleteConnection(id int64) error
	CountRemoteVideos(sourceID int64) (int, error)
	SetConnectionEnabled(id int64, enabled bool, at int64) error
	MarkConnectionOK(id int64, at int64) error
	MarkConnectionError(id int64, message string) error
}

type sqliteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository builds the connection repository over the app database.
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepository{db: db}
}

const connectionColumns = `id, name, kind, endpoint, web_url, username, password_enc, root_path,
	verify_tls, enabled, COALESCE(disabled_at, 0), created_at, updated_at, COALESCE(last_ok_at, 0), last_error`

func scanConnection(scan func(...any) error) (*Connection, error) {
	var c Connection
	var verifyTLS, enabled int
	err := scan(&c.ID, &c.Name, &c.Kind, &c.Endpoint, &c.WebURL, &c.Username, &c.PasswordEnc,
		&c.RootPath, &verifyTLS, &enabled, &c.DisabledAt, &c.CreatedAt, &c.UpdatedAt, &c.LastOKAt, &c.LastError)
	if err != nil {
		return nil, err
	}
	c.VerifyTLS = verifyTLS != 0
	c.Enabled = enabled != 0
	return &c, nil
}

func (r *sqliteRepository) ListConnections() ([]Connection, error) {
	rows, err := r.db.Query(`SELECT ` + connectionColumns + ` FROM cloud_connections ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("list cloud connections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]Connection, 0, 4)
	for rows.Next() {
		c, err := scanConnection(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan cloud connection: %w", err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *sqliteRepository) GetConnection(id int64) (*Connection, error) {
	row := r.db.QueryRow(`SELECT `+connectionColumns+` FROM cloud_connections WHERE id = ?`, id)
	c, err := scanConnection(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConnectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get cloud connection: %w", err)
	}
	return c, nil
}

func (r *sqliteRepository) CreateConnection(c *Connection) error {
	res, err := r.db.Exec(`
		INSERT INTO cloud_connections
			(name, kind, endpoint, web_url, username, password_enc, root_path, verify_tls, enabled, disabled_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Kind, c.Endpoint, c.WebURL, c.Username, c.PasswordEnc, c.RootPath, boolInt(c.VerifyTLS),
		boolInt(c.Enabled), nullInt(c.DisabledAt), c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert cloud connection: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("cloud connection id: %w", err)
	}
	c.ID = id
	return nil
}

func (r *sqliteRepository) UpdateConnection(c *Connection) error {
	res, err := r.db.Exec(`
		UPDATE cloud_connections
		SET name = ?, kind = ?, endpoint = ?, web_url = ?, username = ?, password_enc = ?,
		    root_path = ?, verify_tls = ?, enabled = ?, disabled_at = ?, updated_at = ?,
		    last_ok_at = NULL, last_error = ''
		WHERE id = ?`,
		c.Name, c.Kind, c.Endpoint, c.WebURL, c.Username, c.PasswordEnc,
		c.RootPath, boolInt(c.VerifyTLS), boolInt(c.Enabled), nullInt(c.DisabledAt), c.UpdatedAt, c.ID)
	if err != nil {
		return fmt.Errorf("update cloud connection: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConnectionNotFound
	}
	c.LastOKAt = 0
	c.LastError = ""
	return nil
}

func (r *sqliteRepository) DeleteConnection(id int64) error {
	res, err := r.db.Exec(`DELETE FROM cloud_connections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete cloud connection: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConnectionNotFound
	}
	return nil
}

// CountRemoteVideos counts active favorites. Archived rows live in the recycle
// bin and no longer block deleting a retired connection.
func (r *sqliteRepository) CountRemoteVideos(sourceID int64) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM videos WHERE source_id = ? AND deleted_at IS NULL`, sourceID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count cloud favorites: %w", err)
	}
	return n, nil
}

func (r *sqliteRepository) SetConnectionEnabled(id int64, enabled bool, at int64) error {
	var disabled any
	if !enabled {
		disabled = at
	}
	_, err := r.db.Exec(`UPDATE cloud_connections SET enabled = ?, disabled_at = ?, updated_at = ? WHERE id = ?`,
		boolInt(enabled), disabled, at, id)
	if err != nil {
		return fmt.Errorf("set cloud connection enabled: %w", err)
	}
	return nil
}

func (r *sqliteRepository) MarkConnectionOK(id int64, at int64) error {
	_, err := r.db.Exec(`UPDATE cloud_connections SET last_ok_at = ?, last_error = '' WHERE id = ?`, at, id)
	if err != nil {
		return fmt.Errorf("mark cloud connection healthy: %w", err)
	}
	return nil
}

func (r *sqliteRepository) MarkConnectionError(id int64, message string) error {
	_, err := r.db.Exec(`UPDATE cloud_connections SET last_error = ? WHERE id = ?`, message, id)
	if err != nil {
		return fmt.Errorf("mark cloud connection unhealthy: %w", err)
	}
	return nil
}

func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
