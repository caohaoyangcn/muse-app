package cloud

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"muse-app/backend/streaming"
)

// ServeVideo proxies a remote favorite to an HTTP client (the LAN phone UI).
// It forwards the client's Range and the upstream status/headers; a source that
// ignores Range degrades to a full 200 stream, which native players handle.
func (s *Service) ServeVideo(w http.ResponseWriter, r *http.Request, sourceID int64, remotePath, fileName string) {
	client, conn, err := s.client(sourceID)
	if err != nil {
		writeStreamError(w, err)
		return
	}
	start, length, hasRange, err := parseRangeHeader(r.Header.Get("Range"))
	if err != nil {
		http.Error(w, "range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if !hasRange {
		start = 0
	}
	resp, err := client.OpenRange(r.Context(), remotePath, start, length)
	if err == nil {
		s.observe(conn, nil)
	} else {
		s.observe(conn, err)
		writeStreamError(w, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	contentType := resp.ContentType
	if contentType == "" {
		contentType = streaming.ContentTypeFor(fileName)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "no-store")

	if !hasRange || resp.StatusCode != http.StatusPartialContent {
		if resp.ContentLength > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	w.Header().Set("Content-Range", resp.ContentRange)
	if resp.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	w.WriteHeader(http.StatusPartialContent)
	_, _ = io.Copy(w, resp.Body)
}

func writeStreamError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "网盘文件不存在", http.StatusNotFound)
	case errors.Is(err, ErrUnauthorized):
		http.Error(w, "网盘认证失败", http.StatusBadGateway)
	case errors.Is(err, ErrUnreachable):
		http.Error(w, "网盘未连接", http.StatusServiceUnavailable)
	case errors.Is(err, ErrConnectionNotFound):
		http.Error(w, "网盘连接不存在", http.StatusServiceUnavailable)
	case errors.Is(err, ErrRangeNotSatisfiable):
		http.Error(w, "远程文件已变化", http.StatusRequestedRangeNotSatisfiable)
	default:
		http.Error(w, "网盘视频不可用", http.StatusBadGateway)
	}
}
