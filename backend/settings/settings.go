// Package settings persists small key/value user preferences in the app
// database (the "settings" table, added in schema v5): the LAN server port,
// encrypted provider cookies, the scraping proxy address, and one-off first-run
// flags. The store is generic so new preferences can reuse it without another
// migration.
package settings

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"muse-app/backend/netproxy"
	"muse-app/backend/windowstate"
)

// DefaultServerPort is the non-well-known fixed port used by the LAN server
// until the user configures one in Settings > 服务器. Chosen above the IANA
// well-known range (0–1023) and clear of common app ports (8080/3000/5200…) so
// collisions are unlikely on a typical PC.
const DefaultServerPort = 8899

const (
	keyServerPort      = "server_port"
	keyScrapeProxy     = "scrape_proxy"
	minPort            = 1
	maxPort            = 65535
	keyJavBusCookie    = "javbus_cookie_dpapi"
	keyFC2Cookie       = "fc2cmadb_cookie_dpapi"
	keyWindowPlacement = "window_placement"
	keyTourSeen        = "tour_seen"
	// Cookie headers are normally much smaller than this. The limit prevents
	// accidentally persisting an unbounded paste while allowing complete
	// browser Cookie headers with auxiliary session values.
	maxJavBusCookieLength = 16 * 1024
	maxFC2CookieLength    = maxJavBusCookieLength
)

// ErrSecretStorageUnsupported indicates that this build cannot persist
// secrets securely. There is intentionally no plaintext fallback.
var ErrSecretStorageUnsupported = errors.New("cookie secret storage is unsupported on this platform")

// Store reads and writes settings. It is safe for concurrent use (SQLite is
// opened with a single shared connection).
type Store struct {
	db *sql.DB
}

// New creates a Store backed by the app database.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// get returns the raw string value for a key or "" when it is unset.
func (s *Store) get(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read setting %q: %w", key, err)
	}
	return value, nil
}

// set upserts a value for a key.
func (s *Store) set(key, value string) error {
	if _, err := s.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	); err != nil {
		return fmt.Errorf("write setting %q: %w", key, err)
	}
	return nil
}

// ServerPort returns the configured LAN server port, falling back to
// DefaultServerPort when unset or stored invalid.
func (s *Store) ServerPort() (int, error) {
	raw, err := s.get(keyServerPort)
	if err != nil {
		return DefaultServerPort, err
	}
	if raw == "" {
		return DefaultServerPort, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || !ValidPort(port) {
		// Corrupt/out-of-range value: ignore and use the default. The value is
		// left in place so we don't silently overwrite a future user edit; the
		// caller (settings UI) still shows the effective default.
		return DefaultServerPort, nil
	}
	return port, nil
}

// SetServerPort validates and persists the LAN server port. It returns an
// error (with a user-facing Chinese message) when out of range.
func (s *Store) SetServerPort(port int) error {
	if !ValidPort(port) {
		return fmt.Errorf("端口必须在 %d-%d 之间", minPort, maxPort)
	}
	if err := s.set(keyServerPort, strconv.Itoa(port)); err != nil {
		return err
	}
	return nil
}

// ValidPort reports whether a port number is an allowed TCP port (1–65535;
// 0 is excluded because it means "let the OS pick a free port").
func ValidPort(port int) bool {
	return port >= minPort && port <= maxPort
}

// ScrapeProxy returns the configured scraping proxy address in canonical form,
// or an empty string when scraping should dial directly (still honoring the
// HTTP_PROXY/HTTPS_PROXY environment variables).
func (s *Store) ScrapeProxy() (string, error) {
	raw, err := s.get(keyScrapeProxy)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(raw), nil
}

// SetScrapeProxy validates and persists the scraping proxy address. An empty
// value removes the setting; the backend then falls back to the environment.
// Invalid addresses are rejected so a typo never silently disables scraping.
func (s *Store) SetScrapeProxy(raw string) error {
	u, err := netproxy.Parse(raw)
	if err != nil {
		return err
	}
	if u == nil {
		if _, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, keyScrapeProxy); err != nil {
			return fmt.Errorf("clear scraping proxy: %w", err)
		}
		return nil
	}
	return s.set(keyScrapeProxy, u.String())
}

// WindowPlacement returns the desktop window geometry saved by the previous
// run. ok is false when nothing usable is stored: an absent, corrupt or
// out-of-range value simply leaves the window at its default placement.
func (s *Store) WindowPlacement() (windowstate.Placement, bool, error) {
	raw, err := s.get(keyWindowPlacement)
	if err != nil {
		return windowstate.Placement{}, false, err
	}
	if raw == "" {
		return windowstate.Placement{}, false, nil
	}
	var placement windowstate.Placement
	if err := json.Unmarshal([]byte(raw), &placement); err != nil {
		return windowstate.Placement{}, false, nil
	}
	if !placement.Valid() {
		return windowstate.Placement{}, false, nil
	}
	return placement, true, nil
}

// SetWindowPlacement persists the desktop window geometry for the next run.
func (s *Store) SetWindowPlacement(placement windowstate.Placement) error {
	if !placement.Valid() {
		return fmt.Errorf("invalid window placement: %+v", placement)
	}
	raw, err := json.Marshal(placement)
	if err != nil {
		return fmt.Errorf("encode window placement: %w", err)
	}
	return s.set(keyWindowPlacement, string(raw))
}

// TourSeen reports whether the first-run guided tour has already played.
//
// The flag lives in the database instead of the webview's localStorage: that
// profile is disposable, and a corrupted LevelDB log silently drops every later
// write, which would replay the welcome on every launch. A one-time greeting is
// app state, so it is stored where the rest of the app state lives.
func (s *Store) TourSeen() (bool, error) {
	raw, err := s.get(keyTourSeen)
	if err != nil {
		return false, err
	}
	return raw == "1", nil
}

// MarkTourSeen records that the guided tour must not open automatically again.
// It is written for every way of leaving the tour (finished, skipped, Esc).
func (s *Store) MarkTourSeen() error {
	return s.set(keyTourSeen, "1")
}

// GetJavBusCookie returns the decrypted JavBus Cookie, or an empty string when
// no Cookie has been configured. The database contains only DPAPI-protected,
// base64-encoded data on supported platforms.
func (s *Store) GetJavBusCookie() (string, error) {
	return s.getSecretCookie(keyJavBusCookie, "JavBus")
}

// SetJavBusCookie validates and securely persists a JavBus Cookie. Leading
// and trailing whitespace is removed; CR/LF and oversized values are rejected
// to prevent malformed or header-injection input.
func (s *Store) SetJavBusCookie(cookie string) error {
	return s.setSecretCookie(keyJavBusCookie, "JavBus", cookie, maxJavBusCookieLength)
}

// HasJavBusCookie reports whether encrypted JavBus Cookie data is stored.
func (s *Store) HasJavBusCookie() (bool, error) {
	return s.hasSecretCookie(keyJavBusCookie)
}

// ClearJavBusCookie removes the persisted Cookie.
func (s *Store) ClearJavBusCookie() error {
	return s.clearSecretCookie(keyJavBusCookie, "JavBus")
}

// GetFC2Cookie returns the decrypted FC2CMADB Cookie without exposing it to
// callers that only need configuration status.
func (s *Store) GetFC2Cookie() (string, error) {
	return s.getSecretCookie(keyFC2Cookie, "FC2CMADB")
}

// SetFC2Cookie validates and securely persists an FC2CMADB Cookie.
func (s *Store) SetFC2Cookie(cookie string) error {
	return s.setSecretCookie(keyFC2Cookie, "FC2CMADB", cookie, maxFC2CookieLength)
}

// HasFC2Cookie reports whether encrypted FC2CMADB Cookie data is stored.
func (s *Store) HasFC2Cookie() (bool, error) {
	return s.hasSecretCookie(keyFC2Cookie)
}

// ClearFC2Cookie removes the persisted FC2CMADB Cookie.
func (s *Store) ClearFC2Cookie() error {
	return s.clearSecretCookie(keyFC2Cookie, "FC2CMADB")
}

func (s *Store) getSecretCookie(key, name string) (string, error) {
	if err := ensureSecretStorage(); err != nil {
		return "", err
	}
	raw, err := s.get(key)
	if err != nil {
		return "", err
	}
	if raw == "" {
		return "", nil
	}
	cookie, err := unprotectSecret(raw)
	if err != nil {
		return "", fmt.Errorf("read %s Cookie: %w", name, err)
	}
	return cookie, nil
}

func (s *Store) setSecretCookie(key, name, cookie string, maxLength int) error {
	cookie, err := normalizeCookie(cookie, name, maxLength)
	if err != nil {
		return err
	}
	if err := ensureSecretStorage(); err != nil {
		return err
	}
	protected, err := protectSecret(cookie)
	if err != nil {
		return fmt.Errorf("protect %s Cookie: %w", name, err)
	}
	return s.set(key, protected)
}

func (s *Store) hasSecretCookie(key string) (bool, error) {
	if err := ensureSecretStorage(); err != nil {
		return false, err
	}
	raw, err := s.get(key)
	if err != nil {
		return false, err
	}
	return raw != "", nil
}

func (s *Store) clearSecretCookie(key, name string) error {
	if err := ensureSecretStorage(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, key); err != nil {
		return fmt.Errorf("clear %s Cookie: %w", name, err)
	}
	return nil
}

func normalizeCookie(cookie, name string, maxLength int) (string, error) {
	cookie = strings.TrimSpace(cookie)
	if len(cookie) >= len("Cookie:") && strings.EqualFold(cookie[:len("Cookie:")], "Cookie:") {
		cookie = strings.TrimSpace(cookie[len("Cookie:"):])
	}
	if cookie == "" {
		return "", fmt.Errorf("%s Cookie cannot be empty", name)
	}
	if strings.ContainsAny(cookie, "\r\n") {
		return "", fmt.Errorf("%s Cookie must not contain CR or LF", name)
	}
	if strings.IndexByte(cookie, 0) >= 0 {
		return "", fmt.Errorf("%s Cookie contains an invalid NUL character", name)
	}
	if len(cookie) > maxLength {
		return "", fmt.Errorf("%s Cookie is too long (maximum %d bytes)", name, maxLength)
	}
	return cookie, nil
}
