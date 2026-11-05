package cloud

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const propfindBody = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:resourcetype/>
    <d:getcontentlength/>
    <d:getlastmodified/>
    <d:getcontenttype/>
    <d:displayname/>
  </d:prop>
</d:propfind>`

// Client is a read-only WebDAV client for one connection. It never issues PUT,
// DELETE, MOVE, COPY or MKCOL. It is safe for concurrent use: requests carry
// their own context and the underlying http.Client is shared read-only.
type Client struct {
	endpoint *url.URL
	root     string
	username string
	password string
	http     *http.Client
}

// NewClient validates one connection and builds its HTTP client. password is
// the decrypted secret, kept only in memory and never logged.
func NewClient(conn Connection, password string) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(conn.Endpoint))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, ErrInvalidEndpoint
	}
	if u.User != nil {
		// Credentials belong in the username/password fields, where they can be
		// protected at rest; a URL userinfo would leak into logs and the UI.
		return nil, ErrInvalidEndpoint
	}
	root, err := normalizePath(strings.TrimSpace(conn.RootPath))
	if err != nil {
		return nil, fmt.Errorf("%w: root path", ErrInvalidPath)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 15 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ExpectContinueTimeout = time.Second
	if !conn.VerifyTLS {
		// Per-connection opt-in for a self-signed OpenList certificate. The UI
		// warns about it; credentials still travel over TLS, only verification
		// is skipped.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit user opt-in
	}

	return &Client{
		endpoint: u,
		root:     root,
		username: conn.Username,
		password: password,
		http:     &http.Client{Transport: transport},
	}, nil
}

// Endpoint returns the normalized endpoint without credentials.
func (c *Client) Endpoint() string { return c.endpoint.String() }

// Root returns the normalized root path.
func (c *Client) Root() string { return c.root }

// scoped validates a caller path and enforces the connection root.
func (c *Client) scoped(raw string) (string, error) {
	p, err := normalizePath(raw)
	if err != nil {
		return "", err
	}
	if !withinRoot(c.root, p) {
		return "", ErrPathOutsideRoot
	}
	return p, nil
}

// urlFor builds the request URL for an endpoint-relative path.
func (c *Client) urlFor(p string) string {
	u := *c.endpoint
	u.Path = joinURLPath(c.endpoint.Path, p)
	u.RawPath = ""
	return u.String()
}

// newRequest builds an authenticated request. It is the only place credentials
// are attached.
func (c *Client) newRequest(ctx context.Context, method, p string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.urlFor(p), body)
	if err != nil {
		return nil, fmt.Errorf("build WebDAV request: %w", err)
	}
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	return req, nil
}

// Test checks that the connection's root is reachable and readable. It returns
// the round-trip latency for the settings screen.
func (c *Client) Test(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	if _, err := c.Stat(ctx, c.root); err != nil {
		return time.Since(start), err
	}
	return time.Since(start), nil
}

// Stat describes one resource with PROPFIND Depth 0.
func (c *Client) Stat(ctx context.Context, raw string) (*Entry, error) {
	p, err := c.scoped(raw)
	if err != nil {
		return nil, err
	}
	entries, err := c.propfind(ctx, p, "0")
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Path == p {
			return &entries[i], nil
		}
	}
	if len(entries) > 0 {
		// Some servers omit or rewrite href for the collection root. Depth 0
		// returned exactly the requested resource, so its first entry is the
		// answer even when the path spelling differs.
		return &entries[0], nil
	}
	return nil, ErrNotFound
}

// List returns the direct children of a collection with PROPFIND Depth 1. The
// collection itself is omitted. Directories and files are returned as the
// server sent them; presentation ordering belongs to the service/UI.
func (c *Client) List(ctx context.Context, raw string) ([]Entry, error) {
	p, err := c.scoped(raw)
	if err != nil {
		return nil, err
	}
	entries, err := c.propfind(ctx, p, "1")
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.Path == p {
			continue
		}
		if !withinRoot(c.root, entry.Path) {
			continue
		}
		if _, dup := seen[entry.Path]; dup {
			continue
		}
		seen[entry.Path] = struct{}{}
		out = append(out, entry)
	}
	return out, nil
}

// Head asks for a resource's headers without reading its body. It is the cheap
// existence check used before marking a remote favorite missing.
func (c *Client) Head(ctx context.Context, raw string) (*Entry, error) {
	p, err := c.scoped(raw)
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, http.MethodHead, p, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, wrapNetworkError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, statusError(resp.StatusCode)
	}
	size := resp.ContentLength
	if size < 0 {
		size = 0
	}
	modified, _ := http.ParseTime(resp.Header.Get("Last-Modified"))
	return &Entry{
		Name:        pathBase(p),
		Path:        p,
		Size:        size,
		Modified:    modified.UnixMilli(),
		ContentType: resp.Header.Get("Content-Type"),
	}, nil
}

// OpenRange opens a byte range of a remote file. A 206 is a real partial
// response; a 200 means the server ignored Range and Body is the whole file.
// The caller owns Body.
func (c *Client) OpenRange(ctx context.Context, raw string, offset, length int64) (*RangeResponse, error) {
	if offset < 0 || length < 0 {
		return nil, fmt.Errorf("%w: negative range", ErrInvalidPath)
	}
	p, err := c.scoped(raw)
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, http.MethodGet, p, nil)
	if err != nil {
		return nil, err
	}
	if length > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, wrapNetworkError(err)
	}
	switch resp.StatusCode {
	case http.StatusPartialContent, http.StatusOK:
		// accepted below
	case http.StatusRequestedRangeNotSatisfiable:
		_ = resp.Body.Close()
		return nil, ErrRangeNotSatisfiable
	default:
		_ = resp.Body.Close()
		return nil, statusError(resp.StatusCode)
	}

	rr := &RangeResponse{
		Body:          resp.Body,
		StatusCode:    resp.StatusCode,
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
		ContentRange:  resp.Header.Get("Content-Range"),
	}
	if rr.ContentLength < 0 {
		rr.ContentLength = 0
	}
	if resp.StatusCode == http.StatusPartialContent {
		rr.TotalSize = parseContentRangeTotal(rr.ContentRange)
	} else {
		rr.TotalSize = rr.ContentLength
	}
	return rr, nil
}

// propfind performs the request and decodes its 207 response.
func (c *Client) propfind(ctx context.Context, p, depth string) ([]Entry, error) {
	req, err := c.newRequest(ctx, "PROPFIND", p, strings.NewReader(propfindBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", depth)
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Accept", "application/xml, text/xml")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, wrapNetworkError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		return nil, statusError(resp.StatusCode)
	}
	// Bound the XML read: a malformed/hostile server must not be able to make
	// the desktop app allocate without limit.
	limited := io.LimitReader(resp.Body, 16<<20)
	var ms davMultistatus
	if err := xml.NewDecoder(limited).Decode(&ms); err != nil {
		return nil, fmt.Errorf("parse WebDAV response: %w", ErrUnsupported)
	}
	entries := make([]Entry, 0, len(ms.Responses))
	for _, response := range ms.Responses {
		entry, ok := c.entryFromResponse(c.endpoint.Path, p, response)
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// --- XML shapes ---------------------------------------------------------

type davMultistatus struct {
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href      string        `xml:"href"`
	Propstats []davPropstat `xml:"propstat"`
}

type davPropstat struct {
	Status string  `xml:"status"`
	Prop   davProp `xml:"prop"`
}

type davProp struct {
	DisplayName   string `xml:"displayname"`
	ContentLength string `xml:"getcontentlength"`
	LastModified  string `xml:"getlastmodified"`
	ContentType   string `xml:"getcontenttype"`
	ResourceType  struct {
		Collection []struct{} `xml:"collection"`
	} `xml:"resourcetype"`
}

func (c *Client) entryFromResponse(basePath, requestPath string, response davResponse) (Entry, bool) {
	// A resource can have one propstat per status; use the successful one.
	var prop *davProp
	for i := range response.Propstats {
		if propstatOK(response.Propstats[i].Status) {
			prop = &response.Propstats[i].Prop
			break
		}
	}
	if prop == nil && len(response.Propstats) > 0 {
		return Entry{}, false
	}
	p, err := stripURLBase(basePath, requestPath, response.Href)
	if err != nil {
		return Entry{}, false
	}
	entry := Entry{Path: p, Name: pathBase(p)}
	if prop != nil {
		if name := prop.DisplayName; strings.TrimSpace(name) != "" {
			// Preserve the server's spelling: a trailing space is a legal
			// filename character, and displayname is what the user should see.
			entry.Name = name
		}
		entry.IsDir = len(prop.ResourceType.Collection) > 0
		entry.Size = parseSize(prop.ContentLength)
		entry.Modified = parseDAVTime(prop.LastModified).UnixMilli()
		entry.ContentType = strings.TrimSpace(prop.ContentType)
	}
	return entry, true
}

func propstatOK(status string) bool {
	fields := strings.Fields(status)
	return len(fields) >= 2 && strings.HasPrefix(fields[1], "2")
}

func parseSize(raw string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func parseDAVTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	// DAV:getlastmodified is an HTTP-date, but some servers send RFC3339.
	for _, layout := range []string{http.TimeFormat, time.RFC1123, time.RFC1123Z, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

func pathBase(p string) string {
	if p == "/" || p == "" {
		return "/"
	}
	if i := strings.LastIndex(p, "/"); i >= 0 && i < len(p)-1 {
		return p[i+1:]
	}
	return p
}

func parseContentRangeTotal(raw string) int64 {
	// bytes 0-1023/4096
	slash := strings.LastIndexByte(raw, '/')
	if slash < 0 {
		return 0
	}
	total, err := strconv.ParseInt(strings.TrimSpace(raw[slash+1:]), 10, 64)
	if err != nil || total < 0 {
		return 0
	}
	return total
}

func statusError(code int) error {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w (HTTP %d)", ErrUnauthorized, code)
	case http.StatusNotFound, http.StatusGone:
		return fmt.Errorf("%w (HTTP %d)", ErrNotFound, code)
	case http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return fmt.Errorf("%w (HTTP %d)", ErrUnsupported, code)
	case http.StatusRequestedRangeNotSatisfiable:
		return ErrRangeNotSatisfiable
	}
	if code >= 500 {
		return fmt.Errorf("%w (HTTP %d)", ErrUnreachable, code)
	}
	return fmt.Errorf("unexpected WebDAV response (HTTP %d): %w", code, ErrUnsupported)
}

func wrapNetworkError(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrUnreachable, err)
}
