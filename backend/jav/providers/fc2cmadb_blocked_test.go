package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFC2CMADBDoesNotTreatOrdinaryLoginOrCloudflareTextAsAgeGate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><head><title>FC2CMADB</title></head><body>
<nav><a href="/login">Login</a><span>Cloudflare protects this site</span></nav>
<h1>作品标题</h1></body></html>`)
	}))
	defer srv.Close()

	provider := newTestFC2CMADB(srv.Client(), srv.URL, FC2CMADBConfig{Cookie: "ageVerified=true; fc2cmadb-session=fixture"})
	metadata, err := provider.Lookup(context.Background(), "FC2-4970405")
	if err != nil {
		t.Fatalf("ordinary login/cloudflare text was classified as blocked: %v", err)
	}
	if metadata == nil || metadata.Title != "作品标题" {
		t.Fatalf("metadata=%#v", metadata)
	}
}
