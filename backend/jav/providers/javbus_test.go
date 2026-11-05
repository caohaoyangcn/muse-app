package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"muse-app/backend/jav"
)

func TestJavBusLookupParsesCurrentDetailStructure(t *testing.T) {
	const fixture = `<!doctype html>
<html><body><div class="movie">
<h3>SMD-133 S Model 133 極上素人貸しちゃいます。 : 保坂えり</h3>
<p>識別碼: SMD-133</p>
<p>發行日期: 2015-06-05</p>
<p>長度: 110分鐘</p>
<p>製作商: <a href="/maker/s-model">S Model</a></p>
<p>系列: <a href="/series/s-model">S Model</a></p>
<div><a href="/uncensored/star/hosaka">保坂えり</a><a href="/star/legacy">Legacy Actor</a></div>
<div><a href="/genre/素人">素人</a><a href="/genre/巨乳">巨乳</a></div>
<a href="/imgs/cover/vqe_b.jpg"><img src="/imgs/cover/vqe_b.jpg" alt="cover"></a>
</div></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/SMD-133" {
			t.Fatalf("request path = %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, fixture)
	}))
	defer srv.Close()

	provider := newTestJavBus(srv.Client(), srv.URL)
	metadata, err := provider.Lookup(context.Background(), "smd-133")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Code != "SMD-133" || metadata.Title != "SMD-133 S Model 133 極上素人貸しちゃいます。 : 保坂えり" {
		t.Fatalf("identity = %#v", metadata)
	}
	if metadata.CoverURL != srv.URL+"/imgs/cover/vqe_b.jpg" {
		t.Fatalf("cover = %q", metadata.CoverURL)
	}
	if metadata.Studio != "S Model" || metadata.Series != "S Model" {
		t.Fatalf("company fields = %#v", metadata)
	}
	if metadata.DurationMin == nil || *metadata.DurationMin != 110 {
		t.Fatalf("duration = %#v", metadata.DurationMin)
	}
	if metadata.ReleaseUnix == nil || time.Unix(*metadata.ReleaseUnix, 0).UTC().Format("2006-01-02") != "2015-06-05" {
		t.Fatalf("release date = %#v", metadata.ReleaseUnix)
	}
	if !contains(metadata.Actors, "保坂えり") || !contains(metadata.Actors, "Legacy Actor") {
		t.Fatalf("actors = %#v", metadata.Actors)
	}
	if !contains(metadata.Tags, "素人") || !contains(metadata.Tags, "巨乳") {
		t.Fatalf("tags = %#v", metadata.Tags)
	}
}

func TestJavBusLookupKeepsOGAndLegacyHeaderCompatibility(t *testing.T) {
	const fixture = `<html><head>
<meta content="IPX-633 Sample" property="og:title">
<meta content="https://img.invalid/x.jpg" name="og:image">
</head><body><span class="header">製作商:</span> Studio X <span class="header">系列:</span> Series Y <span class="header">發行日期:</span> 2024-01-02 <span class="header">長度:</span> 120 分鐘 <a href="/star/a">Alice</a><a href="/genre/b">Drama</a></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fixture)
	}))
	defer srv.Close()

	metadata, err := newTestJavBus(srv.Client(), srv.URL).Lookup(context.Background(), "IPX-633")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Title != "IPX-633 Sample" || metadata.CoverURL != "https://img.invalid/x.jpg" {
		t.Fatalf("OG fields = %#v", metadata)
	}
	if metadata.Studio != "Studio X" || metadata.Series != "Series Y" {
		t.Fatalf("legacy fields = %#v", metadata)
	}
	if metadata.DurationMin == nil || *metadata.DurationMin != 120 || metadata.ReleaseUnix == nil {
		t.Fatalf("legacy date/duration = %#v", metadata)
	}
	if !contains(metadata.Actors, "Alice") || !contains(metadata.Tags, "Drama") {
		t.Fatalf("legacy actors/tags = %#v", metadata)
	}
}

func TestJavBusLookupClassifiesAgeVerificationAndHTTPFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.ToLower(r.URL.Path) {
		case "/age-body":
			_, _ = fmt.Fprint(w, `<html><title>Age Verification</title><body>please verify</body></html>`)
		case "/redirect":
			http.Redirect(w, r, "/driver-verify", http.StatusFound)
		case "/driver-verify":
			_, _ = fmt.Fprint(w, `<html><body>verification form</body></html>`)
		case "/rate-limited":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/server-error":
			w.WriteHeader(http.StatusBadGateway)
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		case "/parse-failure":
			_, _ = fmt.Fprint(w, `<html><body><h1>not a detail page</h1></body></html>`)
		case "/mismatch":
			_, _ = fmt.Fprint(w, `<html><body><h3>Other title</h3><p>識別碼: OTHER-1</p></body></html>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	provider := newTestJavBus(srv.Client(), srv.URL)
	cases := []struct {
		code string
		kind string
	}{
		{"age-body", "blocked"},
		{"redirect", "blocked"},
		{"rate-limited", "retryable"},
		{"server-error", "retryable"},
		{"missing", "not_found"},
		{"parse-failure", "parse"},
		{"mismatch", "parse"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			_, err := provider.Lookup(context.Background(), tc.code)
			assertLookupKind(t, err, tc.kind)
		})
	}

	secret := "PHPSESSID=secret-value; existmag=secret-exist"
	_, err := newTestJavBusWithConfig(srv.Client(), srv.URL, JavBusConfig{Cookie: secret}).Lookup(context.Background(), "age-body")
	assertLookupKind(t, err, "blocked")
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("age error exposed Cookie: %v", err)
	}
}

func TestJavBusLookupReturnsNetworkError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial fixture failed")
	})}
	_, err := newTestJavBus(client, "http://javbus.invalid").Lookup(context.Background(), "SMD-133")
	assertLookupKind(t, err, "network")
	if strings.Contains(err.Error(), "Cookie") {
		t.Fatalf("network error unexpectedly mentions Cookie: %v", err)
	}
}

func TestJavBusFetchUsesConfiguredRequestHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = fmt.Fprint(w, "cover fixture")
	}))
	defer srv.Close()

	provider := newTestJavBusWithConfig(nil, srv.URL, JavBusConfig{
		Cookie:    "PHPSESSID=session; existmag=adult",
		UserAgent: "Muse Test UA",
		Accept:    "image/jpeg",
		Referer:   srv.URL + "/SMD-133",
		Timeout:   time.Second,
	})
	response, err := provider.Fetch(context.Background(), srv.URL+"/imgs/cover/vqe_b.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"Cookie":     "PHPSESSID=session; existmag=adult",
		"User-Agent": "Muse Test UA",
		"Accept":     "image/jpeg",
		"Referer":    srv.URL + "/SMD-133",
	} {
		if got.Get(key) != want {
			t.Fatalf("header %s = %q, want %q", key, got.Get(key), want)
		}
	}
}

func assertLookupKind(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	var lookupErr *jav.LookupError
	if !errors.As(err, &lookupErr) || lookupErr.Kind != want {
		t.Fatalf("error = %#v, want kind %s", err, want)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestJavBusFetchUsesDetailRefererForPicsCover(t *testing.T) {
	var coverReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/IPVR-373":
			_, _ = fmt.Fprint(w, `<html><body><h3>IPVR-373 Sample</h3><p>識別碼: IPVR-373</p><a class="bigImage" href="/pics/cover/cigl_b.jpg"><img src="/pics/cover/cigl_b.jpg"></a></body></html>`)
		case "/pics/cover/cigl_b.jpg":
			coverReferer = r.Header.Get("Referer")
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("cover fixture"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	provider := newTestJavBus(srv.Client(), srv.URL)
	if _, err := provider.Lookup(context.Background(), "IPVR-373"); err != nil {
		t.Fatal(err)
	}
	response, err := provider.Fetch(context.Background(), srv.URL+"/pics/cover/cigl_b.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	if coverReferer != srv.URL+"/IPVR-373" {
		t.Fatalf("cover referer = %q, want %q", coverReferer, srv.URL+"/IPVR-373")
	}
}

func newTestJavBus(client *http.Client, baseURL string) *JavBus {
	return newJavBusWithLimiter(client, baseURL, RequestConfig{}, newIntervalLimiter(0))
}

func newTestJavBusWithConfig(client *http.Client, baseURL string, config RequestConfig) *JavBus {
	return newJavBusWithLimiter(client, baseURL, config, newIntervalLimiter(0))
}
