//go:build !windows

package settings

func ensureSecretStorage() error {
	return ErrSecretStorageUnsupported
}

func protectSecret(string) (string, error) {
	return "", ErrSecretStorageUnsupported
}

func unprotectSecret(string) (string, error) {
	return "", ErrSecretStorageUnsupported
}
