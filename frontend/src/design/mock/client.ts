// Dev-only transport for the design preview.
//
// It implements the same `MuseClient` contract the Wails and HTTP clients do, so
// every view runs unmodified: no `if (preview)` branches leak into components.
// Writes resolve locally and then invoke the real `invalidate()` so the UI takes
// the same refresh path a real backend event would.
import type { MuseClient } from "../../services/client";
import { invalidate } from "../../services/live";
import type { TrashSnapshot, Video } from "../../types";
import {
  ACTORS,
  BACKGROUND,
  CONTENT_DUPLICATES,
  HIGHLIGHTS,
  CONTENT_SCAN_STATUS,
  JAV_DUPLICATES,
  JAV_TASK_OVERVIEWS,
  SIMILAR,
  TRASH_SNAPSHOT,
  VIDEOS,
  javDetailFor,
} from "./data";

let videos: Video[] = VIDEOS.map((video) => ({ ...video }));

function byId(id: number): Video {
  const found = videos.find((video) => video.id === id);
  if (!found) throw new Error(`mock: 没有 id=${id} 的收藏`);
  return found;
}

function patch(id: number, changes: Partial<Video>): Video {
  videos = videos.map((video) =>
    video.id === id ? { ...video, ...changes, updatedAt: Date.now() } : video,
  );
  return byId(id);
}

/** Writes go through here so the preview behaves like a live library. */
function afterWrite<T>(value: T): Promise<T> {
  invalidate();
  return Promise.resolve(value);
}

export const mockClient: MuseClient = {
  listVideos: () => Promise.resolve(videos),
  searchVideos: (keyword) => {
    const needle = keyword.trim().toLowerCase();
    if (!needle) return Promise.resolve(videos);
    return Promise.resolve(
      videos.filter((video) =>
        [video.javCode, video.javTitle, video.javTitleZh, video.title, ...video.actors]
          .join(" ")
          .toLowerCase()
          .includes(needle),
      ),
    );
  },
  listActors: () => Promise.resolve(ACTORS),
  listVideoHighlights: (videoId) => Promise.resolve(HIGHLIGHTS[videoId] ?? []),
  videosByActor: (name) =>
    Promise.resolve(videos.filter((video) => video.actors.includes(name))),
  similarVideos: (videoId) =>
    Promise.resolve(SIMILAR.filter((item) => item.video.id !== videoId).slice(0, 4)),
  actorProfile: (name) =>
    Promise.resolve(ACTORS.find((actor) => actor.name === name)?.profile ?? null),

  videoJav: (id) => Promise.resolve(javDetailFor(byId(id))),
  videoJavTask: (id) =>
    Promise.resolve(
      JAV_TASK_OVERVIEWS.find((task) => task.videoId === id) ?? null,
    ),
  listJavTaskOverviews: () => Promise.resolve(JAV_TASK_OVERVIEWS),
  backgroundTasks: () => Promise.resolve(BACKGROUND),

  listJavDuplicates: () => Promise.resolve(JAV_DUPLICATES),
  listContentDuplicates: () => Promise.resolve(CONTENT_DUPLICATES),
  contentScanStatus: () => Promise.resolve(CONTENT_SCAN_STATUS),

  listTrash: () =>
    Promise.resolve({
      ...TRASH_SNAPSHOT,
      videos: TRASH_SNAPSHOT.videos,
    } as unknown as TrashSnapshot),
  listArchived: () => Promise.resolve([]),

  updateVideoInfo: (id, title, notes) =>
    afterWrite(patch(id, { title, notes })),
  likeVideo: (id) => afterWrite(patch(id, { likeCount: byId(id).likeCount + 1 })),
  clearVideoLikes: (id) => afterWrite(patch(id, { likeCount: 0 })),
  removeVideo: (id) => {
    patch(id, { deletedAt: Date.now() });
    return afterWrite(undefined);
  },
  restoreVideo: (id) => {
    patch(id, { deletedAt: 0 });
    return afterWrite(undefined);
  },
  deleteVideoPermanently: () => afterWrite(undefined),
  emptyTrash: () => afterWrite(0),
  clearArchivedVideos: () => afterWrite(0),
  ignoreJavDuplicate: () => afterWrite(undefined),
  ignoreContentDuplicate: () => afterWrite(undefined),
  reverifyContentDuplicate: () => afterWrite(undefined),
  confirmContentDuplicate: () => afterWrite(undefined),
  scanContentDuplicates: () => afterWrite(CONTENT_SCAN_STATUS),
};
