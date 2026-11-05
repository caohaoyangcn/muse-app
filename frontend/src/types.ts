// Types re-export the Wails-generated models so the bindings stay the single
// source of truth for backend shapes.
import { api, cloud, group, jav, server, video, watch } from "../wailsjs/go/models";

export type Video = video.Video;
export type AddVideoResult = video.AddVideoResult;
/** 回收站快照：条目 + 当前保留天数（由后端统一给出，避免客户端各算一套）。 */
export type TrashSnapshot = api.TrashSnapshot;
export type Group = group.Group;
export type ServerStartInfo = server.StartInfo;
export type GroupSuggestion = api.GroupSuggestion;
export type VideoGroupSuggestion = api.VideoGroupSuggestion;
export type WatchedDir = watch.WatchedDir;
/** 一个只读 WebDAV 网盘连接（不含密码）。 */
export type CloudConnection = cloud.ConnectionView;
export type CloudConnectionInput = cloud.ConnectionInput;
export type CloudEntry = cloud.Entry;
export type CloudTestResult = cloud.TestResult;
/** 网盘下载缓存的占用情况。 */
export type CloudCacheInfo = cloud.CacheInfo;
/** cloud:download 进度事件（下载/完成/失败/取消）。 */
export type CloudDownloadProgress = {
  videoId: number;
  fileName: string;
  downloaded: number;
  total: number;
  status: "downloading" | "done" | "error" | "cancelled";
  error: string;
};
export type JavDetail = jav.Detail;
export type JavTask = jav.Task;
export type ActorSummary = jav.ActorSummary;
/** 演员档案：刮削到的头像、生日、身高三围等信息。 */
export type ActorProfile = jav.ActorProfile;
/** 演员档案补全（补全演员信息）的进度。 */
export type ActorFillStatus = jav.ActorFillStatus;
export type JavDuplicateGroup = jav.DuplicateGroup;
// 内容重复（部分哈希去重）：同一份字节被收藏了两份。与番号查重是不同维度，两者
// 可以同时命中同一组文件。
export type ContentDuplicateGroup = video.ContentDuplicateGroup;
export type ContentDuplicateMember = video.ContentDuplicateMember;
export type ContentScanStatus = video.ContentScanStatus;
/** 文件检测（手动重跑启动时的“文件是否存在”检查）的运行状态。 */
export type FileScanStatus = video.FileScanStatus;
/** One volume's attachment state as the backend last observed it. */
export type VolumeChange = video.VolumeChange;

export type JavTaskOverview = jav.TaskOverview;

export type JavBusCookieStatus = {
  configured: boolean;
};

export type FC2CookieStatus = {
  configured: boolean;
};

export type VideoStatus = "available" | "missing";
export const STATUS_AVAILABLE: VideoStatus = "available";
export const STATUS_MISSING: VideoStatus = "missing";

// A library view is either all favorites, a single group, the scrape-task
// board, the aggregated actor index, the 查重 (duplicate) worklist, or the
// search results. An individual actor's film list is not a filter — it opens as
// its own workspace tab.
export type ViewFilter =
  | { kind: "all" }
  | { kind: "group"; id: number }
  | { kind: "scrapeTasks"; status: "queued" | "succeeded" | "failed" }
  | { kind: "actors" }
  | { kind: "duplicates" }
  | { kind: "trash" };

// Custom MIME type used to carry a video id during HTML5 drag & drop from a
// video card to a sidebar group. Distinct from "text/plain" (which the sidebar
// uses for group reordering) so the two drags never collide.
export const MIME_VIDEO_DRAG = "application/x-video-drag";

// Custom MIME type that carries a comma-separated list of video ids when a
// multi-selected batch of cards is dragged (the selection set is dropped as a
// whole). A single card drag still uses MIME_VIDEO_DRAG.
export const MIME_VIDEO_DRAG_MULTI = "application/x-video-drag-multi";

// Custom MIME type that carries an actor name when an actor chip is dragged
// onto another chip to merge the two stage names (演员合并). Kept separate from
// the video drags so dropping a chip is never mistaken for adding a film to a
// group.
export const MIME_ACTOR_DRAG = "application/x-actor-drag";

// A workspace tab is the library home, one video detail tab (its id), or one
// actor's film list. Actor tabs are tagged strings so they can share the same
// strip and history stacks as numeric video ids without colliding.
export type ActorTab = `actor:${string}`;
export type WorkspaceTab = "library" | number | ActorTab;

export const actorTabID = (name: string): ActorTab => `actor:${name}`;

/** The actor name an actor tab points at, or null for the other tab kinds. */
export const actorTabName = (tab: WorkspaceTab): string | null =>
  typeof tab === "string" && tab.startsWith("actor:") ? tab.slice("actor:".length) : null;

// Library grid sort options. Sorting happens in the frontend over the fully
// loaded list, so it applies uniformly to every view (all / group / search
// results).
export type SortKey = "createdAt" | "fileSize" | "fileName" | "likeCount";
export type SortDir = "asc" | "desc";

// How the library grid renders the video list: thumbnail cards or compact
// rows. The choice is a UI preference persisted in localStorage.
export type ViewMode = "grid" | "list";

// Card size is the target card width in CSS pixels, shared by both view modes:
// the grid renders fixed-width tracks and the list derives its row thumbnail,
// title size and padding from it. The usable range and default are per client
// (desktop vs mobile), so this type deliberately carries no bounds.
export type CardSize = number;
