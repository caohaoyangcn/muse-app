package cloud

import (
	"net/url"
	"path"
	"strings"
)

// normalizePath canonicalizes an endpoint-relative WebDAV path. Paths are
// always absolute and POSIX-style. `..` is rejected before path.Clean can hide
// it: silently resolving an escape attempt would make root scoping meaningless.
func normalizePath(raw string) (string, error) {
	if strings.IndexByte(raw, 0) >= 0 {
		return "", ErrInvalidPath
	}
	if raw == "" {
		return "/", nil
	}
	if !strings.HasPrefix(raw, "/") {
		return "", ErrInvalidPath
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == ".." {
			return "", ErrInvalidPath
		}
	}
	cleaned := path.Clean(raw)
	if cleaned == "." {
		cleaned = "/"
	}
	if !strings.HasPrefix(cleaned, "/") {
		return "", ErrInvalidPath
	}
	return cleaned, nil
}

// withinRoot reports whether p is root itself or a descendant. Both arguments
// are normalized before this is called.
func withinRoot(root, p string) bool {
	if root == "/" {
		return true
	}
	return p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/")
}

// joinURLPath appends an endpoint-relative logical path to the endpoint URL's
// own path. The result is decoded; url.URL.String re-escapes each segment,
// which is what makes Chinese and spaces safe on the wire.
func joinURLPath(base, logical string) string {
	base = strings.TrimSuffix(base, "/")
	if base == "" || base == "/" {
		return logical
	}
	return base + logical
}

// stripURLBase removes the endpoint URL's path prefix from a PROPFIND href, so
// an href like /dav/媒体/a.mp4 becomes /媒体/a.mp4. Servers are allowed to
// return absolute URLs, relative references or already-unescaped hrefs.
func stripURLBase(base, requestPath, href string) (string, error) {
	raw := strings.TrimSpace(href)
	if raw == "" {
		return "", ErrInvalidPath
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", ErrInvalidPath
	}
	p := parsed.Path
	if p == "" {
		p = parsed.EscapedPath()
		if decoded, decodeErr := url.PathUnescape(p); decodeErr == nil {
			p = decoded
		}
	}
	if p == "" {
		return "", ErrInvalidPath
	}
	if !strings.HasPrefix(p, "/") {
		// A relative href names a child of the collection that was requested.
		// Most servers send absolute paths; this fallback keeps a
		// non-conforming server usable without inventing a wrong root.
		p = path.Join(requestPath, p)
	}
	base = strings.TrimSuffix(base, "/")
	if base != "" && base != "/" {
		switch {
		case p == base:
			p = "/"
		case strings.HasPrefix(p, base+"/"):
			p = strings.TrimPrefix(p, base)
		}
	}
	return normalizePath(p)
}
