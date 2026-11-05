package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFC2CMADBUsesBrowserNavigationHeaders(t *testing.T) {
	got := make(http.Header)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = fmt.Fprint(w, `<html><body><h1>作品标题</h1></body></html>`)
	}))
	defer srv.Close()

	provider := newTestFC2CMADB(srv.Client(), srv.URL, FC2CMADBConfig{Cookie: "ageVerified=true; fc2cmadb-session=fixture"})
	if _, err := provider.Lookup(context.Background(), "FC2-4970405"); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Accept-Language":           fc2CMADBAcceptLanguage,
		"Cache-Control":             "max-age=0",
		"DNT":                       "1",
		"Priority":                  fc2CMADBPriority,
		"Sec-CH-UA":                 fc2CMADBSecCHUA,
		"Sec-CH-UA-Mobile":          "?0",
		"Sec-CH-UA-Platform":        fc2CMADBSecCHPlatform,
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "same-origin",
		"Sec-Fetch-User":            "?1",
		"Upgrade-Insecure-Requests": "1",
	}
	for key, value := range want {
		if got.Get(key) != value {
			t.Fatalf("header %s=%q want=%q", key, got.Get(key), value)
		}
	}
	if got.Get("User-Agent") != fc2CMADBUserAgent || got.Get("Accept") != fc2CMADBAccept {
		t.Fatalf("browser headers not reproduced")
	}
}
