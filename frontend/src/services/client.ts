// The data contract between views and the backend.
//
// Two transports exist for the same Go façade (`api.Library`): the desktop app
// calls generated Wails bindings, the LAN web client speaks JSON over HTTP.
// Views must not care which one they are running on, so they call `client.*`
// and the entry module decides what sits behind it:
//
//   main.tsx          configureClient(wailsClient)
//   mobile/main.tsx   configureClient(httpClient)
//
// Only capabilities that *both* clients can perform belong here. Anything a
// browser cannot do (file pickers, cover frame selection, cloud drive setup,
// server control, launching a local player) stays in the platform module and is
// imported directly by platform code — see
// docs/spec/cross-platform-ui-plan.md §3.
import type {
  ActorProfile,
  ActorSummary,
  BackgroundTasks,
  ContentDuplicateGroup,
  ContentScanStatus,
  JavDetail,
  JavDuplicateGroup,
  JavTask,
  JavTaskOverview,
  SimilarVideo,
  TrashSnapshot,
  Video,
} from "../types";

export interface MuseClient {
  // ---- library reads ----
  listVideos(): Promise<Video[]>;
  searchVideos(keyword: string): Promise<Video[]>;
  listActors(): Promise<ActorSummary[]>;
  videosByActor(name: string): Promise<Video[]>;
  /** 详情页底部的「类似影片」：按相似度排序，limit 省略时由后端决定条数。 */
  similarVideos(videoId: number, limit?: number): Promise<SimilarVideo[]>;
  /** 某位演员的档案（头像 / 生日 / 身高三围）；还没补全时为 null。 */
  actorProfile(name: string): Promise<ActorProfile | null>;

  // ---- scrape reads ----
  videoJav(id: number): Promise<JavDetail | null>;
  videoJavTask(id: number): Promise<JavTask | null>;
  listJavTaskOverviews(): Promise<JavTaskOverview[]>;

  // ---- background work ----
  /** 后台任务看板：刮削、片名翻译、内容扫描等任务的进度（只读）。 */
  backgroundTasks(): Promise<BackgroundTasks>;

  // ---- duplicate reads ----
  listJavDuplicates(includeIgnored?: boolean): Promise<JavDuplicateGroup[]>;
  listContentDuplicates(includeIgnored?: boolean): Promise<ContentDuplicateGroup[]>;
  contentScanStatus(): Promise<ContentScanStatus>;

  // ---- trash and archive ----
  listTrash(): Promise<TrashSnapshot>;
  listArchived(): Promise<Video[]>;

  // ---- writes ----
  updateVideoInfo(id: number, title: string, notes: string): Promise<Video>;
  likeVideo(id: number): Promise<Video>;
  clearVideoLikes(id: number): Promise<Video>;
  removeVideo(id: number): Promise<void>;
  restoreVideo(id: number): Promise<void>;
  deleteVideoPermanently(id: number): Promise<void>;
  emptyTrash(): Promise<number>;
  clearArchivedVideos(): Promise<number>;
  ignoreJavDuplicate(key: string): Promise<void>;
  ignoreContentDuplicate(hash: string): Promise<void>;
  reverifyContentDuplicate(hash: string): Promise<void>;
  confirmContentDuplicate(hash: string): Promise<void>;
  scanContentDuplicates(): Promise<ContentScanStatus>;
}

let impl: MuseClient | null = null;

/**
 * Binds the transport for this client. Called once from the entry module before
 * anything renders; a second call replaces the previous binding.
 */
export function configureClient(next: MuseClient): void {
  impl = next;
}

function bound(): MuseClient {
  if (!impl) {
    throw new Error(
      "Muse client 未配置：入口模块必须在渲染前调用 configureClient()。",
    );
  }
  return impl;
}

/**
 * The active transport. Each member is a stable function reference so it can be
 * passed straight to `useLiveQuery` without re-triggering reads on re-render.
 */
export const client: MuseClient = {
  listVideos: () => bound().listVideos(),
  searchVideos: (keyword) => bound().searchVideos(keyword),
  listActors: () => bound().listActors(),
  videosByActor: (name) => bound().videosByActor(name),
  similarVideos: (videoId, limit) => bound().similarVideos(videoId, limit),
  actorProfile: (name) => bound().actorProfile(name),

  videoJav: (id) => bound().videoJav(id),
  videoJavTask: (id) => bound().videoJavTask(id),
  listJavTaskOverviews: () => bound().listJavTaskOverviews(),
  backgroundTasks: () => bound().backgroundTasks(),

  listJavDuplicates: (includeIgnored) => bound().listJavDuplicates(includeIgnored),
  listContentDuplicates: (includeIgnored) => bound().listContentDuplicates(includeIgnored),
  contentScanStatus: () => bound().contentScanStatus(),

  listTrash: () => bound().listTrash(),
  listArchived: () => bound().listArchived(),

  updateVideoInfo: (id, title, notes) => bound().updateVideoInfo(id, title, notes),
  likeVideo: (id) => bound().likeVideo(id),
  clearVideoLikes: (id) => bound().clearVideoLikes(id),
  removeVideo: (id) => bound().removeVideo(id),
  restoreVideo: (id) => bound().restoreVideo(id),
  deleteVideoPermanently: (id) => bound().deleteVideoPermanently(id),
  emptyTrash: () => bound().emptyTrash(),
  clearArchivedVideos: () => bound().clearArchivedVideos(),
  ignoreJavDuplicate: (key) => bound().ignoreJavDuplicate(key),
  ignoreContentDuplicate: (hash) => bound().ignoreContentDuplicate(hash),
  reverifyContentDuplicate: (hash) => bound().reverifyContentDuplicate(hash),
  confirmContentDuplicate: (hash) => bound().confirmContentDuplicate(hash),
  scanContentDuplicates: () => bound().scanContentDuplicates(),
};
