// HTTP transport for the LAN mobile client.
//
// This is the mobile counterpart of services/api.ts: the desktop module calls
// Wails bindings (window.go), which do not exist in a browser, so the mobile
// client speaks JSON over the LAN server instead. Both are thin adapters — all
// business logic lives in the Go api.Library façade behind them.
import type {
  ActorSummary,
  ContentDuplicateGroup,
  ContentScanStatus,
  Group,
  JavDetail,
  JavDuplicateGroup,
  JavTask,
  JavTaskOverview,
  Video,
} from "../types";

const API = "/api";

/** An error carrying the server's user-facing message (the `error` field). */
export class ApiError extends Error {}

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
    throw new ApiError(message);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
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

export function listGroups(): Promise<Group[]> {
  return request<Group[]>(`${API}/groups`);
}

export function videosInGroup(id: number): Promise<Video[]> {
  return request<Video[]>(`${API}/groups/${id}/videos`);
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

export function listActors(): Promise<ActorSummary[]> {
  return request<ActorSummary[]>(`${API}/actors`);
}

export function videosByActor(name: string): Promise<Video[]> {
  return request<Video[]>(`${API}/actors/videos?name=${encodeURIComponent(name)}`);
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

export function removeVideo(id: number): Promise<void> {
  return request<void>(`${API}/videos/${id}`, { method: "DELETE" });
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
