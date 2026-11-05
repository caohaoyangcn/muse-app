// Package secret protects small credentials at rest. It has no plaintext
// fallback: a platform that cannot protect secrets reports ErrUnsupported, and
// callers must either offer an anonymous configuration or refuse to persist the
// secret.
package secret

import "errors"

// ErrUnsupported means this build cannot persist secrets securely.
var ErrUnsupported = errors.New("secret storage is unsupported on this platform")

// Available reports whether Protect and Unprotect can work on this platform.
func Available() error { return ensureAvailable() }
