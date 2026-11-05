//go:build !windows

package secret

func ensureAvailable() error { return ErrUnsupported }

func Protect(string) (string, error) { return "", ErrUnsupported }

func Unprotect(string) (string, error) { return "", ErrUnsupported }
