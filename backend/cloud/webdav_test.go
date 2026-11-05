package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const multistatusPrefix = `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">`

func davResponseXML(href, displayName, length, modified, contentType string, collection bool) string {
	resourceType := ""
	if collection {
		resourceType = "<d:collection/>"
	}
	return fmt.Sprintf(`<d:response><d:href>%s</d:href><d:propstat><d:prop>`+
		`<d:displayname>%s</d:displayname>`+
		`<d:getcontentlength>%s</d:getcontentlength>`+
		`<d:getlastmodified>%s</d:getlastmodified>`+
		`<d:getcontenttype>%s</d:getcontenttype>`+
		`<d:resourcetype>%s</d:resourcetype>`+
		`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`,
		href, displayName, length, modified, contentType, resourceType)
}

func testConnection(endpoint string) Connection {
	return Connection{
		Name:      "测试",
		Endpoint:  endpoint,
		Username:  "alice",
		RootPath:  "/媒体",
		VerifyTLS: true,
	}
}

func TestClientListParsesChineseEscapedHref(t *testing.T) {
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			t.Errorf("method = %s, want PROPFIND", r.Method)
		}
		if r.URL.Path != "/dav/媒体" {
			t.Errorf("request path = %q, want /dav/媒体", r.URL.Path)
		}
		if r.Header.Get("Depth") != "1" {
			t.Errorf("Depth = %q, want 1", r.Header.Get("Depth"))
		}
		if user, pass, ok := r.BasicAuth(); !ok || user != "alice" || pass != "s3cret" {
			t.Errorf("basic auth = %q/%q/%v", user, pass, ok)
		} else {
			sawAuth = true
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, multistatusPrefix+
			davResponseXML("/dav/%E5%AA%92%E4%BD%93", "媒体", "", "Wed, 01 Jan 2025 02:03:04 GMT", "", true)+
			davResponseXML("http://example.test/dav/%E5%AA%92%E4%BD%93/a%20b.mp4", "a b.mp4", "123", "2025-01-01T02:03:04Z", "video/mp4", false)+
			davResponseXML("/dav/%E5%AA%92%E4%BD%93/%E5%AD%90%E7%9B%AE%E5%BD%95", "子目录", "", "", "", true)+
			`</d:multistatus>`)
	}))
	defer srv.Close()

	client, err := NewClient(testConnection(srv.URL+"/dav"), "s3cret")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	entries, err := client.List(context.Background(), "/媒体")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !sawAuth {
		t.Error("request did not carry Basic Auth")
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want 2 children (self omitted)", entries)
	}
	file := entries[0]
	if file.Name != "a b.mp4" || file.Path != "/媒体/a b.mp4" || file.IsDir || file.Size != 123 {
		t.Errorf("file entry = %+v", file)
	}
	if file.ContentType != "video/mp4" || file.Modified == 0 {
		t.Errorf("file metadata = %+v", file)
	}
	dir := entries[1]
	if dir.Name != "子目录" || dir.Path != "/媒体/子目录" || !dir.IsDir {
		t.Errorf("dir entry = %+v", dir)
	}
}

func TestClientStatAndRootScoping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, multistatusPrefix+
			davResponseXML("/dav/%E5%AA%92%E4%BD%93/%E5%AD%90%E7%9B%AE%E5%BD%95", "子目录", "", "", "", true)+
			`</d:multistatus>`)
	}))
	defer srv.Close()

	client, err := NewClient(testConnection(srv.URL+"/dav"), "")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	entry, err := client.Stat(context.Background(), "/媒体/子目录")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if entry.Path != "/媒体/子目录" || !entry.IsDir {
		t.Errorf("stat entry = %+v", entry)
	}
	if _, err := client.List(context.Background(), "/其他"); !errors.Is(err, ErrPathOutsideRoot) {
		t.Errorf("outside root err = %v, want ErrPathOutsideRoot", err)
	}
	if _, err := client.List(context.Background(), "/媒体/../其他"); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("dot-dot err = %v, want ErrInvalidPath", err)
	}
	if _, err := client.List(context.Background(), "relative"); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("relative err = %v, want ErrInvalidPath", err)
	}
}

func TestClientErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, ErrUnauthorized},
		{"forbidden", http.StatusForbidden, ErrUnauthorized},
		{"not found", http.StatusNotFound, ErrNotFound},
		{"server error", http.StatusInternalServerError, ErrUnreachable},
		{"not implemented", http.StatusNotImplemented, ErrUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			client, err := NewClient(testConnection(srv.URL+"/dav"), "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Stat(context.Background(), "/媒体"); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestClientOpenRangePartialAndFallback(t *testing.T) {
	body := []byte("0123456789abcdefghij")
	var fullFallback bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "bytes=10-14"
		if r.Header.Get("Range") != want {
			t.Errorf("Range = %q, want %q", r.Header.Get("Range"), want)
		}
		if fullFallback {
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			_, _ = w.Write(body)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 10-14/%d", len(body)))
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[10:15])
	}))
	defer srv.Close()
	client, err := NewClient(testConnection(srv.URL+"/dav"), "")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.OpenRange(context.Background(), "/媒体/a.mp4", 10, 5)
	if err != nil {
		t.Fatalf("open range: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || string(got) != "abcde" {
		t.Errorf("partial response = %d %q", resp.StatusCode, got)
	}
	if resp.TotalSize != int64(len(body)) || resp.ContentRange == "" {
		t.Errorf("range metadata = %+v", resp)
	}

	fullFallback = true
	resp, err = client.OpenRange(context.Background(), "/媒体/a.mp4", 10, 5)
	if err != nil {
		t.Fatalf("open range fallback: %v", err)
	}
	got, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(got) != len(body) {
		t.Errorf("fallback = %d %q", resp.StatusCode, got)
	}
	if resp.TotalSize != int64(len(body)) {
		t.Errorf("fallback total = %d, want %d", resp.TotalSize, len(body))
	}
}

func TestClientTLSVerificationToggle(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, multistatusPrefix+
			davResponseXML("/媒体", "媒体", "", "", "", true)+
			`</d:multistatus>`)
	}))
	defer srv.Close()

	conn := testConnection(srv.URL)
	conn.RootPath = "/"
	strict, err := NewClient(conn, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := strict.Test(context.Background()); !errors.Is(err, ErrUnreachable) {
		t.Errorf("strict TLS err = %v, want ErrUnreachable", err)
	}

	conn.VerifyTLS = false
	relaxed, err := NewClient(conn, "")
	if err != nil {
		t.Fatal(err)
	}
	latency, err := relaxed.Test(context.Background())
	if err != nil {
		t.Fatalf("relaxed TLS test: %v", err)
	}
	if latency <= 0 || latency > 5*time.Second {
		t.Errorf("latency = %v", latency)
	}
}

func TestNewClientRejectsBadEndpointAndUserinfo(t *testing.T) {
	for _, endpoint := range []string{"", "ftp://host/dav", "https:///dav", "https://user:pass@host/dav"} {
		if _, err := NewClient(Connection{Endpoint: endpoint, RootPath: "/"}, ""); !errors.Is(err, ErrInvalidEndpoint) {
			t.Errorf("endpoint %q err = %v, want ErrInvalidEndpoint", endpoint, err)
		}
	}
	if _, err := NewClient(Connection{Endpoint: "https://host/dav", RootPath: "../etc"}, ""); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("bad root err = %v, want ErrInvalidPath", err)
	}
}

func TestClientRequestURLKeepsEndpointPrefix(t *testing.T) {
	c, err := NewClient(Connection{Endpoint: "https://host:5244/dav", RootPath: "/"}, "")
	if err != nil {
		t.Fatal(err)
	}
	got := c.urlFor("/媒体/a b.mp4")
	if !strings.HasSuffix(got, "/dav/%E5%AA%92%E4%BD%93/a%20b.mp4") {
		t.Errorf("url = %q", got)
	}
}

func TestClientHeadAndRangeNotSatisfiable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", "4096")
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Last-Modified", "Wed, 01 Jan 2025 02:03:04 GMT")
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()
	client, err := NewClient(testConnection(srv.URL+"/dav"), "")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := client.Head(context.Background(), "/媒体/a.mp4")
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if entry.Size != 4096 || entry.Name != "a.mp4" || entry.Modified == 0 {
		t.Errorf("head entry = %+v", entry)
	}
	if _, err := client.OpenRange(context.Background(), "/媒体/a.mp4", 9999, 10); !errors.Is(err, ErrRangeNotSatisfiable) {
		t.Errorf("range err = %v, want ErrRangeNotSatisfiable", err)
	}
}
