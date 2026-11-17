//go:build darwin

package secret

import (
	"strings"
	"testing"
)

// TestKeychainRoundTrip uses real login-keychain items with a random account
// per Protect call, so it never collides with a developer's saved Muse secrets.
func TestKeychainRoundTrip(t *testing.T) {
	if err := Available(); err != nil {
		t.Fatalf("Available: %v", err)
	}
	reference, err := Protect("sk-test-secret")
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	t.Cleanup(func() { _ = Delete(reference) })

	got, err := Unprotect(reference)
	if err != nil {
		t.Fatalf("Unprotect: %v", err)
	}
	if got != "sk-test-secret" {
		t.Fatalf("Unprotect = %q, want the stored secret", got)
	}

	// A second Protect must create a distinct item, so overwriting one saved
	// secret never disturbs another.
	other, err := Protect("second-secret")
	if err != nil {
		t.Fatalf("Protect second: %v", err)
	}
	t.Cleanup(func() { _ = Delete(other) })
	if other == reference {
		t.Fatal("Protect reused the same keychain account")
	}
	if got, _ := Unprotect(reference); got != "sk-test-secret" {
		t.Fatalf("first secret changed to %q", got)
	}

	// Delete is idempotent, which is what makes clearing a setting safe.
	if err := Delete(reference); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := Delete(reference); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
	if _, err := Unprotect(reference); err == nil {
		t.Fatal("deleted keychain item is still readable")
	}
}

func TestProtectRejectsInvalidPlaintext(t *testing.T) {
	for _, input := range []string{"", "line\nbreak", "carriage\rreturn"} {
		if _, err := Protect(input); err == nil {
			t.Fatalf("Protect(%q) should fail", input)
		}
	}
}

func TestUnprotectRejectsForeignValue(t *testing.T) {
	// A base64 DPAPI blob from a copied Windows database is not a Keychain
	// reference and must not be parsed as one.
	for _, encoded := range []string{"", "YWJjZGVm", "keychain:missing"} {
		if _, err := Unprotect(encoded); err == nil {
			t.Fatalf("Unprotect(%q) should fail", encoded)
		}
	}
	if err := Delete("YWJjZGVm"); err != nil {
		t.Fatalf("Delete of a foreign value should be a no-op: %v", err)
	}
}

func TestKeychainErrorMessageIsActionable(t *testing.T) {
	if _, err := Unprotect("muse:00000000000000000000000000000000"); err == nil {
		t.Fatal("expected an error for a missing keychain item")
	} else if !strings.Contains(err.Error(), "钥匙串") {
		t.Fatalf("error = %v, want a keychain-specific message", err)
	}
}
