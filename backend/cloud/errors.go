package cloud

import "errors"

// Typed failures of a cloud source. Every transport error is wrapped so service
// callers can answer three very different questions without parsing strings:
// is the source offline, is it rejecting us, or is this one file gone?
var (
	// ErrUnauthorized means the WebDAV server rejected the credentials (401 or
	// 403). The favorite must be kept; the user has to fix the connection.
	ErrUnauthorized = errors.New("cloud source rejected the credentials")
	// ErrNotFound means the requested path does not exist (404). For a video
	// this is the only failure that may mark the favorite missing.
	ErrNotFound = errors.New("cloud path does not exist")
	// ErrUnreachable covers DNS, timeout, TLS and 5xx failures: the source is
	// offline or unhealthy, which says nothing about the files themselves.
	ErrUnreachable = errors.New("cloud source is unreachable")
	// ErrUnsupported means the server does not implement the WebDAV method or
	// the response shape the request needs.
	ErrUnsupported = errors.New("cloud source does not support this operation")
	// ErrPathOutsideRoot means a caller tried to escape the connection's
	// configured root_path. It is a programming error or a hostile path, never
	// a normal browse result.
	ErrPathOutsideRoot = errors.New("cloud path is outside the configured root")
	// ErrInvalidPath means the path is malformed (empty, relative, contains a
	// NUL or a `..` segment).
	ErrInvalidPath = errors.New("invalid cloud path")
	// ErrInvalidEndpoint means the WebDAV endpoint is not a usable HTTP(S) URL.
	ErrInvalidEndpoint = errors.New("invalid WebDAV endpoint")
)

// ErrRangeNotSatisfiable means the server rejected a byte range (HTTP 416).
// It is not a missing file: the cached size is probably stale and a fresh Stat
// is needed.
var ErrRangeNotSatisfiable = errors.New("cloud range is not satisfiable")

// ErrConnectionNotFound means the requested cloud_connections row is gone.
var ErrConnectionNotFound = errors.New("cloud connection not found")

// ErrPasswordStorageUnavailable means this platform cannot protect a password
// at rest. Anonymous WebDAV remains usable; a password must not be stored in
// plaintext as a fallback.
var ErrPasswordStorageUnavailable = errors.New("安全保存密码在当前平台不可用")
