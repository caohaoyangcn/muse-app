//go:build !windows

package secret

import (
	"errors"
	"testing"
)

func TestUnsupportedPlatformRefusesSecrets(t *testing.T) {
	if err := Available(); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Available err = %v, want ErrUnsupported", err)
	}
	if _, err := Protect("secret"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Protect err = %v, want ErrUnsupported", err)
	}
	if _, err := Unprotect("ciphertext"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Unprotect err = %v, want ErrUnsupported", err)
	}
}
