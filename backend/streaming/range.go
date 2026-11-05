// Package streaming serves original media files over HTTP with Range support
// for native players.
package streaming

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ServeFile streams a file with HTTP Range support (RFC 7233), allowing native
// players to seek without loading the whole file first.
//
// The file is opened read-only and never modified, honouring the app's
// "Favorite ≠ File" rule.
func ServeFile(w http.ResponseWriter, r *http.Request, absPath, contentType string) {
	f, err := os.Open(absPath)
	if err != nil {
		http.Error(w, "文件缺失", http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		http.Error(w, "读取文件信息失败", http.StatusInternalServerError)
		return
	}
	size := st.Size()

	// CORS: allow requests from the SPA origin (same server) and mobile clients.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Accept-Ranges", "bytes")
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Content-Disposition", "inline")

	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		start, end, ok := parseRange(rangeHeader, size)
		if !ok {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
			http.Error(w, "Range 无法满足", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(http.StatusPartialContent)
		_, err = io.CopyN(w, io.NewSectionReader(f, start, end-start+1), end-start+1)
		if err != nil {
			// Client closed mid-stream (e.g. seeking away). Nothing to report.
			return
		}
		return
	}

	// Whole-file response.
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

// parseRange parses a single "bytes=a-b" header against totalSize and returns
// an inclusive [start,end] or ok=false for an unsatisfiable request. It handles
// open-ended ("bytes=a-") and suffix ("bytes=-n") forms.
func parseRange(spec string, totalSize int64) (start, end int64, ok bool) {
	if totalSize <= 0 {
		return 0, 0, false
	}
	spec = strings.TrimPrefix(spec, "bytes=")
	// Only support a single range.
	if strings.Contains(spec, ",") {
		return 0, 0, false
	}
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return 0, 0, false
	}
	startStr := strings.TrimSpace(spec[:dash])
	endStr := strings.TrimSpace(spec[dash+1:])

	if startStr == "" {
		// Suffix range: last n bytes.
		n, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > totalSize {
			n = totalSize
		}
		return totalSize - n, totalSize - 1, true
	}

	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false
	}
	if start >= totalSize {
		return 0, 0, false
	}
	if endStr == "" {
		return start, totalSize - 1, true
	}
	end, err = strconv.ParseInt(endStr, 10, 64)
	if err != nil || end < start {
		return 0, 0, false
	}
	if end >= totalSize {
		end = totalSize - 1
	}
	return start, end, true
}

// ContentTypeFor maps a file name to a media type for native players. Unknown
// formats fall back to application/octet-stream.
func ContentTypeFor(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".m4v", ".mov":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".ogv":
		return "video/ogg"
	case ".mpg", ".mpeg":
		return "video/mpeg"
	case ".3g2", ".3gp":
		return "video/3gpp"
	default:
		return "application/octet-stream"
	}
}
