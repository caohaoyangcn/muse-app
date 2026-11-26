// Replaces the Wails runtime inside the design preview.
//
// The real shell injects `window.go` (generated bindings) and `window.runtime`
// (events, drag-and-drop, window controls). A browser has neither, so every
// module that imports them at the top of the file would throw the moment it is
// called. This module answers those calls with inert, correctly-shaped data and
// logs anything it does not know about, which is how a new platform call shows
// up while capturing screens instead of silently breaking a frame.
import { VIDEOS } from "./data";

type Stub = (...args: unknown[]) => unknown;

const noop = (): void => {};

const FILE_SCAN_IDLE = {
  running: false,
  total: VIDEOS.length,
  checked: VIDEOS.length,
  removed: 0,
  skipped: 0,
  revived: 0,
  offline: 1,
  remote: 1,
  error: "",
  startedAt: Date.now() - 90_000,
  finishedAt: Date.now() - 88_000,
};

const SERVER_IDLE = {
  running: false,
  urls: [],
  local: "",
  port: 8899,
};

const STUBS: Record<string, Stub> = {
  // ---- settings surface ----
  GetTourSeen: () => true, // the guided tour stays closed unless asked for
  GetHiddenTags: () => [],
  SetHiddenTags: noop,
  GetJavBusCookieStatus: () => ({ configured: false }),
  GetFC2CookieStatus: () => ({ configured: false }),
  GetLLMSettings: () => ({
    enabled: true,
    baseUrl: "https://api.example.com/v1",
    model: "muse-translate",
    hasApiKey: true,
    resumeMissing: true,
  }),
  GetScrapeProxy: () => "",
  GetServerPort: () => 8899,
  GetLanAccessKey: () => "muse-design-preview",
  RotateLanAccessKey: () => "muse-design-preview",
  GetTrashRetentionDays: () => 14,
  ServerStatus: () => SERVER_IDLE,
  StartServer: () => ({ ...SERVER_IDLE, running: true, local: "http://127.0.0.1:8899" }),
  StopServer: noop,

  // ---- library platform calls ----
  RefreshVolumes: () => [],
  RefreshStatus: (...args: unknown[]) =>
    VIDEOS.find((video) => video.id === args[0]) ?? null,
  FileScanStatus: () => FILE_SCAN_IDLE,
  ScanFileStatuses: () => FILE_SCAN_IDLE,
  ListWatchDirs: () => [],
  ListCloudConnections: () => [],
  CloudStorageAvailable: () => false,
  ActorFillStatus: () => ({
    total: 4,
    queued: 0,
    running: 0,
    succeeded: 4,
    notFound: 0,
    failed: 0,
    active: false,
  }),
  StartActorFill: () => 0,
  CheckJavCodes: () => [],
  PickVideoFiles: () => null,
  PickWatchDir: () => "",

  // ---- actions that would leave the preview ----
  OpenVideo: noop,
  OpenContainingFolder: noop,
  CopyPath: noop,
  OpenCloudPage: noop,
};

/**
 * Installs `window.go` / `window.runtime` before React renders. Idempotent: the
 * preview can be re-imported by Vite HMR without stacking stubs.
 */
export function installPlatformStubs(): void {
  const host = window as unknown as {
    go?: { main?: { App?: Record<string, unknown> } };
    runtime?: Record<string, unknown>;
    __museDesignStubs?: boolean;
  };
  if (host.__museDesignStubs) return;
  host.__museDesignStubs = true;

  const app = new Proxy(
    {},
    {
      get(_target, property: string) {
        const stub = STUBS[property];
        if (stub) return (...args: unknown[]) => Promise.resolve(stub(...args));
        return (...args: unknown[]) => {
          console.warn(`[design-preview] 未模拟的平台调用：${property}`, args);
          return Promise.resolve(null);
        };
      },
    },
  ) as Record<string, unknown>;

  host.go = { main: { App: app } };
  host.runtime = {
    // Every event subscription is a no-op that still returns its unsubscribe
    // handle, so `attachBackendEvents` cleans up the way it expects to.
    EventsOn: () => () => {},
    EventsOnMultiple: () => () => {},
    EventsOnce: () => () => {},
    EventsOff: noop,
    EventsOffAll: noop,
    EventsEmit: noop,
    OnFileDrop: noop,
    OnFileDropOff: noop,
    BrowserOpenURL: (url: string) => {
      console.info(`[design-preview] 忽略外链打开：${url}`);
    },
    LogPrint: noop,
    LogTrace: noop,
    LogDebug: noop,
    LogInfo: noop,
    LogWarning: noop,
    LogError: noop,
    LogFatal: noop,
  };
}
