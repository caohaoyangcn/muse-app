package netproxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseAcceptsUserAddresses(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{name: "bare host and port", in: "127.0.0.1:7890", want: "http://127.0.0.1:7890"},
		{name: "http with trailing slash", in: " http://127.0.0.1:7890/ ", want: "http://127.0.0.1:7890"},
		{name: "https", in: "https://proxy.example.com:8443", want: "https://proxy.example.com:8443"},
		{name: "socks5 with credentials", in: "socks5://user:pass@10.0.0.2:1080", want: "socks5://user:pass@10.0.0.2:1080"},
		{name: "ipv6", in: "[::1]:7890", want: "http://[::1]:7890"},
		{name: "uppercase scheme", in: "HTTP://127.0.0.1:7890", want: "http://127.0.0.1:7890"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, err := Parse(tc.in)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.in, err)
			}
			if u == nil || u.String() != tc.want {
				t.Fatalf("Parse(%q) = %v, want %s", tc.in, u, tc.want)
			}
		})
	}
}

func TestParseEmptyClears(t *testing.T) {
	for _, in := range []string{"", "   ", "\t"} {
		u, err := Parse(in)
		if err != nil || u != nil {
			t.Fatalf("Parse(%q) = (%v, %v), want (nil, nil)", in, u, err)
		}
	}
}

func TestParseRejectsBadAddresses(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{name: "unsupported scheme", in: "ftp://127.0.0.1:21", want: "协议"},
		{name: "missing host", in: "http://:7890", want: "主机名"},
		{name: "missing port", in: "http://127.0.0.1", want: "端口"},
		{name: "port zero", in: "http://127.0.0.1:0", want: "端口"},
		{name: "port too large", in: "http://127.0.0.1:70000", want: "端口"},
		{name: "path", in: "http://127.0.0.1:7890/proxy", want: "路径"},
		{name: "query", in: "http://127.0.0.1:7890?x=1", want: "路径"},
		{name: "fragment", in: "http://127.0.0.1:7890#frag", want: "路径"},
		{name: "newline", in: "http://127.0.0.1:7890\r\nX: y", want: "换行"},
		{name: "too long", in: "http://127.0.0.1:7890/" + strings.Repeat("a", maxURLLength), want: "过长"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.in)
			if err == nil {
				t.Fatalf("Parse(%q) should fail", tc.in)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse(%q) error = %q, want it to mention %q", tc.in, err, tc.want)
			}
		})
	}
}

func TestConfigSwitchesProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("NO_PROXY", "")
	cfg := New()
	req, err := http.NewRequest(http.MethodGet, "https://javbus.com/ABP-576", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	if got, err := cfg.ProxyFunc(req); err != nil || got != nil {
		t.Fatalf("unset proxy = (%v, %v), want (nil, nil)", got, err)
	}

	u, err := Parse("127.0.0.1:7890")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg.Set(u)
	got, err := cfg.ProxyFunc(req)
	if err != nil || got == nil || got.String() != "http://127.0.0.1:7890" {
		t.Fatalf("configured proxy = (%v, %v), want http://127.0.0.1:7890", got, err)
	}

	// The config owns its URL: mutating the caller's value must not leak in.
	u.Host = "evil.example:1"
	got, _ = cfg.ProxyFunc(req)
	if got.String() != "http://127.0.0.1:7890" {
		t.Fatalf("config kept external mutation: %v", got)
	}

	cfg.Set(nil)
	if got, err := cfg.ProxyFunc(req); err != nil || got != nil {
		t.Fatalf("cleared proxy = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestTransportUsesConfig(t *testing.T) {
	cfg := New()
	transport := cfg.Transport()
	if transport == http.DefaultTransport {
		t.Fatal("Transport must return a clone, not http.DefaultTransport")
	}

	u, err := Parse("http://127.0.0.1:7890")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg.Set(u)
	req, err := http.NewRequest(http.MethodGet, "https://fc2cmadb.com/articles/1", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	got, err := transport.Proxy(req)
	if err != nil || got == nil || got.Host != "127.0.0.1:7890" {
		t.Fatalf("transport proxy = (%v, %v), want 127.0.0.1:7890", got, err)
	}
}

func TestTransportRoutesThroughConfiguredProxy(t *testing.T) {
	requests := make(chan string, 1)
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Host
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxyServer.Close()

	proxyURL, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatalf("parse test proxy: %v", err)
	}
	cfg := New()
	cfg.Set(proxyURL)

	// A plain-HTTP target keeps the test free of CONNECT/TLS handling: the
	// transport must hand the absolute request to the configured proxy.
	client := &http.Client{Transport: cfg.Transport()}
	res, err := client.Get("http://scrape.invalid/ABP-576")
	if err != nil {
		t.Fatalf("request through proxy: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusNoContent)
	}
	select {
	case host := <-requests:
		if host != "scrape.invalid" {
			t.Fatalf("proxy saw host %q, want scrape.invalid", host)
		}
	default:
		t.Fatal("request did not reach the configured proxy")
	}
}
