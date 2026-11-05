package providers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"muse-app/backend/jav"
)

func TestRoutedProviderKeepsFC2OutOfOrdinaryFallback(t *testing.T) {
	fc2 := &countingProvider{name: "fc2cmadb", meta: &jav.Metadata{Code: "FC2-4970405", Provider: "fc2cmadb"}}
	ordinary := &countingProvider{name: "javbus", meta: &jav.Metadata{Code: "IPX-633", Provider: "javbus"}}
	provider := NewRoutedProvider(fc2, ordinary)

	metadata, err := provider.Lookup(context.Background(), "FC2-4970405")
	if err != nil || metadata == nil || metadata.Provider != "fc2cmadb" {
		t.Fatalf("FC2 metadata=%#v err=%v", metadata, err)
	}
	if got := fc2.calls.Load(); got != 1 {
		t.Fatalf("FC2 calls=%d want=1", got)
	}
	if got := ordinary.calls.Load(); got != 0 {
		t.Fatalf("ordinary calls=%d want=0", got)
	}
}

func TestRoutedProviderUsesOrdinaryForNormalCode(t *testing.T) {
	fc2 := &countingProvider{name: "fc2cmadb", meta: &jav.Metadata{Code: "FC2-4970405", Provider: "fc2cmadb"}}
	ordinary := &countingProvider{name: "javbus", meta: &jav.Metadata{Code: "IPX-633", Provider: "javbus"}}
	provider := NewRoutedProvider(fc2, ordinary)

	metadata, err := provider.Lookup(context.Background(), "IPX-633")
	if err != nil || metadata == nil || metadata.Provider != "javbus" {
		t.Fatalf("ordinary metadata=%#v err=%v", metadata, err)
	}
	if fc2.calls.Load() != 0 || ordinary.calls.Load() != 1 {
		t.Fatalf("calls fc2=%d ordinary=%d", fc2.calls.Load(), ordinary.calls.Load())
	}
}

func TestRoutedProviderReturnsFC2ErrorWithoutFallback(t *testing.T) {
	fc2Err := jav.NewLookupError("provider-auth", http.StatusForbidden, errors.New("session rejected"))
	fc2 := &countingProvider{name: "fc2cmadb", err: fc2Err}
	ordinary := &countingProvider{name: "javbus", meta: &jav.Metadata{Code: "IPX-633", Provider: "javbus"}}
	provider := NewRoutedProvider(fc2, ordinary)

	_, err := provider.Lookup(context.Background(), "FC2-4970405")
	if !errors.Is(err, fc2Err) {
		t.Fatalf("error=%v want=%v", err, fc2Err)
	}
	if ordinary.calls.Load() != 0 {
		t.Fatal("ordinary provider was called after FC2 failure")
	}
}

func TestIsFC2CodeVariants(t *testing.T) {
	for _, code := range []string{"FC2-4970405", "fc2_ppv_4970405", "FC2PPV4970405"} {
		if !isFC2Code(code) {
			t.Fatalf("isFC2Code(%q)=false", code)
		}
	}
	if isFC2Code("FC-2-4970405") {
		t.Fatal("FC-2 should not be treated as FC2")
	}
}
