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

func TestFC2CMADBLookupParsesArticleAndSendsCookie(t *testing.T) {
	const fixture = `<!doctype html><html><head>
<meta property="og:title" content="FC2-PPV-4970405 作品标题">
<meta property="og:image" content="/storage/cover.jpg">
<meta property="article:published_time" content="2025-06-07">
</head><body><main class="hero"><h1>作品标题</h1>
<p>発売日: 2025-06-07</p><p>長さ: 120 分</p>
<a href="/tags/drama">剧情</a><a href="/actors/alice">Alice</a>
<script type="application/ld+json">{"name":"JSON 标题","duration":"95 min","tags":["剧情","剧情"]}</script>
</main></body></html>`

	var gotCookie, gotPath, gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		gotPath = r.URL.Path
		gotReferer = r.Header.Get("Referer")
		if r.URL.Path == "/articles/4970405" {
			_, _ = fmt.Fprint(w, fixture)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	provider := newTestFC2CMADB(srv.Client(), srv.URL, FC2CMADBConfig{Cookie: "ageVerified=true; fc2cmadb-session=secret"})
	metadata, err := provider.Lookup(context.Background(), "fc2-ppv-004970405")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Code != "FC2-4970405" || metadata.Provider != "fc2cmadb" {
		t.Fatalf("identity = %#v", metadata)
	}
	if metadata.Title != "作品标题" {
		t.Fatalf("title = %q", metadata.Title)
	}
	if metadata.CoverURL != srv.URL+"/storage/cover.jpg" {
		t.Fatalf("cover = %q", metadata.CoverURL)
	}
	if metadata.ReleaseUnix == nil || time.Unix(*metadata.ReleaseUnix, 0).UTC().Format("2006-01-02") != "2025-06-07" {
		t.Fatalf("release = %#v", metadata.ReleaseUnix)
	}
	if metadata.DurationMin == nil || *metadata.DurationMin != 95 {
		t.Fatalf("duration = %#v", metadata.DurationMin)
	}
	if !containsFC2(metadata.Tags, "剧情") || !containsFC2(metadata.Actors, "Alice") {
		t.Fatalf("lists = tags=%#v actors=%#v", metadata.Tags, metadata.Actors)
	}
	if gotPath != "/articles/4970405" || gotCookie != "ageVerified=true; fc2cmadb-session=secret" || gotReferer != srv.URL+"/" {
		t.Fatalf("request path=%q cookieKeys=%q referer=%q", gotPath, cookieKeysFC2(gotCookie), gotReferer)
	}
}

func TestFC2CMADBLookupClassifiesGateAndHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/articles/4970405":
			_, _ = io.WriteString(w, `<html><body>18歳以上です 年齢確認</body></html>`)
		case "/articles/4970406":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/articles/4970407":
			w.WriteHeader(http.StatusNotFound)
		case "/articles/4970408":
			w.WriteHeader(http.StatusForbidden)
		case "/articles/4970409":
			_, _ = io.WriteString(w, `<html><body><h1>Cloudflare challenge</h1></body></html>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	provider := newTestFC2CMADB(srv.Client(), srv.URL, FC2CMADBConfig{})
	cases := []struct {
		code string
		kind string
	}{
		{"FC2-4970405", "provider-auth"},
		{"FC2-4970406", "retryable"},
		{"FC2-4970407", "not_found"},
		{"FC2-4970408", "provider-auth"},
		{"FC2-4970409", "provider-auth"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			_, err := provider.Lookup(context.Background(), tc.code)
			var lookupErr *jav.LookupError
			if !errors.As(err, &lookupErr) || lookupErr.Kind != tc.kind {
				t.Fatalf("err=%v kind=%q want=%q", err, lookupErrKindFC2(err), tc.kind)
			}
		})
	}
}

func TestFC2CMADBRejectsInvalidCodeAndOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, strings.Repeat("x", fc2CMADBMaxResponse+1))
	}))
	defer srv.Close()
	provider := newTestFC2CMADB(srv.Client(), srv.URL, FC2CMADBConfig{})
	if _, err := provider.Lookup(context.Background(), "4970405"); err == nil {
		t.Fatal("bare numeric code should be rejected by the provider")
	}
	_, err := provider.Lookup(context.Background(), "FC2-4970405")
	var lookupErr *jav.LookupError
	if !errors.As(err, &lookupErr) || lookupErr.Kind != "parse" {
		t.Fatalf("oversized response error=%v", err)
	}
}

func TestFC2CMADBCoverDoesNotAcceptForeignHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	provider := newTestFC2CMADB(srv.Client(), srv.URL, FC2CMADBConfig{Cookie: "secret"})
	if _, err := provider.Fetch(context.Background(), "https://cdn.example.invalid/cover.jpg"); err == nil {
		t.Fatal("foreign cover host should be rejected")
	}
}

func newTestFC2CMADB(client *http.Client, baseURL string, config FC2CMADBConfig) *FC2CMADB {
	return newFC2CMADBWithLimiter(client, baseURL, config, newIntervalLimiter(0))
}

func lookupErrKindFC2(err error) string {
	var lookupErr *jav.LookupError
	if errors.As(err, &lookupErr) {
		return lookupErr.Kind
	}
	return ""
}

func containsFC2(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
