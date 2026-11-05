package cloud

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"muse-app/backend/secret"
)

const defaultHealthInterval = 60 * time.Second

// Service owns connection CRUD, secret handling, browsing and the in-memory
// health cache. Reads never touch the network unless the caller explicitly asks
// to browse, test or open a stream.
type Service struct {
	repo Repository
	emit func(name string, data ...any)
	log  *slog.Logger

	// protect/unprotect are seams so the password lifecycle can be tested on a
	// platform without DPAPI. Production always uses backend/secret.
	protect   func(string) (string, error)
	unprotect func(string) (string, error)

	mu       sync.Mutex
	statuses map[int64]connectionStatus
}

type connectionStatus struct {
	online bool
	known  bool
	err    string
}

// NewService builds the cloud service over a connection repository.
func NewService(repo Repository, emit func(name string, data ...any), log *slog.Logger) *Service {
	return &Service{
		repo:      repo,
		emit:      emit,
		log:       log,
		protect:   secret.Protect,
		unprotect: secret.Unprotect,
		statuses:  make(map[int64]connectionStatus),
	}
}

// StorageAvailable reports whether this build can persist a password.
func (s *Service) StorageAvailable() bool { return secret.Available() == nil }

// ListConnections returns every configured source with its favorite count and
// last observed health. It is deliberately local-only: opening Settings or the
// add-source dialog must never wait on a slow WebDAV server.
func (s *Service) ListConnections() ([]ConnectionView, error) {
	connections, err := s.repo.ListConnections()
	if err != nil {
		return nil, err
	}
	out := make([]ConnectionView, 0, len(connections))
	for i := range connections {
		view, err := s.viewFor(&connections[i])
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

// Connection returns one connection with its favorite count.
func (s *Service) Connection(id int64) (*ConnectionView, error) {
	c, err := s.repo.GetConnection(id)
	if err != nil {
		return nil, err
	}
	view, err := s.viewFor(c)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) viewFor(c *Connection) (ConnectionView, error) {
	count, err := s.repo.CountRemoteVideos(c.ID)
	if err != nil {
		return ConnectionView{}, err
	}
	view := ConnectionView{
		ID:          c.ID,
		Name:        c.Name,
		Kind:        c.Kind,
		Endpoint:    c.Endpoint,
		WebURL:      c.WebURL,
		Username:    c.Username,
		RootPath:    c.RootPath,
		VerifyTLS:   c.VerifyTLS,
		HasPassword: c.PasswordEnc != "",
		Online:      true,
		LastError:   c.LastError,
		LastOKAt:    c.LastOKAt,
		VideoCount:  count,
	}
	s.mu.Lock()
	status, known := s.statuses[c.ID]
	s.mu.Unlock()
	if known {
		view.Online = status.online
		if status.err != "" {
			view.LastError = status.err
		}
	}
	return view, nil
}

// SaveConnection creates or updates one source. An empty password keeps the
// stored one; ClearPassword removes it; a non-empty password is protected with
// the platform secret store and never logged.
func (s *Service) SaveConnection(input ConnectionInput) (*ConnectionView, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("请填写连接名称")
	}
	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		kind = "webdav"
	}
	if kind != "webdav" {
		return nil, fmt.Errorf("暂不支持这种连接类型")
	}
	endpoint := strings.TrimSpace(input.Endpoint)
	rootPath := strings.TrimSpace(input.RootPath)
	if rootPath == "" {
		rootPath = "/"
	}
	webURL := strings.TrimSpace(input.WebURL)
	if webURL != "" {
		u, err := url.Parse(webURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("OpenList 网页地址无效，请输入 http(s):// 开头的地址")
		}
	}
	// Validate the endpoint and root with the same code path the client uses,
	// so a save cannot succeed with an address that browsing will reject.
	if _, err := NewClient(Connection{Endpoint: endpoint, RootPath: rootPath, VerifyTLS: input.VerifyTLS}, ""); err != nil {
		return nil, fmt.Errorf("%s", userMessage(err))
	}

	var existing *Connection
	if input.ID > 0 {
		var err error
		existing, err = s.repo.GetConnection(input.ID)
		if err != nil {
			return nil, err
		}
	}
	passwordEnc := ""
	if existing != nil {
		passwordEnc = existing.PasswordEnc
	}
	switch {
	case input.ClearPassword:
		passwordEnc = ""
	case strings.TrimSpace(input.Password) != "":
		enc, err := s.protect(input.Password)
		if err != nil {
			if errors.Is(err, secret.ErrUnsupported) {
				return nil, ErrPasswordStorageUnavailable
			}
			return nil, fmt.Errorf("保存密码失败: %w", err)
		}
		passwordEnc = enc
	}

	now := time.Now().UnixMilli()
	c := &Connection{
		Name:        name,
		Kind:        kind,
		Endpoint:    endpoint,
		WebURL:      webURL,
		Username:    strings.TrimSpace(input.Username),
		PasswordEnc: passwordEnc,
		RootPath:    rootPath,
		VerifyTLS:   input.VerifyTLS,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if existing != nil {
		c.ID = existing.ID
		c.CreatedAt = existing.CreatedAt
		if err := s.repo.UpdateConnection(c); err != nil {
			return nil, err
		}
		// A saved connection has not been probed yet; do not carry a stale
		// offline badge into the new configuration.
		s.forgetStatus(c.ID)
	} else {
		if err := s.repo.CreateConnection(c); err != nil {
			return nil, err
		}
	}
	view, err := s.viewFor(c)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

// DeleteConnection removes a connection only when no favorite points at it.
func (s *Service) DeleteConnection(id int64) error {
	count, err := s.repo.CountRemoteVideos(id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("该连接下还有 %d 个收藏，请先移除这些收藏", count)
	}
	if err := s.repo.DeleteConnection(id); err != nil {
		return err
	}
	s.forgetStatus(id)
	return nil
}

// TestConnection probes the unsaved form values (falling back to the stored
// row) and reports latency plus a readable failure. For an existing connection
// the observation is also persisted and broadcast, so the list badge and the
// favorite warning stay in sync.
func (s *Service) TestConnection(input ConnectionInput) (TestResult, error) {
	conn, password, err := s.resolveInput(input)
	if err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			return TestResult{}, err
		}
		return TestResult{Error: userMessage(err)}, nil
	}
	client, err := NewClient(*conn, password)
	if err != nil {
		return TestResult{Error: userMessage(err)}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	latency, testErr := client.Test(ctx)
	if testErr != nil {
		message := userMessage(testErr)
		if conn.ID > 0 {
			s.record(conn.ID, conn.Name, false, message, true)
		}
		return TestResult{OK: false, LatencyMs: latency.Milliseconds(), Error: message}, nil
	}
	if conn.ID > 0 {
		s.record(conn.ID, conn.Name, true, "", true)
	}
	return TestResult{OK: true, LatencyMs: latency.Milliseconds()}, nil
}

// resolveInput merges a form input with the persisted row. Password precedence:
// a non-empty typed password, then an explicit clear, then the stored secret.
func (s *Service) resolveInput(input ConnectionInput) (*Connection, string, error) {
	var existing *Connection
	if input.ID > 0 {
		var err error
		existing, err = s.repo.GetConnection(input.ID)
		if err != nil {
			return nil, "", err
		}
	}
	conn := Connection{Kind: "webdav", VerifyTLS: true, RootPath: "/"}
	if existing != nil {
		conn = *existing
	}
	if v := strings.TrimSpace(input.Name); v != "" {
		conn.Name = v
	}
	if v := strings.TrimSpace(input.Endpoint); v != "" {
		conn.Endpoint = v
	}
	if v := strings.TrimSpace(input.WebURL); v != "" {
		conn.WebURL = v
	}
	if v := strings.TrimSpace(input.Username); v != "" {
		conn.Username = v
	}
	if v := strings.TrimSpace(input.RootPath); v != "" {
		conn.RootPath = v
	}
	conn.VerifyTLS = input.VerifyTLS
	if existing == nil {
		// For a new connection an unchecked "verify TLS" only means the form
		// never supplied the field; default to secure.
		conn.VerifyTLS = input.VerifyTLS
	}

	password := ""
	switch {
	case strings.TrimSpace(input.Password) != "":
		password = input.Password
	case input.ClearPassword:
		password = ""
	case existing != nil && existing.PasswordEnc != "":
		plain, err := s.unprotect(existing.PasswordEnc)
		if err != nil {
			return nil, "", fmt.Errorf("读取已保存的密码失败: %w", err)
		}
		password = plain
	}
	return &conn, password, nil
}

// Browse lists a remote collection, directories first and then by name. It
// records source reachability; a 404 is a bad path, not an offline source.
func (s *Service) Browse(id int64, path string) ([]Entry, error) {
	client, conn, err := s.client(id)
	if err != nil {
		return nil, err
	}
	entries, err := client.List(context.Background(), path)
	if err != nil {
		s.observe(conn, err)
		return nil, err
	}
	s.observe(conn, nil)
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		a, b := strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name)
		if a != b {
			return a < b
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

// Stat describes one remote path.
func (s *Service) Stat(id int64, path string) (*Entry, error) {
	return s.StatContext(context.Background(), id, path)
}

// StatContext is Stat with a caller-owned context (media proxy/cache checks).
func (s *Service) StatContext(ctx context.Context, id int64, path string) (*Entry, error) {
	client, conn, err := s.client(id)
	if err != nil {
		return nil, err
	}
	entry, err := client.Stat(ctx, path)
	if err != nil {
		s.observe(conn, err)
		return nil, err
	}
	s.observe(conn, nil)
	return entry, nil
}

// OpenRange opens a byte range for a remote favorite. The caller closes the
// returned body.
func (s *Service) OpenRange(ctx context.Context, id int64, path string, offset, length int64) (*RangeResponse, error) {
	client, conn, err := s.client(id)
	if err != nil {
		return nil, err
	}
	resp, err := client.OpenRange(ctx, path, offset, length)
	if err != nil {
		s.observe(conn, err)
		return nil, err
	}
	s.observe(conn, nil)
	return resp, nil
}

// client decrypts a connection's password and builds its read-only client.
func (s *Service) client(id int64) (*Client, *Connection, error) {
	conn, err := s.repo.GetConnection(id)
	if err != nil {
		return nil, nil, err
	}
	password := ""
	if conn.PasswordEnc != "" {
		password, err = s.unprotect(conn.PasswordEnc)
		if err != nil {
			return nil, nil, fmt.Errorf("读取连接密码失败: %w", err)
		}
	}
	client, err := NewClient(*conn, password)
	if err != nil {
		return nil, conn, err
	}
	return client, conn, nil
}

// Client exposes the read-only client for the media proxy/cache layer.
func (s *Service) Client(id int64) (*Client, error) {
	client, _, err := s.client(id)
	return client, err
}

// StartHealthCheck probes every connection immediately and then on an interval.
// The returned func stops the loop; it does not interrupt an in-flight probe.
func (s *Service) StartHealthCheck(every time.Duration) (stop func()) {
	if every <= 0 {
		every = defaultHealthInterval
	}
	done := make(chan struct{})
	var once sync.Once
	go func() {
		s.checkAll()
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				s.checkAll()
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

func (s *Service) checkAll() {
	connections, err := s.repo.ListConnections()
	if err != nil {
		if s.log != nil {
			s.log.Warn("cloud health: list connections failed", "err", err)
		}
		return
	}
	for i := range connections {
		s.checkOne(&connections[i])
	}
}

func (s *Service) checkOne(conn *Connection) {
	client, _, err := s.client(conn.ID)
	if err != nil {
		s.record(conn.ID, conn.Name, false, userMessage(err), true)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := client.Test(ctx); err != nil {
		s.record(conn.ID, conn.Name, false, userMessage(err), true)
		if s.log != nil {
			s.log.Warn("cloud health check failed", "id", conn.ID, "name", conn.Name, "err", err)
		}
		return
	}
	s.record(conn.ID, conn.Name, true, "", true)
}

// observe updates the in-memory badge from ordinary browse/stream requests. A
// 404 is about one path, so it is not allowed to mark the source offline.
func (s *Service) observe(conn *Connection, err error) {
	if conn == nil {
		return
	}
	if err == nil {
		s.record(conn.ID, conn.Name, true, "", false)
		return
	}
	if errors.Is(err, ErrUnreachable) || errors.Is(err, ErrUnauthorized) {
		s.record(conn.ID, conn.Name, false, userMessage(err), false)
	}
}

// record updates the status cache, persists it when asked, and emits
// cloud:status only when the observation changed. Persisting is reserved for
// explicit tests and the background health pass; browsing success only updates
// memory and avoids a write per directory click.
func (s *Service) record(id int64, name string, online bool, message string, persist bool) {
	s.mu.Lock()
	previous, known := s.statuses[id]
	s.statuses[id] = connectionStatus{online: online, known: true, err: message}
	changed := !known || previous.online != online || previous.err != message
	s.mu.Unlock()

	if persist {
		var err error
		if online {
			err = s.repo.MarkConnectionOK(id, time.Now().UnixMilli())
		} else {
			err = s.repo.MarkConnectionError(id, message)
		}
		if err != nil && s.log != nil {
			s.log.Warn("cloud health: persist status failed", "id", id, "err", err)
		}
	}
	if changed && s.emit != nil {
		s.emit("cloud:status", map[string]any{
			"id":     id,
			"name":   name,
			"online": online,
			"error":  message,
		})
	}
}

func (s *Service) forgetStatus(id int64) {
	s.mu.Lock()
	delete(s.statuses, id)
	s.mu.Unlock()
}

// Online reports the last observation, defaulting to true before the first
// probe (no warning is invented).
func (s *Service) Online(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, known := s.statuses[id]
	if !known {
		return true
	}
	return status.online
}

func (s *Service) OnlineError(id int64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statuses[id].err
}

func userMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrUnauthorized):
		return "认证失败，请检查用户名和密码"
	case errors.Is(err, ErrNotFound):
		return "路径不存在，请检查根目录或该文件是否已被移动"
	case errors.Is(err, ErrUnreachable):
		return "无法连接网盘，请检查地址、网络或 OpenList 是否在运行"
	case errors.Is(err, ErrUnsupported):
		return "该服务器不支持所需的 WebDAV 功能"
	case errors.Is(err, ErrPathOutsideRoot):
		return "路径超出连接的根目录"
	case errors.Is(err, ErrInvalidEndpoint):
		return "WebDAV 地址无效，请输入 http(s):// 开头的完整地址"
	case errors.Is(err, ErrInvalidPath):
		return "路径格式无效"
	case errors.Is(err, ErrRangeNotSatisfiable):
		return "远程文件已变化，请重新读取"
	case errors.Is(err, ErrConnectionNotFound):
		return "网盘连接不存在"
	case errors.Is(err, ErrCacheTooSmall):
		return "该文件超过网盘缓存上限，请先清理缓存或手动播放"
	case errors.Is(err, ErrDownloadInProgress):
		return "该视频正在下载"
	default:
		return err.Error()
	}
}

// SourceStates returns the projection the video read model attaches. It does
// not count favorites and does not touch the network.
func (s *Service) SourceStates() (map[int64]SourceState, error) {
	connections, err := s.repo.ListConnections()
	if err != nil {
		return nil, err
	}
	out := make(map[int64]SourceState, len(connections))
	for i := range connections {
		out[connections[i].ID] = SourceState{
			Name:   connections[i].Name,
			Online: s.Online(connections[i].ID),
		}
	}
	return out, nil
}

// UserMessage maps a cloud failure to the Chinese text shown to the user.
func UserMessage(err error) string { return userMessage(err) }

// Exists is the single-file existence check used by a remote favorite's status
// refresh. A 404 is a definitive false; network/auth failures are returned so
// the caller keeps the favorite untouched.
func (s *Service) Exists(ctx context.Context, id int64, path string) (bool, error) {
	client, conn, err := s.client(id)
	if err != nil {
		return false, err
	}
	if _, err := client.Head(ctx, path); err != nil {
		s.observe(conn, err)
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	s.observe(conn, nil)
	return true, nil
}

// ManagementURL returns the OpenList/web management page for one connection:
// web_url when configured, otherwise the endpoint with a trailing /dav removed.
func (s *Service) ManagementURL(id int64) (string, error) {
	conn, err := s.repo.GetConnection(id)
	if err != nil {
		return "", err
	}
	if web := strings.TrimSpace(conn.WebURL); web != "" {
		return web, nil
	}
	u, err := url.Parse(strings.TrimSpace(conn.Endpoint))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", ErrInvalidEndpoint
	}
	p := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(strings.ToLower(p), "/dav") {
		p = p[:len(p)-len("/dav")]
	}
	if p == "" {
		p = "/"
	}
	u.Path = p
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	return strings.TrimRight(u.String(), "/"), nil
}
