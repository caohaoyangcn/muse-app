package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"muse-app/backend/jav"
)

// The browser extension is the only cross-origin client the LAN server has:
// a content script on javdb.com calls this one endpoint. Everything else in the
// API stays same-origin (unauthenticated, on purpose — see TODO.md), so CORS is
// scoped to the extension endpoint and to JavDB's own origins rather than being
// added to /api/*.
//
// The origin list is duplicated in browser-extension/manifest.json
// (content_scripts[].matches). Both must change together when JavDB moves to a
// new domain; docs/spec/javdb-browser-extension.md records the rule.

// extensionRequest is the extension's batch question.
type extensionRequest struct {
	// Codes are 番号 exactly as the JavDB page renders them. The server does not
	// extract anything from them.
	Codes []string `json:"codes"`
}

// extensionResponse echoes the batch in request order.
type extensionResponse struct {
	Codes []jav.JavCodeCheck `json:"codes"`
}

// handleExtensionCheck answers "is this work in the library?" for the JavDB
// browser extension. It is read-only: nothing the extension sends can change
// the library, which is what makes serving it cross-origin acceptable.
func (s *Server) handleExtensionCheck(w http.ResponseWriter, r *http.Request) {
	// The CORS check only protects the response the page can read; it does not
	// stop the request itself. A hostile page can put one of our allowed origins
	// in an Origin header (it controls its own fetch), and if the request is
	// answered the whole library leaks — through no-cors response bytes, an
	// error-message oracle in the popover, or DNS rebinding. The Host header is
	// the one a page cannot set, and this server only ever binds a loopback name
	// or a LAN address, so anything else is not meant for us.
	if !extensionHostAllowed(r.Host) {
		httpError(w, http.StatusForbidden, "请通过本机地址访问")
		return
	}
	var body extensionRequest
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	results, err := s.lib.MatchJavCodes(body.Codes)
	if err != nil {
		// Only an oversized batch fails: individual unknown 番号 are answered
		// with an empty match list, so a normal page never lands here.
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, extensionResponse{Codes: results})
}

// extensionHostAllowed reports whether the request was addressed to a host this
// server owns: a loopback name (the desktop app's own URL) or a private LAN
// address (the address the mobile UI is opened with). Everything else — most
// importantly a public name an attacker can point at a visitor's loopback
// address by DNS rebinding — is refused.
func extensionHostAllowed(hostport string) bool {
	host := hostport
	if parsed, _, err := net.SplitHostPort(hostport); err == nil {
		host = parsed
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" {
		return true
	}
	if parsed := net.ParseIP(host); parsed != nil {
		return parsed.IsLoopback() || parsed.IsPrivate()
	}
	return false
}

// withExtensionCORS adds CORS headers to one handler when the caller is a
// JavDB page (or a local page during development). Requests from anywhere else
// go through untouched: no header is written, so a browser blocks them exactly
// as it blocks any other cross-origin call.
//
// Credentials are deliberately not allowed: the endpoint takes no cookie and
// the server has no session, so allowing them would only widen the exposure.
func withExtensionCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The response varies by origin even when the header is absent, so a
		// shared cache cannot hand a JavDB request another origin's answer.
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin == "" || !extensionOriginAllowed(origin) {
			if r.Method == http.MethodOptions && origin != "" {
				// A preflight from a disallowed origin must not be answered as
				// if the route existed for it.
				httpError(w, http.StatusForbidden, "来源不被允许")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extensionOriginAllowed reports whether origin may call the extension
// endpoint. Allowed are JavDB's permanent domain, the numbered mirrors it
// publishes (javdb581.com, java582.com — a prefix plus digits, not a
// subdomain), the app domain and its subdomains, and localhost (a developer's
// fixture page).
func extensionOriginAllowed(origin string) bool {
	host := originHost(origin)
	if host == "" {
		return false
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	if host == "javdb.com" || host == "www.javdb.com" || host == "jav.app" {
		return true
	}
	if strings.HasSuffix(host, ".jav.app") {
		return true
	}
	// javdb<N>.com and app.javdb<N>.com: the mirrors JavDB rotates through.
	if rest, ok := strings.CutSuffix(host, ".com"); ok {
		rest = strings.TrimPrefix(rest, "app.")
		if label, ok := strings.CutPrefix(rest, "javdb"); ok {
			return label != "" && isDigits(label)
		}
	}
	return false
}

// isDigits reports whether s is a non-empty run of ASCII digits.
func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// originHost extracts the lowercase host of an Origin header, or "" when the
// value is not an http(s) origin. Origin is "null" for sandboxed frames and
// file:// documents; those are rejected.
func originHost(origin string) string {
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	host := parsed.Hostname()
	if host == "" {
		return ""
	}
	// A bare IP is only allowed when it is the loopback address a developer's
	// fixture page runs on; any other numeric origin is a LAN device, not JavDB.
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
		return ""
	}
	return strings.ToLower(host)
}
