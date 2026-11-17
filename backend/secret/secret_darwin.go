//go:build darwin

package secret

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	// securityPath is the system keychain CLI shipped with macOS.
	securityPath = "/usr/bin/security"
	// keychainService groups Muse's items in Keychain Access.
	keychainService = "com.caohaoyang.muse"
	// accountPrefix marks an encoded value as a Keychain reference. The
	// standard base64 alphabet used by the Windows DPAPI blob cannot contain
	// ':', so a value copied from another platform fails clearly instead of
	// being mistaken for a Keychain item.
	accountPrefix = "muse:"
	// keychainTimeout bounds one CLI call. The login keychain is unlocked while
	// the user is logged in; the timeout covers a dialog or a broken keychain
	// instead of hanging a settings save.
	keychainTimeout = 15 * time.Second
)

func ensureAvailable() error { return nil }

// Protect stores the plaintext in the login keychain and returns an opaque
// reference to it. Every call creates a distinct item, so overwriting one saved
// secret never touches another (the settings store deletes the previous
// reference after the new one is written).
func Protect(plaintext string) (string, error) {
	if plaintext == "" {
		return "", errors.New("secret must not be empty")
	}
	if strings.ContainsAny(plaintext, "\r\n") {
		return "", errors.New("secret must not contain CR or LF")
	}
	account, err := newKeychainAccount()
	if err != nil {
		return "", err
	}
	if err := addKeychainItem(account, plaintext); err != nil {
		return "", err
	}
	return account, nil
}

// Unprotect reads back a value produced by Protect.
func Unprotect(encoded string) (string, error) {
	account, ok := keychainAccount(encoded)
	if !ok {
		return "", errors.New("not a macOS Keychain secret reference")
	}
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()
	// `-w` prints only the password, without the item's attributes.
	out, err := runSecurity(ctx, "find-generic-password", "-s", keychainService, "-a", account, "-w")
	if err != nil {
		return "", fmt.Errorf("macOS 钥匙串读取失败：%w", err)
	}
	// The CLI appends one newline; secrets never contain line breaks.
	return strings.TrimSuffix(string(out), "\n"), nil
}

// Delete removes the keychain item behind a reference. It is a no-op for an
// encoded value this platform did not create (for example a Windows DPAPI blob
// in a copied database), and for an item that is already gone.
func Delete(encoded string) error {
	account, ok := keychainAccount(encoded)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()
	_, err := runSecurity(ctx, "delete-generic-password", "-s", keychainService, "-a", account)
	if err != nil {
		// The security CLI exits 44 when the item is not in the keychain. That
		// is the desired end state, so clearing a setting is idempotent.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 44 {
			return nil
		}
		return fmt.Errorf("macOS 钥匙串删除失败：%w", err)
	}
	return nil
}

func newKeychainAccount() (string, error) {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("generate keychain account: %w", err)
	}
	return accountPrefix + hex.EncodeToString(token), nil
}

func keychainAccount(encoded string) (string, bool) {
	encoded = strings.TrimSpace(encoded)
	if !strings.HasPrefix(encoded, accountPrefix) || len(encoded) <= len(accountPrefix) {
		return "", false
	}
	return encoded, true
}

// addKeychainItem stores the password under the account. -w without a value
// makes the CLI prompt for the password on stdin (twice), which keeps the
// secret out of the process arguments where `ps` could read it.
func addKeychainItem(account, plaintext string) error {
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, securityPath, "add-generic-password", "-U", "-s", keychainService, "-a", account, "-w")
	cmd.Stdin = strings.NewReader(plaintext + "\n" + plaintext + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return fmt.Errorf("macOS 钥匙串写入失败：%w（%s）", err, message)
		}
		return fmt.Errorf("macOS 钥匙串写入失败：%w", err)
	}
	return nil
}

func runSecurity(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, securityPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("%w（%s）", err, message)
		}
		return nil, err
	}
	return out, nil
}
