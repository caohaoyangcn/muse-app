// Package settings persists small key/value user preferences in the app
// database (the "settings" table, added in schema v5): the LAN server port,
// encrypted provider cookies, the scraping proxy address, the LLM
// title-translation provider, and one-off first-run flags. The store is generic
// so new preferences can reuse it without another migration.
package settings

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"muse-app/backend/llm"
	"muse-app/backend/netproxy"
	"muse-app/backend/secret"
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
	keyHiddenTags      = "hidden_quick_tags"
	keyTrashRetention  = "trash_retention_days"
	keyLLMBaseURL      = "llm_base_url"
	keyLLMModel        = "llm_model"
	keyLLMEnabled      = "llm_enabled"
	keyLLMResumeTitles = "llm_resume_missing_titles"
	keyLLMAPIKey       = "llm_api_key_dpapi"
	// Cookie headers are normally much smaller than this. The limit prevents
	// accidentally persisting an unbounded paste while allowing complete
	// browser Cookie headers with auxiliary session values.
	maxJavBusCookieLength = 16 * 1024
	maxFC2CookieLength    = maxJavBusCookieLength
	// An API key is a short bearer token; the limit only exists to stop a
	// broken paste from reaching the encrypted settings blob.
	maxLLMAPIKeyLength = 16 * 1024
	// A hidden-tag list is a UI preference; these bounds keep a broken client
	// from writing an unbounded JSON blob into the settings table.
	maxHiddenTags      = 5000
	maxHiddenTagLength = 256
)

// ErrSecretStorageUnsupported indicates that this build cannot persist
// secrets securely. There is intentionally no plaintext fallback.
//
// Deprecated: use secret.ErrUnsupported. Kept as an alias so existing settings
// callers and tests keep one error identity.
var ErrSecretStorageUnsupported = secret.ErrUnsupported

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

// LLMConfig returns the title-translation provider configuration. The endpoint
// and model fall back to the llm package defaults; an absent enabled flag
// counts as enabled, so entering an API key is enough to turn translation on.
// A platform without secure secret storage reports "no key" instead of an
// error, which keeps the rest of the settings form usable.
func (s *Store) LLMConfig() (llm.Config, error) {
	baseURL, err := s.get(keyLLMBaseURL)
	if err != nil {
		return llm.Config{}, err
	}
	model, err := s.get(keyLLMModel)
	if err != nil {
		return llm.Config{}, err
	}
	enabledRaw, err := s.get(keyLLMEnabled)
	if err != nil {
		return llm.Config{}, err
	}
	apiKey, err := s.getLLMAPIKey()
	if err != nil {
		return llm.Config{}, err
	}
	return llm.Normalize(llm.Config{
		Enabled: enabledRaw != "0",
		BaseURL: baseURL,
		Model:   model,
		APIKey:  apiKey,
	})
}

// SetLLMConfig validates and persists the provider configuration. An empty
// API key leaves the previously saved key untouched, so the settings form can
// update the endpoint or model without the user re-entering the secret.
func (s *Store) SetLLMConfig(cfg llm.Config) error {
	normalized, err := llm.Normalize(cfg)
	if err != nil {
		return err
	}
	// Store the secret first: if secure storage is unavailable, no part of the
	// form is saved and the user gets one clear error.
	if normalized.APIKey != "" {
		if err := s.SetLLMAPIKey(normalized.APIKey); err != nil {
			return err
		}
	}
	if err := s.set(keyLLMBaseURL, normalized.BaseURL); err != nil {
		return err
	}
	if err := s.set(keyLLMModel, normalized.Model); err != nil {
		return err
	}
	enabled := "1"
	if !normalized.Enabled {
		enabled = "0"
	}
	return s.set(keyLLMEnabled, enabled)
}

// LLMResumeMissingTitles reports whether opening the app should re-queue every
// scraped favorite that still has no Chinese title. It defaults to false: each
// translation costs a provider request, so resuming a batch interrupted by a
// quit (the queue is in memory only) is an explicit opt-in instead of a side
// effect of every launch.
func (s *Store) LLMResumeMissingTitles() (bool, error) {
	raw, err := s.get(keyLLMResumeTitles)
	if err != nil {
		return false, err
	}
	return raw == "1", nil
}

// SetLLMResumeMissingTitles persists the startup backfill switch.
func (s *Store) SetLLMResumeMissingTitles(enabled bool) error {
	value := "0"
	if enabled {
		value = "1"
	}
	return s.set(keyLLMResumeTitles, value)
}

// SetLLMAPIKey securely persists the bearer token used by the translation
// provider with the platform secret store (DPAPI on Windows, the login
// Keychain on macOS). There is intentionally no plaintext fallback.
func (s *Store) SetLLMAPIKey(apiKey string) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return errors.New("API Key 不能为空")
	}
	if strings.ContainsAny(apiKey, "\r\n") {
		return errors.New("API Key 不能包含换行")
	}
	if len(apiKey) > maxLLMAPIKeyLength {
		return fmt.Errorf("API Key 过长（最多 %d 个字符）", maxLLMAPIKeyLength)
	}
	if err := secret.Available(); err != nil {
		return fmt.Errorf("当前平台不支持安全保存 API Key：%w", err)
	}
	previous, err := s.get(keyLLMAPIKey)
	if err != nil {
		return err
	}
	protected, err := secret.Protect(apiKey)
	if err != nil {
		return fmt.Errorf("protect LLM API key: %w", err)
	}
	if err := s.set(keyLLMAPIKey, protected); err != nil {
		return err
	}
	if previous != "" {
		_ = secret.Delete(previous)
	}
	return nil
}

// HasLLMAPIKey reports whether an API key is stored. A platform without
// secure secret storage simply has none.
func (s *Store) HasLLMAPIKey() (bool, error) {
	if err := secret.Available(); err != nil {
		if errors.Is(err, secret.ErrUnsupported) {
			return false, nil
		}
		return false, err
	}
	raw, err := s.get(keyLLMAPIKey)
	if err != nil {
		return false, err
	}
	return raw != "", nil
}

// ClearLLMAPIKey removes the stored bearer token. Translation then reports
// itself as unconfigured until a new key is saved.
func (s *Store) ClearLLMAPIKey() error {
	if err := secret.Available(); err != nil {
		return fmt.Errorf("当前平台不支持安全保存 API Key：%w", err)
	}
	previous, err := s.get(keyLLMAPIKey)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, keyLLMAPIKey); err != nil {
		return fmt.Errorf("clear LLM API key: %w", err)
	}
	if previous != "" {
		_ = secret.Delete(previous)
	}
	return nil
}

func (s *Store) getLLMAPIKey() (string, error) {
	if err := secret.Available(); err != nil {
		if errors.Is(err, secret.ErrUnsupported) {
			return "", nil
		}
		return "", err
	}
	raw, err := s.get(keyLLMAPIKey)
	if err != nil {
		return "", err
	}
	if raw == "" {
		return "", nil
	}
	return secret.Unprotect(raw)
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

// TrashRetentionDays returns the configured recycle-bin retention in days.
// configured is false when nothing usable is stored (never set, corrupt, or out
// of range), which lets the caller apply its own default without this generic
// store knowing the product policy.
func (s *Store) TrashRetentionDays() (days int, configured bool, err error) {
	raw, err := s.get(keyTrashRetention)
	if err != nil {
		return 0, false, err
	}
	if raw == "" {
		return 0, false, nil
	}
	days, err = strconv.Atoi(raw)
	if err != nil || days < 1 {
		// A corrupt value must not break startup; the effective default applies
		// until the user saves a new one.
		return 0, false, nil
	}
	return days, true, nil
}

// SetTrashRetentionDays persists the retention. Range validation belongs to the
// product façade that owns the policy (api.Library), so this store only checks
// that the value can be stored as a positive integer.
func (s *Store) SetTrashRetentionDays(days int) error {
	if days < 1 {
		return fmt.Errorf("retention days must be positive")
	}
	return s.set(keyTrashRetention, strconv.Itoa(days))
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

// HiddenTags returns the scraped tags the user removed from the sidebar's
// quick list. Like the tour flag this lives in the database instead of the
// webview's localStorage: a rebuilt executable or a disposable webview profile
// must not silently bring every hidden tag back.
//
// A corrupt value reads as an empty list (the shortcut simply shows everything
// again) rather than breaking the library.
func (s *Store) HiddenTags() ([]string, error) {
	raw, err := s.get(keyHiddenTags)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return []string{}, nil
	}
	return normalizeHiddenTags(tags), nil
}

// SetHiddenTags persists the shortcut's hidden list. Values are trimmed,
// deduplicated and stored as a JSON array; an empty selection stores [] rather
// than deleting the row so a later read is deterministic.
func (s *Store) SetHiddenTags(tags []string) error {
	normalized := normalizeHiddenTags(tags)
	if len(normalized) > maxHiddenTags {
		return fmt.Errorf("隐藏标签过多（最多 %d 个）", maxHiddenTags)
	}
	for _, tag := range normalized {
		if len([]rune(tag)) > maxHiddenTagLength {
			return fmt.Errorf("标签名过长（最多 %d 个字符）", maxHiddenTagLength)
		}
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("encode hidden tags: %w", err)
	}
	return s.set(keyHiddenTags, string(encoded))
}

// normalizeHiddenTags trims, drops blanks and deduplicates while preserving the
// order the user hid the tags in. It never returns nil, so the stored JSON is
// always an array.
func normalizeHiddenTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		if _, dup := seen[tag]; dup {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

// GetJavBusCookie returns the decrypted JavBus Cookie, or an empty string when
// no Cookie has been configured. The database contains only a platform secret
// reference (a DPAPI blob on Windows, a Keychain item reference on macOS).
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
	if err := secret.Available(); err != nil {
		return "", err
	}
	raw, err := s.get(key)
	if err != nil {
		return "", err
	}
	if raw == "" {
		return "", nil
	}
	cookie, err := secret.Unprotect(raw)
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
	if err := secret.Available(); err != nil {
		return err
	}
	previous, err := s.get(key)
	if err != nil {
		return err
	}
	protected, err := secret.Protect(cookie)
	if err != nil {
		return fmt.Errorf("protect %s Cookie: %w", name, err)
	}
	if err := s.set(key, protected); err != nil {
		return err
	}
	// The new value is authoritative. A failed cleanup of the previous
	// Keychain item is an orphan, not a reason to report the save as failed.
	if previous != "" {
		_ = secret.Delete(previous)
	}
	return nil
}

func (s *Store) hasSecretCookie(key string) (bool, error) {
	if err := secret.Available(); err != nil {
		return false, err
	}
	raw, err := s.get(key)
	if err != nil {
		return false, err
	}
	return raw != "", nil
}

func (s *Store) clearSecretCookie(key, name string) error {
	if err := secret.Available(); err != nil {
		return err
	}
	previous, err := s.get(key)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, key); err != nil {
		return fmt.Errorf("clear %s Cookie: %w", name, err)
	}
	if previous != "" {
		_ = secret.Delete(previous)
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
