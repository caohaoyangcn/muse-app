// Package netproxy resolves the outbound HTTP proxy used by metadata scraping.
//
// Go's net/http deliberately ignores the Windows system proxy (WinINET), so a
// desktop build that only sets the system proxy still dials the public sites
// directly. Muse therefore stores its own proxy address in settings and applies
// it to the provider transports here. When no address is configured the
// transports fall back to http.ProxyFromEnvironment, which keeps the existing
// HTTP_PROXY/HTTPS_PROXY/NO_PROXY behavior.
package netproxy

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// maxURLLength bounds a pasted proxy address. Real addresses are far shorter;
// the limit keeps a broken paste from reaching the settings table.
const maxURLLength = 2048

// Config holds the proxy address currently in effect and owns every transport
// created from this config, so a change can drop pooled connections that were
// opened through the previous route. It is safe for concurrent use.
type Config struct {
	mu         sync.RWMutex
	url        *url.URL
	transports []*http.Transport
}

// New creates an empty config (direct connections, environment proxy honored).
func New() *Config { return &Config{} }

// Parse validates a user-entered proxy address and returns its canonical form.
// An empty string clears the setting and returns (nil, nil). A bare host:port
// is accepted and treated as an HTTP proxy.
func Parse(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if len(raw) > maxURLLength {
		return nil, fmt.Errorf("代理地址过长（最多 %d 个字符）", maxURLLength)
	}
	if strings.ContainsAny(raw, "\r\n") {
		return nil, errors.New("代理地址不能包含换行")
	}
	candidate := raw
	// url.Parse reads "127.0.0.1:7890" as scheme "127.0.0.1", so a scheme-less
	// value is always completed with http:// instead of being parsed as-is.
	if !strings.Contains(candidate, "://") {
		candidate = "http://" + candidate
	}
	u, err := url.Parse(candidate)
	if err != nil {
		return nil, fmt.Errorf("代理地址格式无效：%w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "socks5":
	default:
		return nil, errors.New("代理协议仅支持 http、https 或 socks5")
	}
	if u.Hostname() == "" {
		return nil, errors.New("代理地址缺少主机名")
	}
	if u.Port() == "" {
		return nil, errors.New("代理地址需要包含端口，例如 127.0.0.1:7890")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("代理端口必须在 1-65535 之间")
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("代理地址只接受 协议://主机:端口，不能包含路径、查询或片段")
	}
	u.Path, u.RawQuery, u.Fragment = "", "", ""
	return u, nil
}

// Set replaces the proxy address. A nil URL restores direct connections (with
// the environment proxy still honored). Idle connections opened through the
// previous route are closed so no request reuses them by accident.
func (c *Config) Set(u *url.URL) {
	c.mu.Lock()
	if u == nil {
		c.url = nil
	} else {
		clone := *u
		c.url = &clone
	}
	transports := append([]*http.Transport(nil), c.transports...)
	c.mu.Unlock()

	for _, t := range transports {
		t.CloseIdleConnections()
	}
}

// ProxyFunc is the http.Transport.Proxy hook: the configured address wins, and
// an unset address delegates to the environment (HTTP_PROXY/HTTPS_PROXY).
func (c *Config) ProxyFunc(req *http.Request) (*url.URL, error) {
	c.mu.RLock()
	u := c.url
	c.mu.RUnlock()
	if u != nil {
		return u, nil
	}
	return http.ProxyFromEnvironment(req)
}

// Transport returns a clone of http.DefaultTransport whose Proxy hook reads
// this Config, so later Set calls take effect without rebuilding providers.
func (c *Config) Transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = c.ProxyFunc
	c.mu.Lock()
	c.transports = append(c.transports, t)
	c.mu.Unlock()
	return t
}
