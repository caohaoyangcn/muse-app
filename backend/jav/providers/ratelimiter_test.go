package providers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestIntervalLimiterSpacesRequests(t *testing.T) {
	limiter := newIntervalLimiter(20 * time.Millisecond)
	started := time.Now()

	for i := 0; i < 3; i++ {
		if err := limiter.Wait(context.Background()); err != nil {
			t.Fatalf("wait %d: %v", i, err)
		}
	}

	if elapsed := time.Since(started); elapsed < 35*time.Millisecond {
		t.Fatalf("three requests completed too quickly in %s", elapsed)
	}
}

func TestIntervalLimiterHonorsContextCancellation(t *testing.T) {
	limiter := newIntervalLimiter(time.Hour)
	if err := limiter.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := limiter.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait error = %v, want context deadline exceeded", err)
	}
}

func TestNewJavBusUsesSharedGlobalLimiter(t *testing.T) {
	first := NewJavBus(nil, "https://javbus.invalid")
	second := NewJavBus(nil, "https://javbus.invalid")
	if first.limiter != second.limiter {
		t.Fatal("JavBus instances do not share the global request limiter")
	}
}

func TestJavBusFetchesShareOneLimiter(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/SMD-133" {
			_, _ = w.Write([]byte(`<html><body><h3>SMD-133</h3><p>識別碼: SMD-133</p></body></html>`))
			return
		}
		_, _ = w.Write([]byte("cover"))
	}))
	defer srv.Close()

	limiter := newIntervalLimiter(0)
	provider := newJavBusWithLimiter(srv.Client(), srv.URL, RequestConfig{}, limiter)
	if _, err := provider.Lookup(context.Background(), "SMD-133"); err != nil {
		t.Fatal(err)
	}
	response, err := provider.Fetch(context.Background(), srv.URL+"/cover.jpg")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()

	if got := requests.Load(); got != 2 {
		t.Fatalf("HTTP requests = %d, want 2", got)
	}
}
