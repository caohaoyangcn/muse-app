package settings

import (
	"muse-app/backend/accesskey"
)

// The LAN access key is the one secret a phone needs to use the local web UI.
// It lives in the generic settings table (plaintext) because the desktop app
// displays it anyway: the threat model is another device on the same network,
// not an attacker who can already read the user's profile. Unlike provider
// cookies and API keys there is no DPAPI/Keychain layer here on purpose — the
// key must be readable on every platform and every startup, and wrapping it
// would add a failure mode without protecting a value the UI shows on screen.
const keyLanAccessKey = "lan_access_key"

// LanAccessKey returns the key that guards the LAN server, generating and
// persisting one on first use. A stored value that no longer normalizes (hand
// edited, truncated, written by an older build) is replaced, so a corrupt row
// locks devices out once instead of leaving the API without a usable key.
func (s *Store) LanAccessKey() (string, error) {
	raw, err := s.get(keyLanAccessKey)
	if err != nil {
		return "", err
	}
	if key, ok := accesskey.Normalize(raw); ok {
		return key, nil
	}
	return s.RotateLanAccessKey()
}

// RotateLanAccessKey generates and persists a new key. Every existing session
// cookie stops matching immediately; the desktop UI warns the user that phones
// must enter the new key.
func (s *Store) RotateLanAccessKey() (string, error) {
	key, err := accesskey.Generate()
	if err != nil {
		return "", err
	}
	if err := s.set(keyLanAccessKey, key); err != nil {
		return "", err
	}
	return key, nil
}
