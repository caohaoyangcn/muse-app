// Package accesskey owns the secret that guards the LAN HTTP server: how it is
// generated and how a value typed or pasted by a user is normalized.
//
// A key is 10 random bytes encoded as 16 lowercase base32 characters (80 bits).
// That is short enough to read from the desktop screen and type on a phone, and
// long enough that guessing it over the network is hopeless even before the
// server throttles failed logins. The displayed form groups the characters
// (xxxx-xxxx-xxxx-xxxx); Normalize accepts that form, upper case, and the raw
// form the URL carries.
package accesskey

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
)

// Length is the normalized length of every access key.
const Length = 16

// keyBytes is the entropy behind one key: 10 bytes encode to exactly 16 base32
// characters without padding.
const keyBytes = 10

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// Generate returns a fresh random key in normalized form.
func Generate() (string, error) {
	raw := make([]byte, keyBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate LAN access key: %w", err)
	}
	return strings.ToLower(encoding.EncodeToString(raw)), nil
}

// Normalize canonicalizes a user-provided key. Case is ignored and the display
// separators (hyphen and whitespace) are dropped, so "ABCD-EFGH-IJKL-MNOP" and
// "abcdefghijklmnop" are the same key. ok is false when the value contains a
// character outside the base32 alphabet or the wrong number of characters;
// callers must treat that as a failed match, never as an empty key.
func Normalize(raw string) (key string, ok bool) {
	var b strings.Builder
	b.Grow(Length)
	for _, r := range strings.ToLower(raw) {
		switch {
		case r >= 'a' && r <= 'z', r >= '2' && r <= '7':
			b.WriteRune(r)
			if b.Len() > Length {
				return "", false
			}
		case r == '-', r == ' ', r == '\t', r == '\r', r == '\n':
			// Display separators are not part of the key.
		default:
			return "", false
		}
	}
	if b.Len() != Length {
		return "", false
	}
	return b.String(), true
}
