import {
  AddVideoToGroup,
  AddVideos,
  AddWatchDir,
  AddCloudVideos,
  ArchiveCloudConnection,
  EnableCloudConnection,
  BrowseCloud,
  ClearVideoLikes,
  CloudStorageAvailable,
  DeleteCloudConnection,
  CopyPath,
  CreateGroup,
  DeleteGroup,
  DeleteVideoPermanently,
  EmptyTrash as EmptyTrashBinding,
  EnsureVideoIn4KGroup,
  GetTrashRetentionDays,
  GetServerPort,
  GetScrapeProxy,
  GetTourSeen,
  ListCloudConnections,
  ListArchived,
  ClearArchivedVideos,
  ClearArchivedSource,
  OpenCloudPage,
  SaveCloudConnection,
  TestCloudConnection,
  ListGroups,
  ListVideos,
  ListWatchDirs,
  LikeVideo,
  OpenContainingFolder,
  OpenVideo,
  PickVideoFiles,
  PickWatchDir,
  ResetVideoCover,
  SetVideoScrapedCoverAllowed,
  RefreshStatus,
  PurgeExpiredTrash as PurgeExpiredTrashBinding,
  RemoveVideo,
  RemoveWatchDir,
  RestoreVideo,
  RenameGroup,
  ReorderGroups,
  RescanWatchDirs,
  RefreshVolumes,
  SearchVideos,
  ServerStatus,
  SetGroupParent,
  SetServerPort,
  SetScrapeProxy,
  SetTourSeen,
  SetVideoCover,
  SetVideoGroups,
  StartServer,
  UpdateVideoInfo,
  StopServer,
  SuggestGroupsForVideos,
  VideoGroupIDs,
  VideosInGroup,
  VideoJav,
  VideoJavTask,
  ListJavTaskOverviews,
  ListActors,
  ListTrash as ListTrashBinding,
  ActorProfile as FetchActorProfile,
  ActorFillStatus as FetchActorFillStatus,
  StartActorFill,
  VideosByActor,
  MergeActors,
  UnmergeActor,
  ListJavDuplicates,
  IgnoreJavDuplicate,
  ListContentDuplicates,
  IgnoreContentDuplicate,
  ReverifyContentDuplicate,
  ConfirmContentDuplicate,
  ScanContentDuplicates,
  ContentScanStatus as ContentScanStatusBinding,
  ScanFileStatuses,
  FileScanStatus as FileScanStatusBinding,
  ScrapeVideoJav,
  ScrapeVideosJav,
  SetVideoJavOverride,
  SetTrashRetentionDays,
  ClearJavBusCookie,
  GetJavBusCookieStatus,
  SetJavBusCookie,
  ClearFC2Cookie,
  GetFC2CookieStatus,
  SetFC2Cookie,
  CheckJavCodes,
} from "../../wailsjs/go/main/App";
import type {
  AddVideoResult,
  ActorFillStatus,
  ActorProfile,
  ActorSummary,
  Group,
  ServerStartInfo,
  Video,
  VideoGroupSuggestion,
  WatchedDir,
  JavDetail,
  JavTask,
  JavTaskOverview,
  JavBusCookieStatus,
  FC2CookieStatus,
  CloudConnection,
  CloudConnectionInput,
  CloudEntry,
  CloudTestResult,
  JavDuplicateGroup,
  ContentDuplicateGroup,
  ContentScanStatus,
  FileScanStatus,
  VolumeChange,
  TrashSnapshot,
  JavCodeCheck,
} from "../types";

/**
 * The only place the frontend touches the backend. Everything goes through
 * the generated Wails bindings (spec-v1.md §23).
 */
export const listVideos = (): Promise<Video[]> => ListVideos();

export const searchVideos = (keyword: string): Promise<Video[]> => SearchVideos(keyword);

export const addVideos = (paths: string[]): Promise<AddVideoResult[]> => AddVideos(paths);

/**
 * 把收藏移入回收站（软删除）。记录、缩略图/封面、分组归属与刮削资料都会保留，
 * 可在回收站恢复；源视频文件永远不会被删除。
 */
export const removeVideo = (id: number): Promise<void> => RemoveVideo(id);

/** 回收站条目与当前保留天数。 */
export const listTrash = (): Promise<TrashSnapshot> => ListTrashBinding();

/** 从回收站恢复一条收藏（同 id、同分组、同资料）。 */
export const restoreVideo = (id: number): Promise<void> => RestoreVideo(id);

/** 彻底删除一条回收站收藏：只清数据库记录与生成的图片，不碰源视频。 */
export const deleteVideoPermanently = (id: number): Promise<void> =>
  DeleteVideoPermanently(id);

/** 清空回收站，返回彻底删除的条目数。 */
export const emptyTrash = (): Promise<number> => EmptyTrashBinding();

/** 停用网盘来源并归档其活动收藏，返回归档数量。 */
export const archiveCloudConnection = (id: number): Promise<number> =>
  ArchiveCloudConnection(id);

/** 重新启用网盘来源；不会自动恢复归档条目。 */
export const enableCloudConnection = (id: number): Promise<void> =>
  EnableCloudConnection(id);

/** 长期保留的网盘归档（回收站的 archived 原因）。 */
export const listArchived = (): Promise<Video[]> => ListArchived();

/** 清空全部归档；源网盘文件不会被删除。 */
export const clearArchivedVideos = (): Promise<number> => ClearArchivedVideos();

/** 清空某个来源的归档；源网盘文件不会被删除。 */
export const clearArchivedSource = (sourceId: number): Promise<number> =>
  ClearArchivedSource(sourceId);

/** 立即清理超过保留期的条目，返回清理数量。 */
export const purgeExpiredTrash = (): Promise<number> => PurgeExpiredTrashBinding();

/** 当前保留天数（默认 14）。 */
export const getTrashRetentionDays = (): Promise<number> => GetTrashRetentionDays();

/** 保存保留天数；缩短后会立即清理已超期条目。 */
export const setTrashRetentionDays = (days: number): Promise<void> =>
  SetTrashRetentionDays(days);

// ---- Video info editing (编辑影片信息) ----

export const updateVideoInfo = (id: number, title: string, notes: string): Promise<Video> =>
  UpdateVideoInfo(id, title, notes);

export const setVideoCover = (id: number, atSec: number): Promise<Video> =>
  SetVideoCover(id, atSec);

export const resetVideoCover = (id: number): Promise<Video> => ResetVideoCover(id);

export const setVideoScrapedCoverAllowed = (id: number, allowed: boolean): Promise<Video> =>
  SetVideoScrapedCoverAllowed(id, allowed);

export const videoJav = (id: number): Promise<JavDetail> => VideoJav(id);
export const videoJavTask = (id: number): Promise<JavTask> => VideoJavTask(id);
export const listJavTaskOverviews = (): Promise<JavTaskOverview[]> => ListJavTaskOverviews();

/** 演员索引：刮削结果中聚合出的演员及其出演影片数。 */
export const listActors = (): Promise<ActorSummary[]> => ListActors();

/** 某个演员出演过的全部影片（含已合并的艺名，按收藏时间倒序）。 */
export const videosByActor = (name: string): Promise<Video[]> => VideosByActor(name);

/** 某位演员的档案（头像 / 生日 / 身高三围）。还没补全时返回 null。 */
export const actorProfile = (name: string): Promise<ActorProfile | null> =>
  FetchActorProfile(name) as Promise<ActorProfile | null>;

/** 为还没有档案的演员排队「补全演员信息」，返回本次新加入队列的人数。 */
export const startActorFill = (): Promise<number> => StartActorFill();

/** 「补全演员信息」的进度。 */
export const actorFillStatus = (): Promise<ActorFillStatus> =>
  FetchActorFillStatus() as Promise<ActorFillStatus>;

/** 合并两个演员（同一演员的两个艺名）：source → target。 */
export const mergeActors = (source: string, target: string): Promise<void> =>
  MergeActors(source, target);

/** 解除某艺名的合并关系，使其恢复为独立演员。 */
export const unmergeActor = (alias: string): Promise<void> => UnmergeActor(alias);

/**
 * 查重：识别出同一番号被收藏了多个版本的列表。带上下集标记的文件（如
 * abc-123-1 / abc-123-2）视为不同作品，不会出现在这里。默认隐藏已「保留两者」
 * 处理过的分组。
 */
export const listJavDuplicates = (includeIgnored = false): Promise<JavDuplicateGroup[]> =>
  ListJavDuplicates(includeIgnored);

/** 保留重复分组的全部版本（等价于忽略重复提示）。 */
export const ignoreJavDuplicate = (key: string): Promise<void> => IgnoreJavDuplicate(key);

/**
 * 内容重复：同一份字节被收藏了两份（改名、换目录、跨盘备份）。判据是文件内容指纹，
 * 与番号无关，所以没有可靠番号的文件也能被识别；反过来，同一作品的不同压制不会被
 * 判为重复。默认隐藏已「保留全部」处理过的分组。
 */
export const listContentDuplicates = (
  includeIgnored = false,
): Promise<ContentDuplicateGroup[]> => ListContentDuplicates(includeIgnored);

/** 保留内容重复分组的全部拷贝（等价于不再提示这组文件）。 */
export const ignoreContentDuplicate = (hash: string): Promise<void> =>
  IgnoreContentDuplicate(hash);

/** 重新读取一组文件的内容指纹：成员在应用之外被改动后，用它让结论重新可信。 */
export const reverifyContentDuplicate = (hash: string): Promise<void> =>
  ReverifyContentDuplicate(hash);

/**
 * 完整校验：逐字节读取一组文件并比较完整 SHA-256。部分指纹只读首尾各 1 MiB，
 * 这一步才会读整个文件（5 GB 的文件在 USB 机械盘上约需 40 秒），所以只在用户显式
 * 要求时执行，后台一次读一个文件。
 */
export const confirmContentDuplicate = (hash: string): Promise<void> =>
  ConfirmContentDuplicate(hash);

/** 扫描整库的内容指纹（后台执行，状态通过 contentScanStatus 或事件获取）。 */
export const scanContentDuplicates = (): Promise<ContentScanStatus> =>
  ScanContentDuplicates();

/** 内容指纹扫描的当前（或最近一次）状态。 */
export const contentScanStatus = (): Promise<ContentScanStatus> =>
  ContentScanStatusBinding();
export const scrapeVideoJav = (id: number, force = false): Promise<void> => ScrapeVideoJav(id, force);
export const scrapeVideosJav = (force = false): Promise<number> => ScrapeVideosJav(force);
export const setVideoJavOverride = (id: number, mode: string, code: string): Promise<void> =>
  SetVideoJavOverride(id, mode, code);

/** 给视频点赞：可多次点赞，计数累加。 */
export const likeVideo = (id: number): Promise<Video> => LikeVideo(id);

/** 清空某视频的点赞数（编辑信息窗口内）。 */
export const clearVideoLikes = (id: number): Promise<Video> => ClearVideoLikes(id);

export const openVideo = (id: number): Promise<void> => OpenVideo(id);

export const openContainingFolder = (id: number): Promise<void> => OpenContainingFolder(id);

export const copyPath = (id: number): Promise<void> => CopyPath(id);

export const refreshStatus = (id: number): Promise<Video> => RefreshStatus(id);

/**
 * Re-observe which disks the favorites live on (is the external drive plugged
 * in?). Called once after the library mounts: the read model only carries what
 * the last observation saw, and this is what makes the first screen right.
 */
export const refreshVolumes = (): Promise<VolumeChange[]> => RefreshVolumes();

/**
 * 检测文件：手动重跑启动时执行的“文件是否存在”检查，不必重启应用。后台执行，
 * 进度与结果通过 fileScanStatus 或 video:filescan 事件获取。
 */
export const scanFileStatuses = (): Promise<FileScanStatus> => ScanFileStatuses();

/** 文件检测的当前（或最近一次）状态，包含启动时那次检查的结果。 */
export const fileScanStatus = (): Promise<FileScanStatus> => FileScanStatusBinding();

export const pickVideoFiles = (): Promise<string[] | null> => PickVideoFiles();

// ---- Grouping API (分组) ----

export const listGroups = (): Promise<Group[]> => ListGroups();

export const createGroup = (name: string): Promise<Group> => CreateGroup(name);

export const renameGroup = (id: number, name: string): Promise<Group> => RenameGroup(id, name);

export const deleteGroup = (id: number): Promise<void> => DeleteGroup(id);

export const reorderGroups = (ids: number[]): Promise<void> => ReorderGroups(ids);

export const setGroupParent = (id: number, parentId: number | null): Promise<Group> =>
  SetGroupParent(id, parentId);

export const videoGroupIDs = (videoId: number): Promise<number[]> => VideoGroupIDs(videoId);

export const setVideoGroups = (videoId: number, groupIds: number[]): Promise<void> =>
  SetVideoGroups(videoId, groupIds);

export const addVideoToGroup = (videoId: number, groupId: number): Promise<void> =>
  AddVideoToGroup(videoId, groupId);

export const videosInGroup = (groupId: number): Promise<number[]> => VideosInGroup(groupId);

export const suggestGroupsForVideos = (paths: string[]): Promise<VideoGroupSuggestion[]> =>
  SuggestGroupsForVideos(paths);

export const ensureVideoIn4KGroup = (videoId: number): Promise<void> =>
  EnsureVideoIn4KGroup(videoId);

// ---- Server mode (局域网访问) ----

export const startServer = (port: number): Promise<ServerStartInfo> => StartServer(port);

export const stopServer = (): Promise<void> => StopServer();

export const serverStatus = (): Promise<ServerStartInfo> => ServerStatus();

// ---- Settings (设置) ----
export const getServerPort = (): Promise<number> => GetServerPort();

export const setServerPort = (port: number): Promise<void> => SetServerPort(port);

/** 刮削代理地址；空字符串表示直连（仍会读取 HTTP_PROXY/HTTPS_PROXY 环境变量）。 */
export const getScrapeProxy = (): Promise<string> => GetScrapeProxy();

export const setScrapeProxy = (url: string): Promise<void> => SetScrapeProxy(url);

export const getJavBusCookieStatus = (): Promise<JavBusCookieStatus> =>
  GetJavBusCookieStatus();

export const setJavBusCookie = (cookie: string): Promise<void> =>
  SetJavBusCookie(cookie);

export const clearJavBusCookie = (): Promise<void> =>
  ClearJavBusCookie();

export const getFC2CookieStatus = (): Promise<FC2CookieStatus> => GetFC2CookieStatus();

export const setFC2Cookie = (cookie: string): Promise<void> => SetFC2Cookie(cookie);

export const clearFC2Cookie = (): Promise<void> => ClearFC2Cookie();

/**
 * 浏览器扩展查询用：某个番号（JavDB 页面渲染出来的片号）是否已在收藏库中。
 * 与 HTTP 端点 `/api/extension/check` 共用同一个门面方法，所以设置页的「测试连接」
 * 验证的就是扩展真正会走到的那条路径。
 */
export const checkJavCodes = (codes: string[]): Promise<JavCodeCheck[]> =>
  CheckJavCodes(codes);

/** 新手引导是否已经播放过（存在应用数据库里，不随 WebView 配置丢失）。 */
export const getTourSeen = (): Promise<boolean> => GetTourSeen();

export const setTourSeen = (): Promise<void> => SetTourSeen();

// ---- Folder watch (目录监控) ----

export const listWatchDirs = (): Promise<WatchedDir[]> => ListWatchDirs();

export const addWatchDir = (path: string): Promise<WatchedDir> => AddWatchDir(path);

export const removeWatchDir = (id: number): Promise<void> => RemoveWatchDir(id);

export const pickWatchDir = (): Promise<string> => PickWatchDir();

export const rescanWatchDirs = (): Promise<void> => RescanWatchDirs();

// ---- Cloud sources (WebDAV 网盘) ----

/** 已配置的网盘连接。此读取不访问网络，只返回最后一次观测到的在线状态。 */
export const listCloudConnections = (): Promise<CloudConnection[]> => ListCloudConnections();

export const saveCloudConnection = (
  input: CloudConnectionInput,
): Promise<CloudConnection> => SaveCloudConnection(input);

export const deleteCloudConnection = (id: number): Promise<void> => DeleteCloudConnection(id);

/** 测试地址与凭据；预期失败通过 result.error 返回，而不是抛异常。 */
export const testCloudConnection = (
  input: CloudConnectionInput,
): Promise<CloudTestResult> => TestCloudConnection(input);

/** 浏览一个远程目录（目录在前，文件按名称排序）。 */
export const browseCloud = (id: number, path: string): Promise<CloudEntry[]> =>
  BrowseCloud(id, path);

/** 把选中的远程文件加入收藏。 */
export const addCloudVideos = (
  sourceId: number,
  paths: string[],
): Promise<AddVideoResult[]> => AddCloudVideos(sourceId, paths);

/** 当前平台是否能安全保存密码（Windows DPAPI）。 */
export const cloudStorageAvailable = (): Promise<boolean> => CloudStorageAvailable();

/** 远程收藏的管理页（OpenList）。Muse 只打开网页，不在内部做写操作。 */
export const openCloudPage = (videoId: number): Promise<void> => OpenCloudPage(videoId);
