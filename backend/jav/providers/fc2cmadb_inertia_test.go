package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const fc2InertiaShellFixture = `<!doctype html><html><body>
<div id="app"></div>
<input id="age-verification-modal" type="checkbox">
<script type="application/json" data-page="app">
{"component":"Articles/Show","version":"fixture-version","props":{"article":{"id":730132,"title":"FC2 作品标题","video_id":4749992,"censored":"無","uncensored_image":null,"not_found":false,"release_date":"2025-08-29T00:00:00.000000Z","duration":"01:12:49","image_url":"/storage/images/article/cover.jpg","writer":{"slug":"seller-slug","name":"Seller Name"},"tags":[{"name":"おっぱい"},{"name":"剧情"}]}},"deferredProps":{"default":["actresses"]}}
</script>
<script>const ageVerificationText = "age verification";</script>
</body></html>`

func TestFC2CMADBLookupParsesInertiaArticleAndDeferredActors(t *testing.T) {
	var mu sync.Mutex
	var requests []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Header.Clone())
		mu.Unlock()
		if r.URL.Path != "/articles/4749992" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Inertia") == "true" {
			if r.Header.Get("X-Inertia-Version") != "fixture-version" ||
				r.Header.Get("X-Inertia-Partial-Component") != "Articles/Show" ||
				r.Header.Get("X-Inertia-Partial-Data") != "actresses" {
				http.Error(w, "bad partial headers", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"component":"Articles/Show","version":"fixture-version","props":{"actresses":[{"name":"Alice"}]}}`)
			return
		}
		_, _ = fmt.Fprint(w, fc2InertiaShellFixture)
	}))
	defer srv.Close()

	provider := newTestFC2CMADB(srv.Client(), srv.URL, FC2CMADBConfig{Cookie: "ageVerified=true; fc2cmadb-session=fixture"})
	metadata, err := provider.Lookup(context.Background(), "FC2-4749992")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Code != "FC2-4749992" || metadata.Provider != "fc2cmadb" || metadata.Title != "FC2 作品标题" || metadata.Studio != "Seller Name" {
		t.Fatalf("identity/title = %#v", metadata)
	}
	if metadata.ReleaseUnix == nil || time.Unix(*metadata.ReleaseUnix, 0).UTC().Format("2006-01-02") != "2025-08-29" {
		t.Fatalf("release = %#v", metadata.ReleaseUnix)
	}
	if metadata.DurationMin == nil || *metadata.DurationMin != 72 {
		t.Fatalf("duration = %#v", metadata.DurationMin)
	}
	if metadata.IsUncensored == nil || !*metadata.IsUncensored {
		t.Fatalf("isUncensored = %#v", metadata.IsUncensored)
	}
	if metadata.CoverURL != srv.URL+"/storage/images/article/cover.jpg" {
		t.Fatalf("cover = %q", metadata.CoverURL)
	}
	if !containsFC2(metadata.Tags, "剧情") || !containsFC2(metadata.Actors, "Alice") {
		t.Fatalf("lists = tags=%#v actors=%#v", metadata.Tags, metadata.Actors)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	for i, headers := range requests {
		if headers.Get("Cookie") != "ageVerified=true; fc2cmadb-session=fixture" {
			t.Fatalf("request %d cookie was not sent", i)
		}
	}
}

func TestFC2CMADBDeferredActorsFailureDoesNotDiscardArticle(t *testing.T) {
	var partial bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Inertia") == "true" {
			partial = true
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = fmt.Fprint(w, fc2InertiaShellFixture)
	}))
	defer srv.Close()

	provider := newTestFC2CMADB(srv.Client(), srv.URL, FC2CMADBConfig{})
	metadata, err := provider.Lookup(context.Background(), "FC2-4749992")
	if err != nil {
		t.Fatal(err)
	}
	if !partial || metadata == nil || metadata.Title != "FC2 作品标题" {
		t.Fatalf("partial=%v metadata=%#v", partial, metadata)
	}
}

func TestFC2CMADBInertiaScriptAgeTextIsNotBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fc2InertiaShellFixture)
	}))
	defer srv.Close()

	provider := newTestFC2CMADB(srv.Client(), srv.URL, FC2CMADBConfig{})
	metadata, err := provider.Lookup(context.Background(), "FC2-4749992")
	if err != nil {
		t.Fatal(err)
	}
	if metadata == nil || metadata.Title != "FC2 作品标题" {
		t.Fatalf("metadata=%#v", metadata)
	}
}
