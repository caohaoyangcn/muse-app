package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Client routes (tabs, detail ids, actor names) are not files. The static
// handler must answer them with the SPA shell so a deep link or a refresh on a
// pushed screen does not 404.
func TestServeWebUIResolvesClientRoutes(t *testing.T) {
	paths := []string{
		"/",
		"/videos/42",
		"/videos/not-an-id",
		"/actor?name=Aoi%20Sora",
		"/actors",
		"/scrape",
		"/duplicates",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, p, nil)

			serveWebUI(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
				t.Fatalf("content type = %q, want SPA shell", got)
			}
			if !strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
				t.Fatalf("body is not the SPA shell: %q", rec.Body.String())
			}
		})
	}
}

// A real embedded file must still be served as itself rather than replaced by
// the SPA fallback.
func TestServeWebUIServesEmbeddedAsset(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)

	serveWebUI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/javascript; charset=utf-8" {
		t.Fatalf("content type = %q, want JavaScript", got)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("asset body is empty")
	}
}
