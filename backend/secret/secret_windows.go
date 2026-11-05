//go:build windows

package secret

import (
	"encoding/base64"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func ensureAvailable() error { return nil }

// Protect seals plaintext with the current Windows user's DPAPI key.
func Protect(plaintext string) (string, error) {
	input := []byte(plaintext)
	defer clear(input)

	inputBlob := windows.DataBlob{Size: uint32(len(input)), Data: &input[0]}
	var outputBlob windows.DataBlob
	if err := windows.CryptProtectData(
		&inputBlob,
		nil,
		nil,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&outputBlob,
	); err != nil {
		return "", fmt.Errorf("DPAPI protect failed: %w", err)
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outputBlob.Data))) }()

	protected := unsafe.Slice(outputBlob.Data, outputBlob.Size)
	return base64.StdEncoding.EncodeToString(protected), nil
}

// Unprotect opens a value produced by Protect for the same Windows user.
func Unprotect(encoded string) (string, error) {
	protected, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("invalid protected secret encoding: %w", err)
	}
	if len(protected) == 0 {
		return "", fmt.Errorf("protected secret is empty")
	}
	defer clear(protected)

	inputBlob := windows.DataBlob{Size: uint32(len(protected)), Data: &protected[0]}
	var outputBlob windows.DataBlob
	if err := windows.CryptUnprotectData(
		&inputBlob,
		nil,
		nil,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&outputBlob,
	); err != nil {
		return "", fmt.Errorf("DPAPI unprotect failed: %w", err)
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outputBlob.Data))) }()

	plaintext := unsafe.Slice(outputBlob.Data, outputBlob.Size)
	result := string(plaintext)
	clear(plaintext)
	return result, nil
}
