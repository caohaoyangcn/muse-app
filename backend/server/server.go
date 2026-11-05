package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"muse-app/backend/api"
	"muse-app/backend/group"
	"muse-app/backend/jav"
	"muse-app/backend/platform"
	"muse-app/backend/streaming"
	"muse-app/backend/video"
)

// Server is a LAN HTTP server that exposes the favorite library to browsers
// (typically a phone on the same network). It serves a mobile web UI and a
// small JSON API; video files are streamed with HTTP Range support so a native
// player can seek without server-side decoding. All business logic (reads + writes)
// goes through the shared api.Library façade the desktop app uses too.
type Server struct {
	lib     *api.Library
	dataDir string
	log     *slog.Logger

	mu       sync.Mutex
	httpSrv  *http.Server
	listener net.Listener
	addr     string // bound host:port, e.g. 0.0.0.0:8090
	listen   func(network, address string) (net.Listener, error)
	firewall func(context.Context, int) error
}

// StartInfo is returned by Start/Status for the frontend to display the
// access URL and listening state.
type StartInfo struct {
	Running bool     `json:"running"`
	URLs    []string `json:"urls"`  // LAN-accessible base URLs (http://<lan-ip>:<port>)
	Local   string   `json:"local"` // localhost base URL
	Port    int      `json:"port"`
	Error   string   `json:"error,omitempty"`
}

// New creates a Server bound to the shared business façade. dataDir is used
// only to resolve the thumbnail directory (a small reuse of the app's layout).
func New(lib *api.Library, dataDir string, log *slog.Logger) *Server {
	return &Server{lib: lib, dataDir: dataDir, log: log, listen: net.Listen, firewall: platform.EnsureInboundRuleContext}
}

// Start binds the HTTP server on 0.0.0.0:port so devices on the same network
// can reach it. It returns the effective access URLs once the listener is up.
// If port <= 0, the OS assigns a free port.
func (s *Server) Start(port int) (StartInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpSrv != nil {
		return s.infoLocked(), nil // already running
	}

	listen := s.listen
	if listen == nil {
		listen = net.Listen
	}
	ln, err := listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return StartInfo{}, fmt.Errorf("无法监听端口 %d: %w", port, err)
	}

	s.listener = ln
	s.addr = ln.Addr().String()
	s.httpSrv = &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 15 * time.Second,
		// No global WriteTimeout: video streams can last minutes (per-request
		// streaming has no idle timeout thanks to the Range handler + Go's
		// http.TimeoutHandler being disabled here on purpose).
	}
	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("web server serve failed", "err", err)
		}
	}()

	// Open the port in Windows Firewall so a phone on the LAN can reach it.
	// This is deliberately asynchronous: firewall tooling may invoke UAC or be
	// blocked by policy, neither of which should prevent the HTTP listener from
	// becoming available or make the UI appear stuck on "starting".
	if _, actualPort, err := net.SplitHostPort(s.listener.Addr().String()); err == nil {
		if p, err := strconv.Atoi(actualPort); err == nil {
			s.configureFirewall(p)
		}
	}

	info := s.infoLocked()
	s.log.Info("web server started", "local", info.Local, "urls", info.URLs)
	return info, nil
}

func (s *Server) configureFirewall(port int) {
	if s.firewall == nil {
		return
	}
	firewall := s.firewall
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if fwErr := firewall(ctx, port); fwErr != nil && s.log != nil {
			s.log.Warn("ensure firewall rule failed (may block LAN access)",
				"err", fwErr, "port", port)
		}
	}()
}

// Stop shuts down the HTTP server and releases the port.
func (s *Server) Stop() error {
	s.mu.Lock()
	srv := s.httpSrv
	ln := s.listener
	s.httpSrv = nil
	s.listener = nil
	s.mu.Unlock()

	if srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := srv.Shutdown(ctx)
	// Shutdown closes the listener too; ignore the redundant close on ln.
	_ = ln.Close()
	if err != nil {
		s.log.Warn("web server shutdown", "err", err)
		return err
	}
	s.log.Info("web server stopped")
	return nil
}

// Status returns the current state without altering it.
func (s *Server) Status() StartInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.infoLocked()
}

func (s *Server) infoLocked() StartInfo {
	info := StartInfo{}
	if s.httpSrv == nil {
		info.Error = "not running"
		return info
	}
	_, portStr, err := net.SplitHostPort(s.addr)
	port := 0
	if err == nil {
		port, _ = strconv.Atoi(portStr)
	}
	local := "localhost"
	if li := LanIPv4(); li != "" {
		local = fmt.Sprintf("http://%s:%d", li, port)
	}
	urls := make([]string, 0, 2)
	urls = append(urls, fmt.Sprintf("http://localhost:%d", port))
	for _, ip := range LanIPv4s() {
		urls = append(urls, fmt.Sprintf("http://%s:%d", ip, port))
	}
	info.Running = true
	info.URLs = urls
	info.Local = local
	info.Port = port
	return info
}

// routes wires all HTTP endpoints. The SPA lives under "/" and the mobile app
// requests JSON/streams via the /api and /stream prefixes. Every endpoint is a
// thin adapter over the shared api.Library façade so the desktop and web
// transports cannot drift; the LAN API is intentionally unauthenticated (see
// TODO.md), so the write endpoints below are reachable by any device on the
// same network.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Reads.
	mux.HandleFunc("GET /api/videos", s.handleListVideos)
	mux.HandleFunc("GET /api/trash", s.handleListTrash)
	mux.HandleFunc("DELETE /api/trash", s.handleEmptyTrash)
	mux.HandleFunc("POST /api/trash/{id}/restore", s.handleRestoreTrash)
	mux.HandleFunc("DELETE /api/trash/{id}", s.handleDeleteTrash)
	mux.HandleFunc("GET /api/videos/{id}", s.handleGetVideo)
	mux.HandleFunc("GET /api/videos/{id}/jav", s.handleVideoJav)
	mux.HandleFunc("GET /api/videos/{id}/jav/task", s.handleVideoJavTask)
	mux.HandleFunc("GET /api/groups", s.handleListGroups)
	mux.HandleFunc("GET /api/groups/{id}/videos", s.handleVideosInGroup)
	mux.HandleFunc("GET /api/jav/tasks", s.handleJavTasks)
	mux.HandleFunc("GET /api/jav/duplicates", s.handleJavDuplicates)
	mux.HandleFunc("GET /api/duplicates/content", s.handleContentDuplicates)
	mux.HandleFunc("GET /api/duplicates/content/scan", s.handleContentScanStatus)
	mux.HandleFunc("POST /api/duplicates/content/scan", s.handleStartContentScan)
	mux.HandleFunc("POST /api/duplicates/content/ignore", s.handleIgnoreContentDuplicate)
	mux.HandleFunc("POST /api/duplicates/content/reverify", s.handleReverifyContentDuplicate)
	mux.HandleFunc("POST /api/duplicates/content/confirm", s.handleConfirmContentDuplicate)
	mux.HandleFunc("GET /api/actors", s.handleListActors)
	mux.HandleFunc("GET /api/actors/videos", s.handleVideosByActor)

	// Writes (API reachable; no Web front-end control).
	mux.HandleFunc("POST /api/videos", s.handleAddVideos)
	mux.HandleFunc("DELETE /api/videos/{id}", s.handleRemoveVideo)
	mux.HandleFunc("PUT /api/videos/{id}/info", s.handleUpdateVideoInfo)
	mux.HandleFunc("POST /api/videos/{id}/likes", s.handleLikeVideo)
	mux.HandleFunc("DELETE /api/videos/{id}/likes", s.handleClearVideoLikes)
	mux.HandleFunc("POST /api/groups", s.handleCreateGroup)
	mux.HandleFunc("PUT /api/groups/{id}", s.handleRenameGroup)
	mux.HandleFunc("DELETE /api/groups/{id}", s.handleDeleteGroup)
	mux.HandleFunc("POST /api/groups/{id}/videos", s.handleAddVideoToGroup)
	mux.HandleFunc("POST /api/jav/duplicates/ignore", s.handleIgnoreJavDuplicate)
	// Maintenance: delete generated images no favorite refers to any more.
	mux.HandleFunc("POST /api/maintenance/orphaned-images", s.handleSweepImages)
	// Maintenance: re-answer "is the file there?" for every favorite (文件检测).
	mux.HandleFunc("GET /api/maintenance/files/scan", s.handleFileScanStatus)
	mux.HandleFunc("POST /api/maintenance/files/scan", s.handleStartFileScan)
	// Maintenance: permanently delete recycle-bin entries past their retention.
	mux.HandleFunc("POST /api/maintenance/trash/purge", s.handlePurgeExpiredTrash)

	// Media.
	mux.HandleFunc("GET /stream/video/{id}", s.handleStreamVideo)
	mux.HandleFunc("GET /thumbnails/{id}", s.handleThumbnail)
	mux.HandleFunc("GET /covers/jav/{id}", s.handleJavCover)
	mux.HandleFunc("GET /avatars/{name}", s.handleActorAvatar)
	// Reserved API prefixes must not fall through to the SPA index.
	mux.HandleFunc("/api/", http.NotFound)
	mux.HandleFunc("/stream/", http.NotFound)

	// Index + static web UI.
	mux.HandleFunc("/", s.handleStatic)

	return logRequests(s.log, mux)
}

func (s *Server) handleListVideos(w http.ResponseWriter, r *http.Request) {
	kw := strings.TrimSpace(r.URL.Query().Get("search"))
	var (
		list []video.Video
		err  error
	)
	if kw != "" {
		list, err = s.lib.SearchVideos(kw)
	} else {
		list, err = s.lib.ListVideos()
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, list)
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	list, err := s.lib.ListGroups()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, list)
}

func (s *Server) handleVideosInGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	// Full Video objects (not just ids) so the mobile UI needs only one source
	// of truth; the façade owns this orchestration.
	videos, err := s.lib.VideosInGroup(id)
	if err != nil {
		httpError(w, http.StatusNotFound, "分组不存在")
		return
	}
	writeJSON(w, videos)
}

// handleGetVideo returns a single favorite. The mobile detail screen refreshes
// through this after an edit or a like so it never has to reload the whole
// library.
func (s *Server) handleGetVideo(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	v, err := s.lib.Video(id)
	if err != nil {
		httpError(w, http.StatusNotFound, "视频不存在")
		return
	}
	writeJSON(w, v)
}

// handleVideoJav returns the scraped metadata for a favorite. An unscraped
// favorite yields 200 with a JSON null body — the same "no data yet" meaning
// the desktop client derives from sql.ErrNoRows — so the mobile UI can tell
// "not scraped" apart from "video not found".
func (s *Server) handleVideoJav(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	detail, err := s.lib.VideoJav(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, nil)
			return
		}
		httpError(w, http.StatusInternalServerError, "刮削详情不可用")
		return
	}
	writeJSON(w, detail)
}

// handleVideoJavTask returns the scrape task for a favorite, or JSON null when
// no scrape has ever been queued for it.
func (s *Server) handleVideoJavTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	task, err := s.lib.VideoJavTask(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, nil)
			return
		}
		httpError(w, http.StatusInternalServerError, "刮削任务不可用")
		return
	}
	writeJSON(w, task)
}

// handleJavTasks returns the scrape board: the latest task per favorite,
// newest first.
func (s *Server) handleJavTasks(w http.ResponseWriter, r *http.Request) {
	overviews, err := s.lib.ListJavTaskOverviews()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "刮削任务不可用")
		return
	}
	if overviews == nil {
		overviews = []jav.TaskOverview{}
	}
	writeJSON(w, overviews)
}

// handleJavDuplicates returns the recognized works (番号) the library holds more
// than one copy of — the 查重 worklist. Dismissed groups are hidden unless
// ?includeIgnored=1 asks for them.
func (s *Server) handleJavDuplicates(w http.ResponseWriter, r *http.Request) {
	includeIgnored := r.URL.Query().Get("includeIgnored") == "1"
	groups, err := s.lib.ListJavDuplicates(includeIgnored)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "查重数据不可用")
		return
	}
	if groups == nil {
		groups = []jav.DuplicateGroup{}
	}
	writeJSON(w, groups)
}

// handleIgnoreJavDuplicate keeps every copy in one duplicate group (忽略重复提示).
func (s *Server) handleIgnoreJavDuplicate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	if strings.TrimSpace(body.Key) == "" {
		httpError(w, http.StatusBadRequest, "缺少查重分组标识")
		return
	}
	if err := s.lib.IgnoreJavDuplicate(body.Key); err != nil {
		httpError(w, http.StatusInternalServerError, "忽略查重失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleContentDuplicates returns the favorites that hold byte-identical files —
// the content dedup worklist. It answers a different question than the 番号
// worklist: that one groups one work's encodes, this one groups the same bytes
// stored twice. Dismissed groups are hidden unless ?includeIgnored=1.
func (s *Server) handleContentDuplicates(w http.ResponseWriter, r *http.Request) {
	includeIgnored := r.URL.Query().Get("includeIgnored") == "1"
	groups, err := s.lib.ListContentDuplicates(includeIgnored)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "内容去重数据不可用")
		return
	}
	if groups == nil {
		groups = []video.ContentDuplicateGroup{}
	}
	writeJSON(w, groups)
}

// handleContentScanStatus returns the state of the library-wide fingerprint pass.
func (s *Server) handleContentScanStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.lib.ContentScanStatus())
}

// handleStartContentScan starts (or joins) the library-wide fingerprint pass. It
// returns immediately; clients follow the status endpoint.
func (s *Server) handleStartContentScan(w http.ResponseWriter, r *http.Request) {
	status, err := s.lib.ScanContentDuplicates()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "无法开始内容扫描")
		return
	}
	writeJSON(w, status)
}

// handleIgnoreContentDuplicate keeps every member of one content-duplicate group
// (保留全部).
func (s *Server) handleIgnoreContentDuplicate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Hash string `json:"hash"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	if strings.TrimSpace(body.Hash) == "" {
		httpError(w, http.StatusBadRequest, "缺少内容指纹")
		return
	}
	if err := s.lib.IgnoreContentDuplicate(body.Hash); err != nil {
		httpError(w, http.StatusInternalServerError, "忽略内容重复失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReverifyContentDuplicate re-reads the fingerprints of one group, which is
// how a group whose files changed becomes actionable again.
func (s *Server) handleReverifyContentDuplicate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Hash string `json:"hash"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	if strings.TrimSpace(body.Hash) == "" {
		httpError(w, http.StatusBadRequest, "缺少内容指纹")
		return
	}
	if err := s.lib.ReverifyContentDuplicate(body.Hash); err != nil {
		httpError(w, http.StatusInternalServerError, "重新校验失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleConfirmContentDuplicate starts a full-file confirmation of one group. It
// reads whole multi-gigabyte files, so it always runs in the background and the
// client follows the group's Confirming/Confirmed state.
func (s *Server) handleConfirmContentDuplicate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Hash string `json:"hash"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	if strings.TrimSpace(body.Hash) == "" {
		httpError(w, http.StatusBadRequest, "缺少内容指纹")
		return
	}
	if err := s.lib.ConfirmContentDuplicate(body.Hash); err != nil {
		httpError(w, http.StatusInternalServerError, "无法开始完整校验")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListActors returns the aggregated actor index (演员), including merged
// stage names under their canonical entry.
func (s *Server) handleListActors(w http.ResponseWriter, r *http.Request) {
	actors, err := s.lib.ListActors()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "演员索引不可用")
		return
	}
	if actors == nil {
		actors = []jav.ActorSummary{}
	}
	writeJSON(w, actors)
}

// handleVideosByActor returns the full favorites featuring the actor named by
// the ?name= query parameter. The name travels as a query value rather than a
// path segment because actor names are arbitrary strings (CJK, punctuation,
// even "/").
func (s *Server) handleVideosByActor(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	videos, err := s.lib.VideosByActor(name)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "演员作品不可用")
		return
	}
	if videos == nil {
		videos = []video.Video{}
	}
	writeJSON(w, videos)
}

// handleSweepImages deletes the generated thumbnails and scraped covers that no
// favorite refers to any more and reports what it freed. Exposed over HTTP so a
// headless LAN deployment (and an operator with curl) can run the same
// maintenance the desktop performs at startup.
func (s *Server) handleSweepImages(w http.ResponseWriter, r *http.Request) {
	result, err := s.lib.RemoveOrphanedImages()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "清理孤儿图片失败")
		return
	}
	writeJSON(w, result)
}

// handleFileScanStatus returns the state of the 文件检测 (file-existence) pass.
func (s *Server) handleFileScanStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.lib.FileScanStatus())
}

// handleStartFileScan starts (or joins) the 文件检测 pass. It returns
// immediately; clients follow the status endpoint, exactly like the content
// scan, because a library on mechanical drives takes long enough that holding
// the request open would look like a hang.
func (s *Server) handleStartFileScan(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.lib.ScanFileStatuses())
}

func (s *Server) handleAddVideos(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	if len(body.Paths) == 0 {
		httpError(w, http.StatusBadRequest, "缺少 paths")
		return
	}
	results := s.lib.AddVideos(body.Paths)
	writeJSON(w, results)
}

// handleListTrash returns the recycle bin together with the retention period.
func (s *Server) handleListTrash(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.lib.ListTrash()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "回收站不可用")
		return
	}
	if snapshot.Videos == nil {
		snapshot.Videos = []video.Video{}
	}
	writeJSON(w, snapshot)
}

// handleRestoreTrash brings one trashed favorite back into the library.
func (s *Server) handleRestoreTrash(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.lib.RestoreVideo(id); err != nil {
		httpError(w, http.StatusBadRequest, video.UserMessage(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteTrash permanently deletes one trashed favorite. The source file
// is never touched; only Muse's own record and generated assets are removed.
func (s *Server) handleDeleteTrash(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.lib.DeleteVideoPermanently(id); err != nil {
		httpError(w, http.StatusBadRequest, video.UserMessage(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleEmptyTrash permanently deletes every trashed favorite.
func (s *Server) handleEmptyTrash(w http.ResponseWriter, r *http.Request) {
	count, err := s.lib.EmptyTrash()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "清空回收站失败")
		return
	}
	writeJSON(w, map[string]int{"count": count})
}

// handlePurgeExpiredTrash is the maintenance entry point for the retention
// sweep, so a headless LAN deployment can run the same cleanup as the desktop.
func (s *Server) handlePurgeExpiredTrash(w http.ResponseWriter, r *http.Request) {
	count, err := s.lib.PurgeExpiredTrash()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "清理回收站失败")
		return
	}
	writeJSON(w, map[string]int{"count": count})
}

func (s *Server) handleRemoveVideo(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.lib.RemoveVideo(id); err != nil {
		httpError(w, http.StatusBadRequest, video.UserMessage(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUpdateVideoInfo saves the custom title/notes for a favorite and returns
// the updated record so the detail screen can repaint without a refetch.
func (s *Server) handleUpdateVideoInfo(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	var body struct {
		Title string `json:"title"`
		Notes string `json:"notes"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	v, err := s.lib.UpdateVideoInfo(id, body.Title, body.Notes)
	if err != nil {
		httpError(w, http.StatusBadRequest, video.UserMessage(err))
		return
	}
	writeJSON(w, v)
}

// handleLikeVideo adds one like (repeated likes stack) and returns the updated
// favorite.
func (s *Server) handleLikeVideo(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	v, err := s.lib.LikeVideo(id)
	if err != nil {
		httpError(w, http.StatusBadRequest, video.UserMessage(err))
		return
	}
	writeJSON(w, v)
}

// handleClearVideoLikes resets a favorite's like counter to zero.
func (s *Server) handleClearVideoLikes(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	v, err := s.lib.ClearVideoLikes(id)
	if err != nil {
		httpError(w, http.StatusBadRequest, video.UserMessage(err))
		return
	}
	writeJSON(w, v)
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	g, err := s.lib.CreateGroup(body.Name)
	if err != nil {
		httpError(w, http.StatusBadRequest, group.UserMessage(err))
		return
	}
	writeJSON(w, g)
}

func (s *Server) handleRenameGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	g, err := s.lib.RenameGroup(id, body.Name)
	if err != nil {
		httpError(w, http.StatusBadRequest, group.UserMessage(err))
		return
	}
	writeJSON(w, g)
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.lib.DeleteGroup(id); err != nil {
		httpError(w, http.StatusBadRequest, group.UserMessage(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddVideoToGroup(w http.ResponseWriter, r *http.Request) {
	groupId, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	var body struct {
		VideoID int64 `json:"videoId"`
	}
	if err := decodeJSON(r, &body); err != nil {
		httpError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	if body.VideoID <= 0 {
		httpError(w, http.StatusBadRequest, "无效的 videoId")
		return
	}
	if err := s.lib.AddVideoToGroup(body.VideoID, groupId); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleStreamVideo serves the original file with HTTP Range support so a
// native player can seek without any server-side decoding or remuxing. This is
// the same "Favorite ≠ File" contract: we only read the source, never modify it.
func (s *Server) handleStreamVideo(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	v, err := s.lib.Video(id)
	if err != nil {
		httpError(w, http.StatusNotFound, "视频不存在")
		return
	}
	if v.Status == video.StatusMissing {
		httpError(w, http.StatusNotFound, "文件缺失")
		return
	}
	if v.VolumeOffline {
		httpError(w, http.StatusNotFound, "磁盘未接入")
		return
	}
	if v.IsRemote() {
		// Playback belongs to OpenList; the LAN server never proxies cloud
		// video bytes through Muse.
		httpError(w, http.StatusConflict, "请在 OpenList 中播放该网盘视频")
		return
	}

	streaming.ServeFile(w, r, v.Path, streaming.ContentTypeFor(v.FileName))
}

func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	// The mobile UI addresses thumbnails as /thumbnails/<id>.jpg (mirroring the
	// desktop app), so the path value carries a ".jpg" suffix. Strip it before
	// parsing; the response body is the JPEG regardless of the URL ending.
	raw := strings.TrimSuffix(r.PathValue("id"), ".jpg")
	id, ok := parseID(w, raw)
	if !ok {
		return
	}
	name := fmt.Sprintf("%d.jpg", id)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepathFromData(s.dataDir, "thumbnails", name))
}

func (s *Server) handleJavCover(w http.ResponseWriter, r *http.Request) {
	// The mobile UI uses the stored relative path format
	// "covers/jav/<id>.jpg". Parse only the numeric id and rebuild the path so
	// this endpoint cannot be used to read arbitrary files under dataDir.
	raw := strings.TrimSuffix(r.PathValue("id"), ".jpg")
	id, ok := parseID(w, raw)
	if !ok {
		return
	}
	name := fmt.Sprintf("%d.jpg", id)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepathFromData(s.dataDir, "covers", "jav", name))
}

func (s *Server) handleActorAvatar(w http.ResponseWriter, r *http.Request) {
	// Actors are addressed by the provider's performer id, not by name: the NAT
	// side of the app never needs to know a path. The pattern keeps the value to
	// what a downloaded avatar file can be called, so the handler cannot be used
	// to read arbitrary files under dataDir.
	name := r.PathValue("name")
	if !validActorAvatarName(name) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepathFromData(s.dataDir, "avatars", name))
}

// validActorAvatarName accepts exactly the files the scraper writes:
// <performer id>.jpg, where the id is the provider's lowercase alphanumeric
// token. Rejecting everything else keeps the handler from serving a path the
// scrape never produced.
func validActorAvatarName(name string) bool {
	base, ok := strings.CutSuffix(name, ".jpg")
	if !ok || base == "" || len(base) > 64 {
		return false
	}
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	serveWebUI(w, r)
}

func parseID(w http.ResponseWriter, raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		httpError(w, http.StatusBadRequest, "无效的 ID")
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// decodeJSON reads a request body into v, rejecting unknown trailing content.
func decodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	return json.NewDecoder(r.Body).Decode(v)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
