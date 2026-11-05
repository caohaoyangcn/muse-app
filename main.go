package main

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"muse-app/backend/api"
	"muse-app/backend/cloud"
	"muse-app/backend/database"
	"muse-app/backend/group"
	"muse-app/backend/jav"
	"muse-app/backend/jav/providers"
	"muse-app/backend/netproxy"
	"muse-app/backend/platform"
	"muse-app/backend/server"
	"muse-app/backend/settings"
	"muse-app/backend/video"
	"muse-app/backend/watch"
)

//go:embed all:frontend/dist
var assets embed.FS

const (
	defaultJavBusBaseURL  = "https://javbus.com"
	defaultFC2BaseURL     = "https://fc2cmadb.com"
	fc2DiagnosticsVersion = 1

	// Initial desktop window geometry. These are the fallback for a first
	// launch, and the lower bound when a saved geometry is restored.
	defaultWindowWidth  = 1100
	defaultWindowHeight = 720
	minWindowWidth      = 960
	minWindowHeight     = 600

	// shutdownDeadline bounds how long the process may spend draining
	// background work after the window closes. Queues are persisted (or
	// re-derived) and resume on the next launch, so an invisible process
	// waiting on a slow disk or a stuck HTTP peer is never acceptable.
	shutdownDeadline = 8 * time.Second
)

// newJavBusProvider keeps runtime-only JavBus configuration at the application
// boundary. Cookie values are read from the environment, passed in memory to
// the provider, and never persisted or logged. The transport reads the proxy
// config on every request, so a settings change applies without a restart.
func newJavBusProvider(log *slog.Logger, storedCookie string, proxy *netproxy.Config) *providers.JavBus {
	cookie := strings.TrimSpace(os.Getenv("MUSE_JAVBUS_COOKIE"))
	if cookie == "" {
		cookie = strings.TrimSpace(storedCookie)
	}
	baseURL := strings.TrimSpace(os.Getenv("MUSE_JAVBUS_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultJavBusBaseURL
	}

	transport := proxy.Transport()
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second}
	provider := providers.NewJavBusWithConfig(client, baseURL, providers.JavBusConfig{Cookie: cookie})

	if log != nil {
		host := "unknown"
		if parsed, err := url.Parse(baseURL); err == nil && parsed.Host != "" {
			host = parsed.Host
		}
		log.Info("jav provider configured",
			"provider", provider.Name(),
			"host", host,
			"cookieConfigured", cookie != "",
		)
	}
	return provider
}

func newFC2CMADBProvider(log *slog.Logger, storedCookie string, proxy *netproxy.Config) *providers.FC2CMADB {
	cookie := strings.TrimSpace(os.Getenv("MUSE_FC2_COOKIE"))
	if cookie == "" {
		cookie = strings.TrimSpace(storedCookie)
	}
	transport := proxy.Transport()
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second}
	provider := providers.NewFC2CMADBWithLogger(client, defaultFC2BaseURL, providers.FC2CMADBConfig{Cookie: cookie}, log)
	if log != nil {
		log.Info("jav provider configured", "provider", provider.Name(), "host", "fc2cmadb.com", "cookieConfigured", cookie != "")
	}
	return provider
}

func main() {
	if wailsBindingBuild {
		runBindingGenerator()
		return
	}

	dataDir, err := platform.AppDataDir("Muse")
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve application data directory: %v\n", err)
		os.Exit(1)
	}
	log := setupLogging(dataDir)
	log.Info("muse build diagnostics", "fc2DiagnosticsVersion", fc2DiagnosticsVersion)
	thumbDir := filepath.Join(dataDir, "thumbnails")
	javCoverDir := filepath.Join(dataDir, "covers", "jav")
	actorAvatarDir := filepath.Join(dataDir, "avatars")
	for _, dir := range []string{dataDir, thumbDir, javCoverDir, actorAvatarDir, filepath.Join(dataDir, "cloud-cache"), filepath.Join(dataDir, "logs")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Error("create data dir failed", "dir", dir, "err", err)
			fatalExit(dataDir)
		}
	}
	enableCrashLog(dataDir, log)

	db, mig, err := database.Open(dataDir)
	if err != nil {
		log.Error("open database failed", "err", err)
		fatalExit(dataDir)
	}
	defer func() { _ = db.Close() }()
	repo := video.NewSQLiteRepository(db)

	// ffmpeg/ffprobe: bundled next to the exe first, then PATH (platform.FindTool).
	ffprobeBin, err := platform.FindTool("ffprobe")
	if err != nil {
		log.Error(err.Error())
	}
	ffmpegBin, err := platform.FindTool("ffmpeg")
	if err != nil {
		log.Error(err.Error())
	}

	app := NewApp(dataDir, log)
	processor := video.NewProcessor(repo, ffprobeBin, ffmpegBin, thumbDir, app.Emit, log, 1)
	videoSvc := video.NewService(repo, processor, dataDir, log, app.Emit)
	// Content fingerprints (部分哈希) run on their own small pool: a pass reads
	// two megabytes per file, so the disks are the limit and not the CPU, and
	// hashing must not compete with thumbnail extraction for workers.
	fingerprinter := video.NewFingerprinter(repo, app.Emit, log, 2)
	videoSvc.SetFingerprinter(fingerprinter)
	groupSvc := group.NewService(group.NewSQLiteRepository(db), log, app.Emit)
	app.SetService(videoSvc)
	// Single business façade shared by the desktop bindings and the LAN server.
	lib := api.New(videoSvc, groupSvc, log)
	app.SetLibrary(lib)
	cloudSvc := cloud.NewService(cloud.NewSQLiteRepository(db), app.Emit, log)
	lib.SetCloudService(cloudSvc)
	cloudCache := cloud.NewCache(filepath.Join(dataDir, "cloud-cache"), cloud.DefaultCacheLimit, app.Emit, log)
	lib.SetCloudCache(cloudCache)
	// The loopback media proxy is what lets ffprobe/ffmpeg read a remote
	// favorite without WebDAV credentials appearing on a command line. It only
	// binds 127.0.0.1 and is addressed by a per-process random token.
	mediaProxy, proxyErr := cloud.NewMediaProxy(
		func(id int64) (cloud.MediaInfo, error) {
			v, err := videoSvc.Get(id)
			if err != nil {
				return cloud.MediaInfo{}, err
			}
			return cloud.MediaInfo{SourceID: v.SourceID, Path: v.RemotePath, FileName: v.FileName, Size: v.FileSize}, nil
		},
		cloudSvc.OpenRange,
		cloudSvc.StatContext,
		log,
	)
	if proxyErr != nil {
		log.Warn("media proxy not available", "err", proxyErr)
		mediaProxy = nil
	} else if err := mediaProxy.Start(); err != nil {
		log.Warn("media proxy start failed", "err", err)
		mediaProxy = nil
	}
	videoSvc.SetMediaPathResolver(func(v *video.Video) (string, error) {
		if !v.IsRemote() {
			return v.Path, nil
		}
		if mediaProxy == nil {
			return "", fmt.Errorf("网盘媒体代理未启动")
		}
		url := mediaProxy.URL(v.ID)
		if url == "" {
			return "", fmt.Errorf("网盘媒体代理未就绪")
		}
		return url, nil
	})
	// Remote play downloads first, then hands the local copy to the system
	// player. The WebDAV source is only read; cache eviction/removal never
	// touches it.
	videoSvc.SetRemoteSupport(
		func(ctx context.Context, v *video.Video) error {
			if !cloudSvc.Online(v.SourceID) {
				return fmt.Errorf("网盘未连接")
			}
			path, err := cloudCache.Ensure(ctx, cloud.CacheSpec{
				VideoID:    v.ID,
				SourceID:   v.SourceID,
				RemotePath: v.RemotePath,
				FileName:   v.FileName,
				Size:       v.FileSize,
			}, cloudSvc.StatContext, cloudSvc.OpenRange)
			if err != nil {
				return fmt.Errorf("%s", cloud.UserMessage(err))
			}
			return platform.OpenWithDefaultApp(path)
		},
		func(ctx context.Context, v *video.Video) (bool, error) {
			return cloudSvc.Exists(ctx, v.SourceID, v.RemotePath)
		},
		nil,
		func(v *video.Video) bool { return cloudSvc.Online(v.SourceID) },
	)
	videoSvc.SetRemotePage(func(v *video.Video) error {
		url, err := cloudSvc.ManagementURL(v.SourceID)
		if err != nil {
			return err
		}
		return platform.OpenURL(url)
	})
	go func() {
		if err := cloudCache.EnforceLimit(); err != nil {
			log.Warn("cloud cache cleanup failed", "err", err)
		}
	}()
	settingsStore := settings.New(db)
	app.SetSettings(settingsStore)
	// The recycle bin's retention is a user preference. The sweep reads it on
	// every pass, so a changed value applies to entries that are already in the
	// trash without restarting the app.
	lib.SetTrashRetention(func() int {
		days, configured, err := settingsStore.TrashRetentionDays()
		if err != nil || !configured {
			return api.DefaultTrashRetentionDays
		}
		return days
	})
	// The window placement is needed by both OnStartup and shutdown; build it
	// here so shutdown can include its poller as a named step.
	placement := newWindowPlacement(settingsStore, log)
	// Resolve the scraping proxy once at startup; SetScrapeProxy updates it in
	// place for the rest of the process, so providers never need rebuilding.
	proxyCfg := netproxy.New()
	if storedProxy, proxyErr := settingsStore.ScrapeProxy(); proxyErr != nil {
		log.Warn("read scrape proxy failed", "err", proxyErr)
	} else if parsedProxy, parseErr := netproxy.Parse(storedProxy); parseErr != nil {
		log.Warn("stored scrape proxy ignored", "err", parseErr)
	} else if parsedProxy != nil {
		proxyCfg.Set(parsedProxy)
	}
	app.setScrapeProxyConfig(proxyCfg)
	storedJavCookie, cookieErr := settingsStore.GetJavBusCookie()
	if cookieErr != nil {
		log.Warn("read javbus cookie failed", "provider", "javbus", "err", cookieErr)
	}
	storedFC2Cookie, fc2CookieErr := settingsStore.GetFC2Cookie()
	if fc2CookieErr != nil {
		log.Warn("read fc2 cookie failed", "provider", "fc2cmadb", "err", fc2CookieErr)
	}
	javRepo := jav.NewSQLiteRepository(db)
	javBusProvider := newJavBusProvider(log, storedJavCookie, proxyCfg)
	avmooProvider := providers.NewAvmooWithConfig(nil, "", providers.AvmooConfig{Proxy: proxyCfg.ProxyFunc})
	ordinaryProvider := providers.NewFallbackProviderWithLogger(javBusProvider, avmooProvider, log)
	fc2Provider := newFC2CMADBProvider(log, storedFC2Cookie, proxyCfg)
	javProvider := providers.NewRoutedProviderWithLogger(fc2Provider, ordinaryProvider, log)
	app.setJavBusCookieProvider(javBusProvider)
	app.setFC2CookieProvider(fc2Provider)
	log.Info("jav provider routing configured", "ordinary", ordinaryProvider.Name(), "fc2", fc2Provider.Name())
	javSvc := jav.NewService(javRepo, repo, javProvider, javCoverDir, app.Emit, log, 2)
	javSvc.SetActorAvatarDir(actorAvatarDir)
	// The backfill resolves names the movie lookups could not describe. Avmoo is
	// the only provider that publishes profiles, so it is the resolver even when
	// javbus supplied the metadata.
	javSvc.SetActorResolver(avmooProvider)
	lib.SetJavService(javSvc)
	javSvc.Start()
	// After ffprobe confirms a 4K resolution, the background processor asks the
	// façade to auto-assign it to the "4K" group (creating it on first use).
	processor.SetAutoGroup4K(lib.EnsureVideoIn4KGroup)
	// LAN web server: exposes the library + streaming to devices on the same
	// network. It starts with the app so the phone URL is ready without opening
	// the header toggle every session; the toggle still stops and restarts it.
	// A port that cannot be bound (another copy of Muse, an unrelated process)
	// is only a warning: the desktop library must keep working and the toggle
	// remains available for a retry.
	lanServer := server.New(app.lib, dataDir, log)
	lanServer.SetCloudService(cloudSvc)
	app.SetServer(lanServer)
	if info, err := app.StartServer(0); err != nil {
		log.Warn("web server auto-start failed", "err", err)
	} else {
		log.Info("web server auto-started", "port", info.Port, "urls", info.URLs)
	}
	// Folder watch (目录监控): persisted auto-monitored directories. Started
	// after every façade dependency is wired so the initial import scan already
	// reaches thumbnail + group auto-tagging; events emitted before Wails
	// startup are no-ops.
	watchSvc := watch.New(watch.NewSQLiteRepository(db), lib, app.Emit, log)
	lib.SetWatchService(watchSvc)
	watchSvc.Start()
	// Cloud sources (WebDAV): probe every configured connection in the
	// background so remote favorites can show an offline badge without any read
	// path waiting on the network. Local-only users have no connections, so this
	// is a no-op for them.
	stopCloudHealth := cloudSvc.StartHealthCheck(0)
	// Volume presence (磁盘未接入): a background poll notices an external drive
	// being unplugged or plugged back in. A returning drive is reconciled as a
	// whole: files that came back are revived, and files confirmed gone are
	// removed as favorites (record, thumbnail and metadata only — the source
	// video is never touched). A drive that is away is still not a missing file.
	stopVolumeWatch := videoSvc.WatchVolumes(0)
	// Generated images outlive their favorites in a few ways removal cannot
	// prevent on its own (a cover downloaded after the row was read, records
	// dropped by an older build). Reconcile the folders against the database at
	// startup and daily after that: one directory read per folder, in the
	// background, never on the path to the first screen.
	stopImageSweep := videoSvc.StartImageSweep(0)
	// Recycle bin retention: sweep once at startup, then hourly. A trashed
	// favorite older than the configured period is permanently removed; the
	// source file is never touched.
	stopTrashPurge := lib.StartTrashPurge(0)
	shutdown := newShutdownManager(log,
		shutdownStep{"window placement", placement.stop},
		shutdownStep{"web server", func() {
			if err := app.server.Stop(); err != nil {
				log.Warn("web server shutdown failed", "err", err)
			}
		}},
		shutdownStep{"cloud health", stopCloudHealth},
		shutdownStep{"media proxy", func() {
			if mediaProxy != nil {
				if err := mediaProxy.Close(); err != nil {
					log.Warn("media proxy shutdown failed", "err", err)
				}
			}
		}},
		shutdownStep{"folder watch", watchSvc.Stop},
		shutdownStep{"volume watch", stopVolumeWatch},
		shutdownStep{"image sweep", stopImageSweep},
		shutdownStep{"trash purge", stopTrashPurge},
		shutdownStep{"thumbnail processor", processor.Close},
		shutdownStep{"fingerprinter", fingerprinter.Close},
		shutdownStep{"jav scraper", javSvc.Close},
	)
	defer shutdown.Close()
	// Log the applied schema version; startup also re-probes favorites missing
	// metadata (an idempotent content-based backfill), see App.startup.
	log.Info("database migrated",
		"schemaVersion", mig.SchemaVersion,
		"backfillSuggested", mig.BackfillVideoMetadata,
	)

	// The desktop window remembers its geometry across restarts. The saved
	// placement is read before Wails creates the window so the maximised state
	// can be passed as a start option; the rectangle itself is applied in
	// OnStartup, before the window is first shown.
	savedPlacement, hasSavedPlacement := placement.saved()
	windowStartState := options.Normal
	if hasSavedPlacement && savedPlacement.Maximised {
		windowStartState = options.Maximised
	}

	if err := wails.Run(&options.App{
		Title:            "Muse Video",
		Width:            defaultWindowWidth,
		Height:           defaultWindowHeight,
		MinWidth:         minWindowWidth,
		MinHeight:        minWindowHeight,
		WindowStartState: windowStartState,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: &thumbnailHandler{dir: thumbDir, coverDir: javCoverDir, avatarDir: actorAvatarDir},
		},
		BackgroundColour: &options.RGBA{R: 23, G: 23, B: 27, A: 255},
		OnStartup: func(ctx context.Context) {
			placement.start(ctx, savedPlacement, hasSavedPlacement)
			app.startup(ctx)
			// The migration that created the fingerprint columns also reports
			// that no favorite has one yet, so this is the one startup that
			// scans the library. Later startups leave the library alone: new
			// imports hash themselves, and a scan would wake every drive up.
			if mig.BackfillFingerprints {
				if _, err := videoSvc.ScanContentDuplicates(); err != nil {
					log.Warn("content scan not started", "err", err)
				}
			}
		},
		OnBeforeClose: func(context.Context) bool {
			// Arm the hard-exit deadline before the first close-time work. A
			// window that is gone while Muse.exe lives on in the task manager
			// is almost always a background queue draining (thumbnails,
			// fingerprints, scrapes) or a folder scan on a slow/offline drive;
			// past the deadline the process must end regardless.
			armExitWatchdog(shutdownDeadline, log, shutdown.CurrentStep, os.Exit)
			// The window still exists here; the geometry the user left is the
			// one the next launch should restore.
			placement.flush()
			return false
		},
		OnShutdown: func(context.Context) {
			shutdown.Close()
		},
		Bind: []interface{}{app},
		DragAndDrop: &options.DragAndDrop{
			// Wails handles drops natively and delivers real file paths;
			// macOS must leave WebView drops enabled so in-app card dragging reaches
			// React instead of being intercepted as an empty native file drop.
			EnableFileDrop:     true,
			DisableWebViewDrop: runtime.GOOS != "darwin",
			CSSDropProperty:    "--wails-drop-target",
			CSSDropValue:       "drop",
		},
	}); err != nil {
		log.Error("app run failed", "err", err)
		fatalExit(dataDir)
	}
	// Reached only when the run loop returns on its own (the window was closed).
	// A run whose app.log ends without this line died before its shutdown path;
	// crash.log then holds the reason.
	log.Info("app exiting")
}

// runBindingGenerator is used by Wails' temporary `bindings` build. Binding
// generation only needs the exported App method set; starting the real
// application here would recover background scrape/watch jobs and block the
// generator while those workers drain during shutdown.
func runBindingGenerator() {
	app := NewApp("", slog.Default())
	if err := wails.Run(&options.App{
		Title: "Muse Video",
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Bind: []interface{}{app},
	}); err != nil {
		fmt.Fprintf(os.Stderr, "generate Wails bindings: %v\n", err)
		os.Exit(1)
	}
}

// thumbnailHandler serves generated thumbnails to the frontend at
// /thumbnails/<id>.jpg, since the desktop webview blocks file:// images.
type thumbnailHandler struct {
	dir       string
	coverDir  string
	avatarDir string
}

var thumbnailNamePattern = regexp.MustCompile(`^\d+\.jpg$`)

var coverNamePattern = regexp.MustCompile(`^\d+\.jpg$`)

// avatarNamePattern accepts the provider's performer ids, which are lowercase
// alphanumerics. Anything else is rejected before it reaches the filesystem.
var avatarNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+\.jpg$`)

func (h *thumbnailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/thumbnails/"):
		name := path.Base(r.URL.Path)
		if !thumbnailNamePattern.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(h.dir, name))
	case strings.HasPrefix(r.URL.Path, "/covers/jav/"):
		name := path.Base(r.URL.Path)
		if !coverNamePattern.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(h.coverDir, name))
	case strings.HasPrefix(r.URL.Path, "/avatars/"):
		name := path.Base(r.URL.Path)
		if !avatarNamePattern.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(h.avatarDir, name))
	default:
		http.NotFound(w, r)
	}
}

func setupLogging(dataDir string) *slog.Logger {
	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	logPath := filepath.Join(logDir, "app.log")
	// V1 log strategy: append, but start fresh when the file grows past 10 MB.
	if info, err := os.Stat(logPath); err == nil && info.Size() > 10*1024*1024 {
		_ = os.Remove(logPath)
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		// GUI apps have an invalid stderr handle (no console). io.MultiWriter
		// stops at the first writer that errors, so combining stderr here would
		// silently drop every log line. Write only to the file; stderr is used
		// solely as a fallback when the file cannot be opened.
		return slog.New(slog.NewTextHandler(f, nil))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func fatalExit(dataDir string) {
	fmt.Fprintf(os.Stderr, "Startup failed; see %s\n", filepath.Join(dataDir, "logs", "app.log"))
	os.Exit(1)
}

// enableCrashLog keeps unhandled panics and fatal runtime errors on disk.
//
// A GUI process has no console: the runtime's report goes to stderr, which
// nobody can read, and slog never sees it. Without this, an app that "closed by
// itself" leaves an app.log that simply stops — and no crash record in Windows
// either, because it is not a native fault. The runtime duplicates the
// descriptor, so the handle here can be closed immediately.
func enableCrashLog(dataDir string, log *slog.Logger) {
	logPath := filepath.Join(dataDir, "logs", "crash.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Warn("crash log unavailable", "path", logPath, "err", err)
		return
	}
	defer func() { _ = f.Close() }()
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		log.Warn("crash log unavailable", "path", logPath, "err", err)
		return
	}
	log.Info("crash log armed", "path", logPath)
}
