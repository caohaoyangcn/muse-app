package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"muse-app/backend/api"
	"muse-app/backend/cloud"
	"muse-app/backend/group"
	"muse-app/backend/jav"
	"muse-app/backend/netproxy"
	"muse-app/backend/platform"
	"muse-app/backend/server"
	"muse-app/backend/settings"
	"muse-app/backend/video"
	"muse-app/backend/watch"
)

type JavBusCookieStatus struct {
	Configured bool `json:"configured"`
}

type FC2CookieStatus struct {
	Configured bool `json:"configured"`
}

type javCookieProvider interface {
	SetCookie(string)
	CookieConfigured() bool
}

// App binds the Wails-facing API. All business logic (reads + writes) is
// delegated to the api.Library façade, which the LAN HTTP server shares, so the
// two transports can never drift. Platform-only concerns (open in the system file manager,
// clipboard, file picker, server start/stop) stay here.
type App struct {
	ctx atomic.Pointer[context.Context]
	lib *api.Library
	// videos is retained only for Wails-platform operations that are not part of
	// the shared business façade (open in system player, reveal in the file manager,
	// status refresh, metadata backfill). Shared reads/writes go through lib.
	videos            *video.Service
	server            *server.Server
	settings          *settings.Store
	javCookieProvider javCookieProvider
	fc2CookieProvider javCookieProvider
	proxy             *netproxy.Config
	dataDir           string
	log               *slog.Logger
}

// SetServer wires the LAN server (see SetLibrary).
func (a *App) SetServer(srv *server.Server) { a.server = srv }

// SetSettings wires the persisted-preference store (LAN server port, …).
func (a *App) SetSettings(s *settings.Store) { a.settings = s }

// setJavBusCookieProvider wires the runtime provider so settings changes take
// effect immediately without restarting the scrape worker.
func (a *App) setJavBusCookieProvider(provider javCookieProvider) { a.javCookieProvider = provider }

func (a *App) setFC2CookieProvider(provider javCookieProvider) { a.fc2CookieProvider = provider }

// GetJavBusCookieConfigured reports only whether the active provider has a
// Cookie. It never returns the Cookie value to the frontend.
func (a *App) GetJavBusCookieStatus() (JavBusCookieStatus, error) {
	configured := false
	if a.javCookieProvider != nil {
		configured = a.javCookieProvider.CookieConfigured()
	} else {
		if a.settings == nil {
			return JavBusCookieStatus{}, fmt.Errorf("设置存储未初始化")
		}
		var err error
		configured, err = a.settings.HasJavBusCookie()
		if err != nil {
			return JavBusCookieStatus{}, err
		}
	}
	return JavBusCookieStatus{Configured: configured}, nil
}

// SetJavBusCookie validates, securely persists, and activates a JavBus Cookie.
func (a *App) SetJavBusCookie(cookie string) error {
	if a.settings == nil {
		return fmt.Errorf("设置存储未初始化")
	}
	if err := a.settings.SetJavBusCookie(cookie); err != nil {
		return err
	}
	if a.javCookieProvider != nil {
		stored, err := a.settings.GetJavBusCookie()
		if err != nil {
			return err
		}
		a.javCookieProvider.SetCookie(stored)
	}
	return nil
}

// ClearJavBusCookie removes the persisted Cookie and clears the active provider.
func (a *App) ClearJavBusCookie() error {
	if a.settings == nil {
		return fmt.Errorf("设置存储未初始化")
	}
	if err := a.settings.ClearJavBusCookie(); err != nil {
		return err
	}
	if a.javCookieProvider != nil {
		a.javCookieProvider.SetCookie("")
	}
	return nil
}

// GetFC2CookieStatus reports only whether an FC2CMADB Cookie is configured.
func (a *App) GetFC2CookieStatus() (FC2CookieStatus, error) {
	configured := false
	if a.fc2CookieProvider != nil {
		configured = a.fc2CookieProvider.CookieConfigured()
	} else {
		if a.settings == nil {
			return FC2CookieStatus{}, fmt.Errorf("设置存储未初始化")
		}
		var err error
		configured, err = a.settings.HasFC2Cookie()
		if err != nil {
			return FC2CookieStatus{}, err
		}
	}
	return FC2CookieStatus{Configured: configured}, nil
}

// SetFC2Cookie validates, securely persists, and activates an FC2CMADB Cookie.
func (a *App) SetFC2Cookie(cookie string) error {
	if a.settings == nil {
		return fmt.Errorf("设置存储未初始化")
	}
	if err := a.settings.SetFC2Cookie(cookie); err != nil {
		return err
	}
	if a.fc2CookieProvider != nil {
		stored, err := a.settings.GetFC2Cookie()
		if err != nil {
			return err
		}
		a.fc2CookieProvider.SetCookie(stored)
	}
	return nil
}

// ClearFC2Cookie removes the persisted Cookie and clears the active provider.
func (a *App) ClearFC2Cookie() error {
	if a.settings == nil {
		return fmt.Errorf("设置存储未初始化")
	}
	if err := a.settings.ClearFC2Cookie(); err != nil {
		return err
	}
	if a.fc2CookieProvider != nil {
		a.fc2CookieProvider.SetCookie("")
	}
	return nil
}

// setScrapeProxyConfig wires the runtime proxy resolver. Scraping transports
// read it on every request, so a settings change needs no provider restart.
func (a *App) setScrapeProxyConfig(proxy *netproxy.Config) { a.proxy = proxy }

// GetScrapeProxy returns the stored scraping proxy address, or an empty string
// when scraping dials directly (HTTP_PROXY/HTTPS_PROXY still apply then).
func (a *App) GetScrapeProxy() (string, error) {
	if a.settings == nil {
		return "", fmt.Errorf("设置存储未初始化")
	}
	return a.settings.ScrapeProxy()
}

// SetScrapeProxy validates, persists, and applies the scraping proxy address.
// An empty address clears the setting. The backend rejects malformed addresses
// with a user-facing Chinese message.
func (a *App) SetScrapeProxy(raw string) error {
	if a.settings == nil {
		return fmt.Errorf("设置存储未初始化")
	}
	parsed, err := netproxy.Parse(raw)
	if err != nil {
		return err
	}
	if err := a.settings.SetScrapeProxy(raw); err != nil {
		return err
	}
	if a.proxy != nil {
		a.proxy.Set(parsed)
	}
	return nil
}

func NewApp(dataDir string, log *slog.Logger) *App {
	return &App{dataDir: dataDir, log: log}
}

// SetLibrary wires the shared business façade. Annotation: this must be called
// after the platform services are wired; the Server factory also receives the
// same façade.
func (a *App) SetLibrary(lib *api.Library) { a.lib = lib }

// SetService wires the video service for Wails-platform operations (open in
// default player, reveal in the file manager, status/metadata tasks).
func (a *App) SetService(svc *video.Service) { a.videos = svc }

// EventContext returns the live Wails context once startup has run, or nil.
func (a *App) EventContext() context.Context {
	if p := a.ctx.Load(); p != nil {
		return *p
	}
	return nil
}

// Emit forwards a backend event to the frontend; no-op before startup.
func (a *App) Emit(name string, data ...any) {
	if ctx := a.ctx.Load(); ctx != nil {
		runtime.EventsEmit(*ctx, name, data...)
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx.Store(&ctx)
	// Re-check every favorite's file existence in the background (spec-v1.md §20).
	go a.videos.RefreshAllStatuses()
	// Re-process favorites missing metadata or an automatic cover. Run on every
	// startup: it is idempotent and self-healing, so a transient media-read
	// failure does not permanently leave a favorite without its cover.
	go func() {
		a.videos.BackfillProcessing()
	}()
	// Drag & drop listeners live on the frontend (Library.tsx calls the JS
	// runtime OnFileDrop, which installs the window handlers that stop the
	// webview from navigating to the dropped file). The JS callback then
	// calls the AddVideos binding, so there is a single handling path.
}

// ListVideos returns all favorites, most recently added first.
func (a *App) ListVideos() ([]video.Video, error) {
	return a.lib.ListVideos()
}

// SearchVideos filters favorites by file name or path.
func (a *App) SearchVideos(keyword string) ([]video.Video, error) {
	return a.lib.SearchVideos(keyword)
}

// AddVideo adds a single favorite via the file picker.
func (a *App) AddVideo(path string) (*video.Video, error) {
	return a.lib.AddVideo(path)
}

// AddVideos adds a batch of paths, one independent result per file.
func (a *App) AddVideos(paths []string) []video.AddVideoResult {
	return a.lib.AddVideos(paths)
}

// RemoveVideo deletes only the favorite record and its thumbnail, and clears
// any group memberships it had (orchestrated in the shared façade).
func (a *App) RemoveVideo(id int64) error {
	return a.lib.RemoveVideo(id)
}

// UpdateVideoInfo saves a custom title/notes for a favorite (编辑影片信息).
func (a *App) UpdateVideoInfo(id int64, title, notes string) (*video.Video, error) {
	return a.lib.UpdateVideoInfo(id, title, notes)
}

// SetVideoCover installs the frame at atSec (seconds from the start) as the
// favorite's custom cover, extracted with ffmpeg.
func (a *App) SetVideoCover(id int64, atSec float64) (*video.Video, error) {
	return a.lib.SetVideoCover(id, atSec)
}

// ResetVideoCover reverts the cover to the automatic thumbnail.
func (a *App) ResetVideoCover(id int64) (*video.Video, error) {
	return a.lib.ResetVideoCover(id)
}

// SetVideoScrapedCoverAllowed configures whether a scraped cover may replace
// this video's current cover. The preference is stored per video.
func (a *App) SetVideoScrapedCoverAllowed(id int64, allowed bool) (*video.Video, error) {
	return a.lib.SetVideoScrapedCoverAllowed(id, allowed)
}

func (a *App) VideoJav(videoID int64) (*jav.Detail, error)   { return a.lib.VideoJav(videoID) }
func (a *App) VideoJavTask(videoID int64) (*jav.Task, error) { return a.lib.VideoJavTask(videoID) }
func (a *App) ListJavTaskOverviews() ([]jav.TaskOverview, error) {
	return a.lib.ListJavTaskOverviews()
}

// ListActors returns the aggregated cast index (演员词条 + 出演影片数).
func (a *App) ListActors() ([]jav.ActorSummary, error) { return a.lib.ListActors() }

// VideosByActor returns the favorites an actor appears in.
func (a *App) VideosByActor(name string) ([]video.Video, error) {
	return a.lib.VideosByActor(name)
}

// MergeActors merges two scraped stage names into one actor (演员合并).
func (a *App) MergeActors(source, target string) error {
	return a.lib.MergeActors(source, target)
}

// UnmergeActor splits a merged stage name back out (解除合并).
func (a *App) UnmergeActor(alias string) error { return a.lib.UnmergeActor(alias) }

// ActorProfile returns one actor's scraped profile (头像 / 生日 / 三围 …).
func (a *App) ActorProfile(name string) (*jav.ActorProfile, error) {
	return a.lib.ActorProfile(name)
}

// StartActorFill queues a profile backfill for the actors that have none yet
// (补全演员信息) and returns how many names were newly queued.
func (a *App) StartActorFill() (int, error) { return a.lib.StartActorFill() }

// ActorFillStatus reports the background profile backfill's progress.
func (a *App) ActorFillStatus() (jav.ActorFillStatus, error) {
	return a.lib.ActorFillStatus()
}

// ListJavDuplicates returns the recognized works (番号) the library holds more
// than one copy of — the 查重 worklist.
func (a *App) ListJavDuplicates(includeIgnored bool) ([]jav.DuplicateGroup, error) {
	return a.lib.ListJavDuplicates(includeIgnored)
}

// IgnoreJavDuplicate keeps every copy in a duplicate group (忽略重复提示).
func (a *App) IgnoreJavDuplicate(key string) error { return a.lib.IgnoreJavDuplicate(key) }

// ListContentDuplicates returns the favorites that hold byte-identical files —
// the content dedup worklist (内容重复).
func (a *App) ListContentDuplicates(includeIgnored bool) ([]video.ContentDuplicateGroup, error) {
	return a.lib.ListContentDuplicates(includeIgnored)
}

// IgnoreContentDuplicate keeps every member of one content-duplicate group
// (保留全部).
func (a *App) IgnoreContentDuplicate(hash string) error {
	return a.lib.IgnoreContentDuplicate(hash)
}

// ReverifyContentDuplicate re-reads the fingerprints of one content-duplicate
// group.
func (a *App) ReverifyContentDuplicate(hash string) error {
	return a.lib.ReverifyContentDuplicate(hash)
}

// ConfirmContentDuplicate reads a content-duplicate group's files in full so the
// verdict is byte-for-byte before the user acts on it.
func (a *App) ConfirmContentDuplicate(hash string) error {
	return a.lib.ConfirmContentDuplicate(hash)
}

// ScanContentDuplicates starts the library-wide fingerprint pass.
func (a *App) ScanContentDuplicates() (video.ContentScanStatus, error) {
	return a.lib.ScanContentDuplicates()
}

// ContentScanStatus returns the current or last fingerprint pass.
func (a *App) ContentScanStatus() video.ContentScanStatus { return a.lib.ContentScanStatus() }

func (a *App) ScrapeVideoJav(videoID int64, force bool) error {
	return a.lib.ScrapeVideoJav(videoID, force)
}
func (a *App) ScrapeVideosJav(force bool) (int, error) {
	return a.lib.ScrapeVideosJav(force)
}
func (a *App) SetVideoJavOverride(videoID int64, mode, code string) error {
	return a.lib.SetVideoJavOverride(videoID, mode, code)
}

// LikeVideo adds one like to a favorite's counter (repeated likes stack).
func (a *App) LikeVideo(id int64) (*video.Video, error) {
	return a.lib.LikeVideo(id)
}

// ClearVideoLikes resets a favorite's like counter to zero.
func (a *App) ClearVideoLikes(id int64) (*video.Video, error) {
	return a.lib.ClearVideoLikes(id)
}

// OpenVideo plays the file with the system default player.
func (a *App) OpenVideo(id int64) error {
	return a.videos.Open(id)
}

// OpenContainingFolder reveals the file in the system file manager.
func (a *App) OpenContainingFolder(id int64) error {
	return a.videos.OpenContainingFolder(id)
}

// CopyPath puts the video's absolute path on the clipboard.
func (a *App) CopyPath(id int64) error {
	v, err := a.videos.Get(id)
	if err != nil {
		return err
	}
	path := v.Path
	if v.IsRemote() {
		// The synthetic cloud:// key is an implementation detail; a user copying
		// a remote path wants the WebDAV path.
		path = v.RemotePath
	}
	if err := runtime.ClipboardSetText(a.EventContext(), path); err != nil {
		if err := platform.CopyText(path); err != nil {
			return fmt.Errorf("copy path failed: %w", err)
		}
	}
	return nil
}

// RefreshStatus re-checks one favorite's file existence.
func (a *App) RefreshStatus(id int64) (*video.Video, error) {
	return a.videos.RefreshStatus(id)
}

// RefreshVolumes re-observes which volumes the favorites live on and returns
// each one's attachment state. The library calls it once after mounting, so a
// drive that is unplugged is marked on the grid without waiting for the
// background poll. A drive that came back is reconciled too: a file it confirms
// gone is removed as a favorite (record and generated assets only).
func (a *App) RefreshVolumes() []video.VolumeChange {
	return a.lib.RefreshVolumes()
}

// ScanFileStatuses starts the manual file-existence pass (文件检测) and returns
// immediately. It is the startup check made reachable: a file deleted or moved
// while the app runs is noticed without a restart. Progress and the outcome
// arrive through the video:filescan events and FileScanStatus.
func (a *App) ScanFileStatuses() video.FileScanStatus {
	return a.lib.ScanFileStatuses()
}

// FileScanStatus returns the current or last file-existence pass, including the
// one startup already ran.
func (a *App) FileScanStatus() video.FileScanStatus {
	return a.lib.FileScanStatus()
}

// PickVideoFiles opens the native multi-file picker with a video filter.
func (a *App) PickVideoFiles() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.EventContext(), runtime.OpenDialogOptions{
		Title: "添加视频",
		Filters: []runtime.FileFilter{
			{DisplayName: "视频文件 (*.mp4;*.mkv;*.webm;*.mov;*.avi;*.m4v)", Pattern: "*.mp4;*.mkv;*.webm;*.mov;*.avi;*.m4v"},
		},
	})
}

// ---- Grouping API (分组) ----

// ListGroups returns all groups in sidebar order, with member counts.
func (a *App) ListGroups() ([]group.Group, error) {
	return a.lib.ListGroups()
}

// CreateGroup makes a new empty group and returns it.
func (a *App) CreateGroup(name string) (*group.Group, error) {
	return a.lib.CreateGroup(name)
}

// RenameGroup updates a group's name.
func (a *App) RenameGroup(id int64, name string) (*group.Group, error) {
	return a.lib.RenameGroup(id, name)
}

// DeleteGroup removes a group and its memberships.
func (a *App) DeleteGroup(id int64) error {
	return a.lib.DeleteGroup(id)
}

// SetGroupParent makes one group a child of another, or detaches it to the top
// level when parentID is nil. It rejects self-parenting and cycles.
func (a *App) SetGroupParent(id int64, parentID *int64) (*group.Group, error) {
	return a.lib.SetGroupParent(id, parentID)
}

// ReorderGroups persists the sidebar drag & drop ordering. ids is the full
// list of group ids in the desired display order.
func (a *App) ReorderGroups(ids []int64) error {
	return a.lib.ReorderGroups(ids)
}

// VideoGroupIDs returns the ids of the groups a video belongs to.
func (a *App) VideoGroupIDs(videoID int64) ([]int64, error) {
	return a.lib.VideoGroupIDs(videoID)
}

// SetVideoGroups assigns a video to the given groups (replacing previous ones).
func (a *App) SetVideoGroups(videoID int64, groupIDs []int64) error {
	return a.lib.SetVideoGroups(videoID, groupIDs)
}

// AddVideoToGroup adds one video to one group (idempotent; no-op if already a member).
func (a *App) AddVideoToGroup(videoID int64, groupID int64) error {
	return a.lib.AddVideoToGroup(videoID, groupID)
}

// SuggestGroupsForVideos infers recommended groups for just-added videos from
// their file names (group-name substring match). The frontend shows these in
// the post-add suggestion carousel.
func (a *App) SuggestGroupsForVideos(paths []string) []api.VideoGroupSuggestion {
	return a.lib.SuggestGroupsForVideos(paths)
}

// EnsureVideoIn4KGroup auto-assigns a confirmed 4K video to the "4K" group. It
// is called by the background processor once ffprobe resolves the resolution.
func (a *App) EnsureVideoIn4KGroup(videoID int64) {
	a.lib.EnsureVideoIn4KGroup(videoID)
}

// VideosInGroup returns the favorite ids in a group, newest first.
func (a *App) VideosInGroup(groupID int64) ([]int64, error) {
	return a.lib.VideoIDsInGroup(groupID)
}

// ---- Server mode (局域网访问) ----

// StartServer launches the LAN HTTP server and returns the access URLs for a
// phone/browser on the same network. When port <= 0 (the frontend's default and
// the app's startup call), the server binds to the fixed, configurable port
// from Settings > 服务器 (falling back to settings.DefaultServerPort). Pass a
// positive port to override the configured one for a single start.
func (a *App) StartServer(port int) (server.StartInfo, error) {
	if a.server == nil {
		return server.StartInfo{}, fmt.Errorf("服务未初始化")
	}
	if port <= 0 {
		port = settings.DefaultServerPort
		if a.settings != nil {
			configured, err := a.settings.ServerPort()
			if err != nil {
				return server.StartInfo{}, err
			}
			port = configured
		}
	}
	return a.server.Start(port)
}

// GetServerPort returns the current fixed LAN server port (the configured
// value, or the default when none has been set).
func (a *App) GetServerPort() (int, error) {
	if a.settings == nil {
		return settings.DefaultServerPort, nil
	}
	return a.settings.ServerPort()
}

// SetServerPort validates and persists the fixed LAN server port used on the
// next server start. The backend returns a user-facing Chinese message when
// the value is out of range.
func (a *App) SetServerPort(port int) error {
	if a.settings == nil {
		return fmt.Errorf("设置存储未初始化")
	}
	return a.settings.SetServerPort(port)
}

// StopServer shuts down the LAN HTTP server.
func (a *App) StopServer() error {
	if a.server == nil {
		return nil
	}
	return a.server.Stop()
}

// ServerStatus returns whether the server is running and its access URLs.
func (a *App) ServerStatus() server.StartInfo {
	if a.server == nil {
		return server.StartInfo{}
	}
	return a.server.Status()
}

// ---- Folder watch (目录监控) ----

// PickWatchDir opens the native directory picker for the folder-watch list.
func (a *App) PickWatchDir() (string, error) {
	return runtime.OpenDirectoryDialog(a.EventContext(), runtime.OpenDialogOptions{
		Title: "选择要监控的目录",
	})
}

// ListWatchDirs returns the auto-monitored directory list.
func (a *App) ListWatchDirs() ([]watch.WatchedDir, error) {
	return a.lib.ListWatchDirs()
}

// AddWatchDir registers a directory for watching: its videos (recursively) are
// imported now, and video files that appear later are picked up automatically.
func (a *App) AddWatchDir(path string) (*watch.WatchedDir, error) {
	return a.lib.AddWatchDir(path)
}

// RemoveWatchDir unregisters a monitored directory; existing favorites are kept.
func (a *App) RemoveWatchDir(id int64) error {
	return a.lib.RemoveWatchDir(id)
}

// RescanWatchDirs schedules an immediate rescan of every monitored directory.
// Outcomes arrive asynchronously via the watch:scanned event.
func (a *App) RescanWatchDirs() {
	a.lib.RescanWatchDirs()
}

// ---- Cloud sources (WebDAV) ----

// ListCloudConnections returns the configured WebDAV sources and their last
// observed health. Local-only users see an empty list.
func (a *App) ListCloudConnections() ([]cloud.ConnectionView, error) {
	return a.lib.ListCloudConnections()
}

// SaveCloudConnection creates or updates a source; the password is protected
// before it is written and never returned.
func (a *App) SaveCloudConnection(input cloud.ConnectionInput) (*cloud.ConnectionView, error) {
	return a.lib.SaveCloudConnection(input)
}

// DeleteCloudConnection removes a source, refusing while favorites still use it.
func (a *App) DeleteCloudConnection(id int64) error {
	return a.lib.DeleteCloudConnection(id)
}

// TestCloudConnection checks the address/credentials before saving.
func (a *App) TestCloudConnection(input cloud.ConnectionInput) (cloud.TestResult, error) {
	return a.lib.TestCloudConnection(input)
}

// BrowseCloud lists one remote collection for the browser dialog.
func (a *App) BrowseCloud(id int64, path string) ([]cloud.Entry, error) {
	return a.lib.BrowseCloud(id, path)
}

// AddCloudVideos adds the selected remote files to the library.
func (a *App) AddCloudVideos(sourceID int64, paths []string) []video.AddVideoResult {
	return a.lib.AddCloudVideos(sourceID, paths)
}

// CloudStorageAvailable reports whether a password can be protected on this
// platform (DPAPI on Windows).
func (a *App) CloudStorageAvailable() bool { return a.lib.CloudStorageAvailable() }

// ---- Guided tour (新手引导) ----

// GetTourSeen reports whether the first-run guided tour has already played. The
// flag is database state, not a webview preference: a disposable or corrupted
// webview profile must not bring the welcome back on every launch.
func (a *App) GetTourSeen() (bool, error) {
	if a.settings == nil {
		return false, nil
	}
	return a.settings.TourSeen()
}

// SetTourSeen records that the tour played, so later launches start in the
// library. It is safe to call more than once (replays from Settings).
func (a *App) SetTourSeen() error {
	if a.settings == nil {
		return fmt.Errorf("设置存储未初始化")
	}
	return a.settings.MarkTourSeen()
}
