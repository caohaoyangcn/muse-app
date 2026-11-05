package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"muse-app/backend/jav"
)

type countingProvider struct {
	name  string
	meta  *jav.Metadata
	err   error
	calls atomic.Int32
}

func (p *countingProvider) Name() string { return p.name }
func (p *countingProvider) Lookup(context.Context, string) (*jav.Metadata, error) {
	p.calls.Add(1)
	return p.meta, p.err
}

func TestFallbackProviderUsesFallbackForBlockedOrNotFound(t *testing.T) {
	cases := []struct {
		name         string
		primaryErr   error
		wantFallback int32
		wantProvider string
	}{
		{name: "primary success", wantFallback: 0, wantProvider: "javbus"},
		{name: "blocked", primaryErr: jav.NewLookupError("blocked", http.StatusForbidden, errors.New("denied")), wantFallback: 1, wantProvider: "avmoo"},
		{name: "not found", primaryErr: jav.NewLookupError("not_found", http.StatusNotFound, nil), wantFallback: 1, wantProvider: "avmoo"},
		{name: "retryable", primaryErr: jav.NewLookupError("retryable", http.StatusTooManyRequests, nil), wantFallback: 0},
		{name: "network", primaryErr: jav.NewLookupError("network", 0, errors.New("offline")), wantFallback: 0},
		{name: "parse", primaryErr: jav.NewLookupError("parse", 0, errors.New("bad html")), wantFallback: 0},
		{name: "http", primaryErr: jav.NewLookupError("http", http.StatusBadGateway, nil), wantFallback: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			primary := &countingProvider{name: "javbus", meta: &jav.Metadata{Code: "IPX-228", Provider: "javbus"}, err: tc.primaryErr}
			fallback := &countingProvider{name: "avmoo", meta: &jav.Metadata{Code: "IPX-228", Provider: "avmoo"}}
			metadata, err := NewFallbackProvider(primary, fallback).Lookup(context.Background(), "IPX-228")
			if tc.primaryErr == nil {
				if err != nil || metadata == nil || metadata.Provider != "javbus" {
					t.Fatalf("metadata=%#v err=%v", metadata, err)
				}
			} else {
				if tc.wantFallback == 0 && err == nil {
					t.Fatal("expected primary error")
				}
				if tc.wantFallback == 1 && (err != nil || metadata == nil || metadata.Provider != tc.wantProvider) {
					t.Fatalf("metadata=%#v err=%v", metadata, err)
				}
			}
			if got := fallback.calls.Load(); got != tc.wantFallback {
				t.Fatalf("fallback calls=%d want=%d", got, tc.wantFallback)
			}
		})
	}
}

type responseProvider struct {
	*countingProvider
	cover func(context.Context, string) (*http.Response, error)
}

func (p *responseProvider) Fetch(ctx context.Context, rawURL string) (*http.Response, error) {
	return p.cover(ctx, rawURL)
}

func TestFallbackProviderRoutesNamedCoverFetch(t *testing.T) {
	var javbusRequests, avmooRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "not an image")
	}))
	defer server.Close()
	makeFetch := func(counter *atomic.Int32, header string) func(context.Context, string) (*http.Response, error) {
		return func(ctx context.Context, rawURL string) (*http.Response, error) {
			counter.Add(1)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("X-Provider", header)
			return http.DefaultClient.Do(req)
		}
	}
	primary := &responseProvider{countingProvider: &countingProvider{name: "javbus"}, cover: makeFetch(&javbusRequests, "javbus")}
	fallback := &responseProvider{countingProvider: &countingProvider{name: "avmoo"}, cover: makeFetch(&avmooRequests, "avmoo")}
	provider := NewFallbackProvider(primary, fallback)
	response, err := provider.FetchFor(context.Background(), "avmoo", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	response, err = provider.FetchFor(context.Background(), "javbus", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if javbusRequests.Load() != 1 || avmooRequests.Load() != 1 {
		t.Fatalf("cover request counts javbus=%d avmoo=%d", javbusRequests.Load(), avmooRequests.Load())
	}
}

func TestFallbackProviderPreservesFallbackError(t *testing.T) {
	fallbackErr := jav.NewLookupError("retryable", http.StatusBadGateway, errors.New("Avmoo unavailable"))
	primary := &countingProvider{name: "javbus", err: jav.NewLookupError("blocked", http.StatusForbidden, errors.New("JavBus blocked"))}
	fallback := &countingProvider{name: "avmoo", err: fallbackErr}
	_, err := NewFallbackProvider(primary, fallback).Lookup(context.Background(), "IPX-228")
	if !errors.Is(err, fallbackErr) {
		t.Fatalf("error=%v want fallback error", err)
	}
}
