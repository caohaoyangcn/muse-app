// Package secret protects small credentials at rest. It has no plaintext
// fallback: a platform that cannot protect secrets reports ErrUnsupported, and
// callers must either offer an anonymous configuration or refuse to persist the
// secret.
package secret

import "errors"

// ErrUnsupported means this build cannot persist secrets securely.
var ErrUnsupported = errors.New("secret storage is unsupported on this platform")

// Available reports whether Protect, Unprotect and Delete can work on this
// platform. Windows seals secrets with the current user's DPAPI key; macOS
// stores them in the login Keychain. There is intentionally no plaintext
// fallback: a platform without either reports ErrUnsupported.
func Available() error { return ensureAvailable() }
