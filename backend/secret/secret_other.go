//go:build !windows && !darwin

package secret

func ensureAvailable() error { return ErrUnsupported }

func Protect(string) (string, error) { return "", ErrUnsupported }

func Unprotect(string) (string, error) { return "", ErrUnsupported }
