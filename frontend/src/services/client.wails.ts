// MuseClient implementation for the desktop app: every call forwards to the
// generated Wails bindings through services/api.ts.
//
// Imported only by the desktop entry (src/main.tsx), so the Wails-specific
// module graph never reaches the LAN web bundle.
import type { MuseClient } from "./client";
import * as api from "./api";

export const wailsClient: MuseClient = {
  listVideos: () => api.listVideos(),
  searchVideos: (keyword) => api.searchVideos(keyword),
  listGroups: () => api.listGroups(),
  listActors: () => api.listActors(),
  videosByActor: (name) => api.videosByActor(name),

  videoJav: (id) => api.videoJav(id),
  videoJavTask: (id) => api.videoJavTask(id),
  listJavTaskOverviews: () => api.listJavTaskOverviews(),

  listJavDuplicates: (includeIgnored) => api.listJavDuplicates(includeIgnored),
  listContentDuplicates: (includeIgnored) => api.listContentDuplicates(includeIgnored),
  contentScanStatus: () => api.contentScanStatus(),

  listTrash: () => api.listTrash(),
  listArchived: () => api.listArchived(),

  updateVideoInfo: (id, title, notes) => api.updateVideoInfo(id, title, notes),
  likeVideo: (id) => api.likeVideo(id),
  clearVideoLikes: (id) => api.clearVideoLikes(id),
  removeVideo: (id) => api.removeVideo(id),
  restoreVideo: (id) => api.restoreVideo(id),
  deleteVideoPermanently: (id) => api.deleteVideoPermanently(id),
  emptyTrash: () => api.emptyTrash(),
  clearArchivedVideos: () => api.clearArchivedVideos(),
  ignoreJavDuplicate: (key) => api.ignoreJavDuplicate(key),
  ignoreContentDuplicate: (hash) => api.ignoreContentDuplicate(hash),
  reverifyContentDuplicate: (hash) => api.reverifyContentDuplicate(hash),
  confirmContentDuplicate: (hash) => api.confirmContentDuplicate(hash),
  scanContentDuplicates: () => api.scanContentDuplicates(),
};
