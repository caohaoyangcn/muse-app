package providers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"muse-app/backend/jav"
)

// FallbackProvider tries the secondary provider when the primary provider
// explicitly reports a blocked or not-found condition. Other failures remain
// owned by the normal JavService retry/status machinery.
type FallbackProvider struct {
	primary  jav.Provider
	fallback jav.Provider
	log      *slog.Logger
}

func NewFallbackProvider(primary, fallback jav.Provider) *FallbackProvider {
	return &FallbackProvider{primary: primary, fallback: fallback, log: slog.Default()}
}

func NewFallbackProviderWithLogger(primary, fallback jav.Provider, log *slog.Logger) *FallbackProvider {
	if log == nil {
		log = slog.Default()
	}
	return &FallbackProvider{primary: primary, fallback: fallback, log: log}
}

// Name stays compatible with the existing service and identifies the primary
// chain. Successful metadata carries the actual provider name.
func (p *FallbackProvider) Name() string {
	if p == nil || p.primary == nil {
		return "javbus"
	}
	return p.primary.Name()
}

func (p *FallbackProvider) Lookup(ctx context.Context, code string) (*jav.Metadata, error) {
	if p == nil || p.primary == nil {
		if p == nil || p.fallback == nil {
			return nil, jav.NewLookupError("blocked", 0, errors.New("no scrape provider configured"))
		}
		return p.fallback.Lookup(ctx, code)
	}
	metadata, err := p.primary.Lookup(ctx, code)
	if err == nil {
		return metadata, nil
	}
	if !isFallbackLookupError(err) || p.fallback == nil {
		return nil, err
	}
	p.log.Info("jav provider fallback", "primary", p.primary.Name(), "fallback", p.fallback.Name(), "code", strings.TrimSpace(code))
	metadata, fallbackErr := p.fallback.Lookup(ctx, code)
	if fallbackErr != nil {
		p.log.Warn("jav provider fallback failed", "primary", p.primary.Name(), "fallback", p.fallback.Name(), "code", strings.TrimSpace(code), "err", fallbackErr)
		return nil, fallbackErr
	}
	if metadata != nil {
		metadata.Provider = p.fallback.Name()
	}
	return metadata, nil
}

func isFallbackLookupError(err error) bool {
	var lookupErr *jav.LookupError
	if !errors.As(err, &lookupErr) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(lookupErr.Kind, "_", "-"))) {
	case "blocked", "not-found":
		return true
	default:
		return false
	}
}

// FetchFor routes cover requests according to the provider that produced the
// metadata. It intentionally avoids copying cookies between providers.
func (p *FallbackProvider) FetchFor(ctx context.Context, providerName, rawURL string) (*http.Response, error) {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	selected := p.primary
	if providerName != "" && p.fallback != nil && providerName == strings.ToLower(strings.TrimSpace(p.fallback.Name())) {
		selected = p.fallback
	}
	if selected == nil {
		return nil, fmt.Errorf("provider %q is not configured", providerName)
	}
	return fetchResponse(ctx, selected, rawURL)
}

// Fetch preserves the historical optional fetch contract for callers that do
// not carry a provider name; it uses the primary provider.
func (p *FallbackProvider) Fetch(ctx context.Context, rawURL string) (*http.Response, error) {
	return p.FetchFor(ctx, p.Name(), rawURL)
}

func fetchResponse(ctx context.Context, provider jav.Provider, rawURL string) (*http.Response, error) {
	if fetcher, ok := provider.(interface {
		Fetch(context.Context, string) (*http.Response, error)
	}); ok {
		return fetcher.Fetch(ctx, rawURL)
	}
	return nil, fmt.Errorf("provider %q cannot fetch covers", provider.Name())
}
