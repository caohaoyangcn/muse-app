package accesskey

import (
	"strings"
	"testing"
)

func TestGenerateProducesNormalizedKeys(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 32; i++ {
		key, err := Generate()
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if len(key) != Length {
			t.Fatalf("key %q has length %d, want %d", key, len(key), Length)
		}
		normalized, ok := Normalize(key)
		if !ok || normalized != key {
			t.Fatalf("generated key %q does not normalize to itself", key)
		}
		if _, dup := seen[key]; dup {
			t.Fatalf("generated duplicate key %q", key)
		}
		seen[key] = struct{}{}
	}
}

func TestNormalize(t *testing.T) {
	const canonical = "abcdefghijklmnop"
	cases := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{"raw", "abcdefghijklmnop", canonical, true},
		{"display groups", "ABCD-EFGH-IJKL-MNOP", canonical, true},
		{"spaces and case", "  ABCD EFGH IJKL MNOP\n", canonical, true},
		{"too short", "abcdefghijklmno", "", false},
		{"too long", "abcdefghijklmnopq", "", false},
		{"invalid character", "abcdefghijklmno0", "", false},
		{"empty", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Normalize(c.raw)
			if ok != c.ok || got != c.want {
				t.Fatalf("Normalize(%q) = (%q, %v), want (%q, %v)", c.raw, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestNormalizeRejectsOverflowEarly(t *testing.T) {
	// A long run of valid characters must not be normalized into a prefix that
	// accidentally matches a key.
	if _, ok := Normalize(strings.Repeat("a", Length+1)); ok {
		t.Fatal("over-long key was accepted")
	}
}
