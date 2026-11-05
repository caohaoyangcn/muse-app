package cloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"muse-app/backend/streaming"
)

// MediaInfo is what the proxy needs to know about a local favorite row. It is
// deliberately tiny: the proxy must not import the video service (which imports
// this package's hooks indirectly), so main wires a lookup callback.
type MediaInfo struct {
	SourceID int64
	Path     string
	FileName string
	Size     int64
}

// MediaProxy is a loopback-only, read-only HTTP proxy that exposes a remote
// favorite as a URL ffprobe/ffmpeg can open. It is how remote metadata and
// covers are produced without putting WebDAV credentials on a command line.
//
// The URL is unguessable (a random per-process token) and only GET/HEAD are
// accepted. The proxy never caches a whole file: it forwards Range and copies
// bytes as they arrive.
type MediaProxy struct {
	lookup func(int64) (MediaInfo, error)
	open   func(context.Context, int64, string, int64, int64) (*RangeResponse, error)
	stat   func(context.Context, int64, string) (*Entry, error)
	token  string
	log    *slog.Logger

	mu       sync.RWMutex
	base     string
	server   *http.Server
	ln       net.Listener
	fallback sync.Once
}

// NewMediaProxy builds the proxy. Call Start before resolving any URL.
func NewMediaProxy(
	lookup func(int64) (MediaInfo, error),
	open func(context.Context, int64, string, int64, int64) (*RangeResponse, error),
	stat func(context.Context, int64, string) (*Entry, error),
	log *slog.Logger,
) (*MediaProxy, error) {
	if lookup == nil || open == nil || stat == nil {
		return nil, errors.New("media proxy requires lookup, open and stat callbacks")
	}
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate media proxy token: %w", err)
	}
	return &MediaProxy{
		lookup: lookup,
		open:   open,
		stat:   stat,
		token:  hex.EncodeToString(tokenBytes),
		log:    log,
	}, nil
}

// Start binds 127.0.0.1 on a random free port. It never listens on a LAN
// interface: the URL carries no authentication beyond the token and exists only
// for the local media tools.
func (p *MediaProxy) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.server != nil {
		return nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start media proxy: %w", err)
	}
	p.ln = ln
	p.base = "http://" + ln.Addr().String()
	p.server = &http.Server{
		Handler:           http.HandlerFunc(p.handle),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := p.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && p.log != nil {
			p.log.Warn("media proxy stopped", "err", err)
		}
	}()
	if p.log != nil {
		p.log.Info("media proxy started", "addr", ln.Addr().String())
	}
	return nil
}

// Close shuts the proxy down. It is safe to call more than once.
func (p *MediaProxy) Close() error {
	p.mu.Lock()
	srv, ln := p.server, p.ln
	p.server, p.ln = nil, nil
	p.base = ""
	p.mu.Unlock()
	if srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := srv.Shutdown(ctx)
	_ = ln.Close()
	return err
}

// URL returns the loopback URL for a remote favorite, or "" before Start.
func (p *MediaProxy) URL(videoID int64) string {
	p.mu.RLock()
	base := p.base
	p.mu.RUnlock()
	if base == "" {
		return ""
	}
	return base + "/media/" + p.token + "/" + strconv.FormatInt(videoID, 10)
}

func (p *MediaProxy) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/media/"), "/")
	if len(parts) != 2 || parts[0] != p.token {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	media, err := p.lookup(id)
	if err != nil || media.SourceID <= 0 || media.Path == "" {
		http.NotFound(w, r)
		return
	}

	contentType := streaming.ContentTypeFor(media.FileName)
	w.Header().Set("Cache-Control", "no-store")

	if r.Method == http.MethodHead {
		entry, err := p.stat(r.Context(), media.SourceID, media.Path)
		if err != nil {
			p.writeError(w, err)
			return
		}
		if entry.IsDir {
			http.NotFound(w, r)
			return
		}
		if entry.ContentType != "" {
			contentType = entry.ContentType
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Accept-Ranges", "bytes")
		if entry.Size > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(entry.Size, 10))
		}
		if entry.Modified > 0 {
			w.Header().Set("Last-Modified", time.UnixMilli(entry.Modified).UTC().Format(http.TimeFormat))
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	rangeHeader := r.Header.Get("Range")
	start, length, hasRange, err := parseRangeHeader(rangeHeader)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", media.Size))
		http.Error(w, "range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	rangeStart := start
	if !hasRange {
		rangeStart = 0
	}
	resp, err := p.open(r.Context(), media.SourceID, media.Path, rangeStart, length)
	if err != nil {
		p.writeError(w, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	ct := resp.ContentType
	if ct == "" {
		ct = contentType
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Accept-Ranges", "bytes")

	if !hasRange || resp.StatusCode != http.StatusPartialContent {
		if hasRange && resp.StatusCode == http.StatusOK {
			// The server ignored Range. Returning the full body is an allowed
			// HTTP answer and keeps playback working; caching/seek will simply
			// restart from zero. Log once so a broken OpenList deployment is
			// visible without flooding the log.
			p.fallback.Do(func() {
				if p.log != nil {
					p.log.Warn("cloud source ignored Range; media falls back to a full stream")
				}
			})
		}
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

func (p *MediaProxy) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "remote file not found", http.StatusNotFound)
	case errors.Is(err, ErrUnauthorized):
		http.Error(w, "cloud authentication failed", http.StatusBadGateway)
	case errors.Is(err, ErrUnreachable):
		http.Error(w, "cloud source unreachable", http.StatusServiceUnavailable)
	case errors.Is(err, ErrRangeNotSatisfiable):
		http.Error(w, "range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
	default:
		http.Error(w, "cloud media unavailable", http.StatusBadGateway)
	}
}

// parseRangeHeader accepts the single-range form a media player sends:
// bytes=start-end or bytes=start-. A suffix range (bytes=-N) needs the full
// size and is not worth implementing for players that always send a start.
// A length of 0 with hasRange means "from start to EOF".
func parseRangeHeader(raw string) (start, length int64, hasRange bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, false, nil
	}
	if !strings.HasPrefix(raw, "bytes=") {
		return 0, 0, true, errors.New("unsupported range unit")
	}
	spec := strings.TrimPrefix(raw, "bytes=")
	if strings.Contains(spec, ",") {
		return 0, 0, true, errors.New("multiple ranges are not supported")
	}
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return 0, 0, true, errors.New("malformed range")
	}
	startRaw, endRaw := spec[:dash], spec[dash+1:]
	if startRaw == "" {
		return 0, 0, true, errors.New("suffix range is not supported")
	}
	start, err = strconv.ParseInt(startRaw, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, true, errors.New("invalid range start")
	}
	if endRaw == "" {
		return start, 0, true, nil
	}
	end, err := strconv.ParseInt(endRaw, 10, 64)
	if err != nil || end < start {
		return 0, 0, true, errors.New("invalid range end")
	}
	return start, end - start + 1, true, nil
}
