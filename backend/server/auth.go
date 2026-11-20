package server

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"muse-app/backend/accesskey"
)

const (
	// sessionCookieName carries the access key once a phone has logged in. The
	// cookie is HttpOnly: the SPA never needs to read it (it asks /api/session
	// for the key when it builds an external-player URL).
	sessionCookieName = "muse_lan_key"
	sessionCookieAge  = 365 * 24 * 60 * 60

	// The two endpoints reachable without the key. /api/session is the log-in
	// handshake itself; the extension endpoint is a read-only cross-origin
	// route with its own host and origin checks (see extension.go).
	sessionPath   = "/api/session"
	extensionPath = "/api/extension/check"

	// loginMaxFailures/loginWindow throttle password guessing. The key has 80
	// bits of entropy, so this only stops a noisy client from hammering the
	// log; it is not what makes guessing infeasible.
	loginMaxFailures = 5
	loginWindow      = 5 * time.Minute
	maxLoginTracked  = 4096
)

// SetAccessKey arms the LAN API with key. An empty key leaves the server open:
// that state exists for tests and for embedding, while the desktop app always
// configures a key before Start.
func (s *Server) SetAccessKey(key string) {
	normalized, _ := accesskey.Normalize(key)
	s.keyMu.Lock()
	s.accessKey = normalized
	s.keyMu.Unlock()
}

// AccessKey returns the key in effect, or "" when the server is unguarded.
func (s *Server) AccessKey() string {
	s.keyMu.RLock()
	defer s.keyMu.RUnlock()
	return s.accessKey
}

// requestAuthorized reports whether r proves it knows key. Three transports are
// accepted: the phone's HttpOnly cookie, a Bearer token (curl/scripts), and —
// only for /stream/ — a `key` query parameter. Native players (Infuse, VLC)
// cannot send either of the first two, and the URL handed to them is the only
// place the key can travel.
func (s *Server) requestAuthorized(r *http.Request, key string) bool {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && keyMatches(cookie.Value, key) {
		return true
	}
	if raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		if keyMatches(strings.TrimSpace(raw), key) {
			return true
		}
	}
	if strings.HasPrefix(r.URL.Path, "/stream/") && keyMatches(r.URL.Query().Get("key"), key) {
		return true
	}
	return false
}

// keyMatches compares a user-supplied value with the stored key in constant
// time. Both sides are normalized first so the grouped display form works.
func keyMatches(candidate, key string) bool {
	normalized, ok := accesskey.Normalize(candidate)
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(normalized), []byte(key)) == 1
}

// requiresAccessKey is the default-deny rule for the whole mux. Only the SPA
// shell/assets and the two handshake endpoints are public; every data route —
// reads, writes, media, generated images — is behind the key.
func requiresAccessKey(path string) bool {
	if path == sessionPath || path == extensionPath {
		return false
	}
	for _, prefix := range []string{"/api/", "/stream/", "/thumbnails/", "/covers/", "/avatars/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// withLANAuth enforces the access key on protected routes. A request that
// arrives without one gets a 401 before any handler touches the library.
func (s *Server) withLANAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresAccessKey(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		key := s.AccessKey()
		if key == "" {
			// Unconfigured server: tests and embedding keep their old reach.
			next.ServeHTTP(w, r)
			return
		}
		if !s.requestAuthorized(r, key) {
			httpError(w, http.StatusUnauthorized, "需要访问口令")
			return
		}
		// A state-changing request from another origin is refused even if it
		// somehow carries the cookie. SameSite=Lax already withholds the cookie
		// from cross-site POSTs; this is the second lock for older clients.
		if crossSiteWrite(r) {
			httpError(w, http.StatusForbidden, "拒绝跨站请求")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// crossSiteWrite reports whether r is a non-GET request a browser sent from a
// different origin. Native clients send no Origin and are not affected.
func crossSiteWrite(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return true
	}
	return !strings.EqualFold(parsed.Host, r.Host)
}

// withSecurityHeaders applies headers that cost nothing and shrink the ways a
// LAN page can leak or be framed. The SPA's responses are included; API
// responses are marked uncacheable so a shared browser cache cannot replay one
// phone's library to another.
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// sessionResponse is the body of every /api/session call. Key is only present
// once the caller is authenticated; it is what lets the SPA build a stream URL
// for a native player without reading the HttpOnly cookie.
type sessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	Key           string `json:"key,omitempty"`
}

// handleSessionStatus answers whether this browser already holds the key.
func (s *Server) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	key := s.AccessKey()
	if key != "" && !s.requestAuthorized(r, key) {
		httpError(w, http.StatusUnauthorized, "需要访问口令")
		return
	}
	writeJSON(w, sessionResponse{Authenticated: true, Key: key})
}

// handleSessionLogin verifies a typed or pasted key and, on success, installs
// the session cookie. Failures are throttled per client address; success clears
// the counter so a typo followed by the right key does not linger.
func (s *Server) handleSessionLogin(w http.ResponseWriter, r *http.Request) {
	ip := remoteIP(r)
	limiter := s.loginLimiter()
	if limiter.blocked(ip) {
		w.Header().Set("Retry-After", "60")
		httpError(w, http.StatusTooManyRequests, "口令尝试次数过多，请稍后再试")
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	key := s.AccessKey()
	if key == "" {
		// Unconfigured server: nothing to prove. The desktop app never runs in
		// this state; it only exists for tests and embedding.
		writeJSON(w, sessionResponse{Authenticated: true})
		return
	}
	if !keyMatches(body.Key, key) {
		limiter.recordFailure(ip)
		httpError(w, http.StatusUnauthorized, "访问口令错误")
		return
	}
	limiter.reset(ip)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    key,
		Path:     "/",
		MaxAge:   sessionCookieAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, sessionResponse{Authenticated: true, Key: key})
}

// handleSessionLogout forgets the cookie. Rotation is the stronger action; this
// one only ends the current browser's session.
func (s *Server) handleSessionLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// loginLimiter counts failed key attempts per client address in memory. It is
// deliberately not persisted: a restart is a clean slate, and the key itself is
// the real defense.
type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginEntry
}

type loginEntry struct {
	failures int
	since    time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: make(map[string]*loginEntry)}
}

// loginLimiter lazily creates the failure counter. The zero value of Server is
// usable (tests build one directly), so the handler cannot assume New ran.
func (s *Server) loginLimiter() *loginLimiter {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.logins == nil {
		s.logins = newLoginLimiter()
	}
	return s.logins
}

// blocked reports whether ip may still attempt a login.
func (l *loginLimiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[ip]
	if entry == nil {
		return false
	}
	if time.Since(entry.since) >= loginWindow {
		delete(l.entries, ip)
		return false
	}
	return entry.failures >= loginMaxFailures
}

// recordFailure counts one wrong key inside the current window.
func (l *loginLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) >= maxLoginTracked {
		// Bound memory: a hostile LAN client cycling addresses must not grow
		// this map without limit. Resetting only costs a fresh window.
		l.entries = make(map[string]*loginEntry)
	}
	entry := l.entries[ip]
	if entry == nil || time.Since(entry.since) >= loginWindow {
		l.entries[ip] = &loginEntry{failures: 1, since: time.Now()}
		return
	}
	entry.failures++
}

// reset forgets ip's failures after a successful login.
func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	delete(l.entries, ip)
	l.mu.Unlock()
}

// remoteIP extracts the client address without a reverse proxy in front (the
// LAN server is dialed directly, so X-Forwarded-For would only be spoofable).
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
