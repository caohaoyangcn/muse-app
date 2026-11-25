// HTTP transport for the LAN web client.
//
// This is the browser counterpart of services/api.ts: the desktop module calls
// Wails bindings (window.go), which do not exist in a browser, so the web client
// speaks JSON over the LAN server instead. Both are thin adapters — all business
// logic lives in the Go api.Library façade behind them.
//
// Views should not import this module directly: they call `client.*` from
// services/client.ts, which services/client.http.ts implements on top of these
// functions. The remaining exports here are capabilities only the web client has
// (media URLs, `getVideo`) or platform-specific playback bookkeeping.
import type {
  ActorSummary,
  BackgroundTasks,
  ContentDuplicateGroup,
  ContentScanStatus,
  Highlight,
  JavDetail,
  JavDuplicateGroup,
  JavTask,
  JavTaskOverview,
  SimilarVideo,
  TrashSnapshot,
  Video,
} from "../types";

const API = "/api";

/** An error carrying the server's user-facing message (the `error` field). */
export class ApiError extends Error {}

/**
 * A 401: the LAN shell must stop showing data and ask for the access key again.
 * A subclass so existing `instanceof ApiError` handling keeps working.
 */
export class UnauthorizedError extends ApiError {}

// The access key is kept in memory only. The server's HttpOnly cookie
// authenticates every request; this copy exists so a stream URL handed to a
// native player (which cannot send that cookie) can carry it.
let accessKey = "";
let sessionLost: (() => void) | null = null;

/** The key of the current session, or "" before login. */
export function currentAccessKey(): string {
  return accessKey;
}

/** Registers the shell's reaction to a 401. Returns an unsubscribe function. */
export function onSessionLost(listener: () => void): () => void {
  sessionLost = listener;
  return () => {
    if (sessionLost === listener) sessionLost = null;
  };
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, init);
  } catch {
    // A network-level failure here almost always means the server stopped or
    // the phone dropped off the LAN.
    throw new ApiError("无法连接到服务，请确认桌面端局域网服务仍在运行");
  }
  if (!res.ok) {
    let message = `请求失败（${res.status}）`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body?.error) message = body.error;
    } catch {
      // Non-JSON error body: keep the status-based message.
    }
    if (res.status === 401 && !path.startsWith(`${API}/session`)) {
      sessionLost?.();
      throw new UnauthorizedError(message);
    }
    throw new ApiError(message);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

// ---------- session ----------

/**
 * Asks the server whether this browser already holds the key. Returns the key
 * (needed for external-player URLs) or null when the login screen must show.
 */
export async function sessionKey(): Promise<string | null> {
  let res: Response;
  try {
    res = await fetch(`${API}/session`, { cache: "no-store" });
  } catch {
    throw new ApiError("无法连接到服务，请确认桌面端局域网服务仍在运行");
  }
  if (res.status === 401) {
    accessKey = "";
    return null;
  }
  if (!res.ok) {
    throw new ApiError(`请求失败（${res.status}）`);
  }
  const body = (await res.json()) as { authenticated?: boolean; key?: string };
  if (!body?.authenticated) {
    accessKey = "";
    return null;
  }
  accessKey = body.key ?? "";
  return accessKey;
}

/** Proves the access key; the server installs the HttpOnly session cookie. */
export async function loginWithKey(key: string): Promise<string> {
  let res: Response;
  try {
    res = await fetch(`${API}/session`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key }),
    });
  } catch {
    throw new ApiError("无法连接到服务，请确认桌面端局域网服务仍在运行");
  }
  if (!res.ok) {
    let message = res.status === 401 ? "访问口令错误" : `请求失败（${res.status}）`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body?.error) message = body.error;
    } catch {
      // Non-JSON error body: keep the status-based message.
    }
    throw new ApiError(message);
  }
  const body = (await res.json()) as { key?: string };
  accessKey = body?.key ?? "";
  return accessKey;
}

/** Ends this browser's session. The shell returns to the login screen. */
export function logout(): void {
  accessKey = "";
  void fetch(`${API}/session`, { method: "DELETE" }).catch(() => undefined);
}

function jsonRequest<T>(path: string, method: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method,
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

// ---------- reads ----------

export function listVideos(keyword?: string): Promise<Video[]> {
  const query = keyword ? `?search=${encodeURIComponent(keyword)}` : "";
  return request<Video[]>(`${API}/videos${query}`);
}

export function getVideo(id: number): Promise<Video> {
  return request<Video>(`${API}/videos/${id}`);
}

/** The scraped metadata for a favorite, or null when it has not been scraped. */
export function javDetail(id: number): Promise<JavDetail | null> {
  return request<JavDetail | null>(`${API}/videos/${id}/jav`);
}

/** The scrape task for a favorite, or null when no scrape was ever queued. */
export function javTask(id: number): Promise<JavTask | null> {
  return request<JavTask | null>(`${API}/videos/${id}/jav/task`);
}

export function listJavTasks(): Promise<JavTaskOverview[]> {
  return request<JavTaskOverview[]>(`${API}/jav/tasks`);
}

/** 后台任务看板：刮削、片名翻译、内容扫描等任务的只读进度快照。 */
export function backgroundTasks(): Promise<BackgroundTasks> {
  return request<BackgroundTasks>(`${API}/tasks`);
}

export function listActors(): Promise<ActorSummary[]> {
  return request<ActorSummary[]>(`${API}/actors`);
}

export function videosByActor(name: string): Promise<Video[]> {
  return request<Video[]>(`${API}/actors/videos?name=${encodeURIComponent(name)}`);
}

/** 详情页底部的「类似影片」；limit 传 0（或省略）时用后端默认条数。 */
export function similarVideos(id: number, limit = 0): Promise<SimilarVideo[]> {
  const query = limit > 0 ? `?limit=${limit}` : "";
  return request<SimilarVideo[]>(`${API}/videos/${id}/similar${query}`);
}

/**
 * 一部影片的精彩时刻。服务端返回全部状态（含失败），手机端只读、自己过滤掉
 * failed：失败的重试与删除入口都在桌面端。
 */
export function listVideoHighlights(videoId: number): Promise<Highlight[]> {
  return request<Highlight[]>(`${API}/videos/${videoId}/highlights`);
}

/**
 * 查重：识别出同一番号被收藏多个版本的分组。带上下集标记的文件（如
 * abc-123-1 / abc-123-2）视为不同作品，不会出现在这里。
 */
export function listJavDuplicates(includeIgnored = false): Promise<JavDuplicateGroup[]> {
  const query = includeIgnored ? "?includeIgnored=1" : "";
  return request<JavDuplicateGroup[]>(`${API}/jav/duplicates${query}`);
}

/**
 * 内容重复：同一份字节被收藏了两份。判据是内容指纹（文件大小 + 首尾各 1 MiB），
 * 与番号无关；同一作品的不同压制不会被判为重复。
 */
export function listContentDuplicates(includeIgnored = false): Promise<ContentDuplicateGroup[]> {
  const query = includeIgnored ? "?includeIgnored=1" : "";
  return request<ContentDuplicateGroup[]>(`${API}/duplicates/content${query}`);
}

/** 内容指纹扫描的当前（或最近一次）状态。 */
export function contentScanStatus(): Promise<ContentScanStatus> {
  return request<ContentScanStatus>(`${API}/duplicates/content/scan`);
}

/** 启动整库内容指纹扫描（后台执行，用 contentScanStatus 跟进）。 */
export function scanContentDuplicates(): Promise<ContentScanStatus> {
  return jsonRequest<ContentScanStatus>(`${API}/duplicates/content/scan`, "POST");
}

// ---------- writes ----------

export function updateVideoInfo(
  id: number,
  title: string,
  notes: string,
): Promise<Video> {
  return jsonRequest<Video>(`${API}/videos/${id}/info`, "PUT", { title, notes });
}

export function likeVideo(id: number): Promise<Video> {
  return jsonRequest<Video>(`${API}/videos/${id}/likes`, "POST");
}

export function clearVideoLikes(id: number): Promise<Video> {
  return jsonRequest<Video>(`${API}/videos/${id}/likes`, "DELETE");
}

/**
 * 最佳努力地记录一次播放：实际播放已交给 Infuse / VLC 或 OpenList，服务端
 * 只更新时间戳。`keepalive` 让请求在 `window.location.href` 跳转后仍能发出；
 * 失败不影响播放，只影响「最近播放」的排序与分组。
 */
export function markVideoOpened(id: number): void {
  void fetch(`${API}/videos/${id}/opened`, { method: "POST", keepalive: true }).catch(
    () => undefined,
  );
}

/** 把收藏移入回收站（软删除）；源文件不会被删除。 */
export function removeVideo(id: number): Promise<void> {
  return request<void>(`${API}/videos/${id}`, { method: "DELETE" });
}

/** 回收站条目与当前保留天数。 */
export function listTrash(): Promise<TrashSnapshot> {
  return request<TrashSnapshot>(`${API}/trash`);
}

/** 从回收站恢复一条收藏。 */
export function restoreVideo(id: number): Promise<void> {
  return request<void>(`${API}/trash/${id}/restore`, { method: "POST" });
}

/** 彻底删除一条回收站收藏；只清除 Muse 的记录与生成图片。 */
export function deleteVideoPermanently(id: number): Promise<void> {
  return request<void>(`${API}/trash/${id}`, { method: "DELETE" });
}

/** 清空回收站，返回彻底删除的条目数。 */
export function emptyTrash(): Promise<number> {
  return jsonRequest<{ count: number }>(`${API}/trash`, "DELETE").then((r) => r.count);
}

/** 长期保留的网盘归档。 */
export function listArchived(): Promise<Video[]> {
  return request<Video[]>(`${API}/archive`);
}

/** 清空全部归档，返回彻底删除的条目数。 */
export function clearArchivedVideos(): Promise<number> {
  return jsonRequest<{ count: number }>(`${API}/archive`, "DELETE").then((r) => r.count);
}

/** 保留重复分组的全部版本（等价于忽略重复提示）。 */
export function ignoreJavDuplicate(key: string): Promise<void> {
  return jsonRequest<void>(`${API}/jav/duplicates/ignore`, "POST", { key });
}

/** 保留内容重复分组的全部拷贝（等价于不再提示这组文件）。 */
export function ignoreContentDuplicate(hash: string): Promise<void> {
  return jsonRequest<void>(`${API}/duplicates/content/ignore`, "POST", { hash });
}

/** 重新读取一组文件的内容指纹：成员在应用之外被改动后，用它让结论重新可信。 */
export function reverifyContentDuplicate(hash: string): Promise<void> {
  return jsonRequest<void>(`${API}/duplicates/content/reverify`, "POST", { hash });
}

/**
 * 完整校验：逐字节读取并比较整份文件。部分指纹只读首尾各 1 MiB，这一步会读整个
 * 文件（5 GB 在 USB 机械盘上约 40 秒），所以由用户显式触发、后台一次读一个。
 */
export function confirmContentDuplicate(hash: string): Promise<void> {
  return jsonRequest<void>(`${API}/duplicates/content/confirm`, "POST", { hash });
}

// ---------- media URLs ----------

/**
 * The full-size JAV cover, falling back to the generated thumbnail. Uses the
 * stored relative cover path so the server's own layout decides the URL shape.
 */
export function coverURL(v: Video): string {
  if (v.coverSource === "scraped" && v.scrapedCoverPath) {
    return `/${v.scrapedCoverPath}`;
  }
  return `/thumbnails/${v.id}.jpg`;
}
