package jav

import (
	"context"
	"fmt"
	"net/http"
)

type Provider interface {
	Name() string
	Lookup(ctx context.Context, code string) (*Metadata, error)
}

// Fetcher exposes the provider's configured HTTP transport for secondary
// requests, such as downloading a cover URL returned by Lookup.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) (*http.Response, error)
}

type LookupError struct {
	Kind       string
	StatusCode int
	Err        error
}

func (e *LookupError) Error() string {
	if e.Err == nil {
		return e.Kind
	}
	return fmt.Sprintf("%s: %v", e.Kind, e.Err)
}

func (e *LookupError) Unwrap() error { return e.Err }

func NewLookupError(kind string, statusCode int, err error) error {
	return &LookupError{Kind: kind, StatusCode: statusCode, Err: err}
}
