package cloud

import "time"

// Entry is one WebDAV resource as PROPFIND described it. Path is an absolute,
// percent-decoded path relative to the connection's endpoint (not including the
// endpoint URL's own path and not including root_path). That makes it stable
// when the OpenList address changes and keeps it independent of the connection
// scope.
type Entry struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	IsDir       bool      `json:"isDir"`
	Size        int64     `json:"size"`
	Modified    time.Time `json:"modified"`
	ContentType string    `json:"contentType"`
}

// Connection is one persisted WebDAV source. PasswordEnc is the platform-
// protected secret column and is never serialized to clients; the client gets
// the decrypted password separately.
type Connection struct {
	ID          int64
	Name        string
	Kind        string
	Endpoint    string
	WebURL      string
	Username    string
	PasswordEnc string
	RootPath    string
	VerifyTLS   bool
	CreatedAt   int64
	UpdatedAt   int64
	LastOKAt    int64
	LastError   string
}

// RangeResponse is an in-progress media response. The caller owns Body and must
// close it. A StatusCode of 200 means the server ignored Range and the body is
// the file from byte 0; 206 means the requested slice starts at ContentRange.
type RangeResponse struct {
	Body          ReadCloser
	StatusCode    int
	ContentType   string
	ContentLength int64
	ContentRange  string
	// TotalSize is the full resource size when known (from Content-Range or
	// Content-Length on a 200 response). 0 means unknown.
	TotalSize int64
}

// ReadCloser is io.ReadCloser, named here so callers do not need the io import
// just to describe a RangeResponse.
type ReadCloser interface {
	Read([]byte) (int, error)
	Close() error
}
