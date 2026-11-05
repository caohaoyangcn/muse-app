package providers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"muse-app/backend/jav"
)

// RoutedProvider keeps FC2CMADB isolated from the ordinary JavBus/Avmoo chain.
// FC2 codes never fall through to a provider that interprets them as ordinary
// alphanumeric JAV codes.
type RoutedProvider struct {
	fc2      jav.Provider
	ordinary jav.Provider
	log      *slog.Logger
}

func NewRoutedProvider(fc2, ordinary jav.Provider) *RoutedProvider {
	return NewRoutedProviderWithLogger(fc2, ordinary, slog.Default())
}

func NewRoutedProviderWithLogger(fc2, ordinary jav.Provider, log *slog.Logger) *RoutedProvider {
	if log == nil {
		log = slog.Default()
	}
	return &RoutedProvider{fc2: fc2, ordinary: ordinary, log: log}
}

func (p *RoutedProvider) Name() string { return "jav-router" }

func (p *RoutedProvider) Lookup(ctx context.Context, code string) (*jav.Metadata, error) {
	if isFC2Code(code) {
		if p != nil && p.log != nil {
			p.log.Info("jav provider route", "route", "fc2cmadb", "code", strings.ToUpper(strings.TrimSpace(code)))
		}
		if p == nil || p.fc2 == nil {
			return nil, jav.NewLookupError("blocked", 0, fmt.Errorf("FC2CMADB provider is not configured"))
		}
		return p.fc2.Lookup(ctx, code)
	}
	if p != nil && p.log != nil {
		p.log.Info("jav provider route", "route", "ordinary", "code", strings.ToUpper(strings.TrimSpace(code)))
	}
	if p == nil || p.ordinary == nil {
		return nil, jav.NewLookupError("blocked", 0, fmt.Errorf("JAV provider is not configured"))
	}
	return p.ordinary.Lookup(ctx, code)
}

// FetchFor routes cover requests using the provider recorded in metadata.
// FC2CMADB remains the only client that can attach the FC2 session cookie.
func (p *RoutedProvider) FetchFor(ctx context.Context, providerName, rawURL string) (*http.Response, error) {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	if providerName == "fc2cmadb" {
		if fetcher, ok := p.fc2.(interface {
			FetchFor(context.Context, string, string) (*http.Response, error)
		}); ok {
			return fetcher.FetchFor(ctx, providerName, rawURL)
		}
		return nil, fmt.Errorf("provider %q cannot fetch covers", providerName)
	}
	if named, ok := p.ordinary.(interface {
		FetchFor(context.Context, string, string) (*http.Response, error)
	}); ok {
		return named.FetchFor(ctx, providerName, rawURL)
	}
	if fetcher, ok := p.ordinary.(interface {
		Fetch(context.Context, string) (*http.Response, error)
	}); ok {
		return fetcher.Fetch(ctx, rawURL)
	}
	return nil, fmt.Errorf("provider %q cannot fetch covers", providerName)
}

func (p *RoutedProvider) Fetch(ctx context.Context, rawURL string) (*http.Response, error) {
	return p.FetchFor(ctx, p.Name(), rawURL)
}

func isFC2Code(code string) bool {
	code = strings.ToUpper(strings.TrimSpace(code))
	return strings.HasPrefix(code, "FC2-") || strings.HasPrefix(code, "FC2_") || strings.HasPrefix(code, "FC2 ") || strings.HasPrefix(code, "FC2PPV")
}
