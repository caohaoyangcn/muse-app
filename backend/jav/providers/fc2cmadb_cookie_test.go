package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFC2CMADBSetCookieNormalizesFullHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Cookie")
		_, _ = fmt.Fprint(w, `<html><body><h1>作品标题</h1></body></html>`)
	}))
	defer srv.Close()

	provider := newTestFC2CMADB(srv.Client(), srv.URL, FC2CMADBConfig{})
	provider.SetCookie("Cookie: ageVerified=true; fc2cmadb-session=fixture")
	if _, err := provider.Lookup(context.Background(), "FC2-4970405"); err != nil {
		t.Fatal(err)
	}
	if got != "ageVerified=true; fc2cmadb-session=fixture" {
		t.Fatalf("cookie header=%q", cookieKeysFC2(got))
	}
}
