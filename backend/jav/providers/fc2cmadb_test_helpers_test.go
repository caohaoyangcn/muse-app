package providers

import "strings"

func cookieKeysFC2(value string) string {
	parts := strings.Split(value, ";")
	keys := make([]string, 0, len(parts))
	for _, part := range parts {
		key := strings.TrimSpace(part)
		if index := strings.IndexByte(key, '='); index >= 0 {
			keys = append(keys, key[:index])
		}
	}
	return strings.Join(keys, ",")
}
