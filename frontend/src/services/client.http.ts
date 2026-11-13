// MuseClient implementation for the LAN web client: every call forwards to the
// JSON endpoints in services/http.ts.
//
// Imported only by the mobile entry (src/mobile/main.tsx). The naming differs
// from the desktop module by design — both are thin, so the mapping lives here
// rather than in the views.
import type { MuseClient } from "./client";
import * as http from "./http";

export const httpClient: MuseClient = {
  listVideos: () => http.listVideos(),
  searchVideos: (keyword) => http.listVideos(keyword),
  listGroups: () => http.listGroups(),
  listActors: () => http.listActors(),
  videosByActor: (name) => http.videosByActor(name),

  videoJav: (id) => http.javDetail(id),
  videoJavTask: (id) => http.javTask(id),
  listJavTaskOverviews: () => http.listJavTasks(),

  listJavDuplicates: (includeIgnored) => http.listJavDuplicates(includeIgnored),
  listContentDuplicates: (includeIgnored) => http.listContentDuplicates(includeIgnored),
  contentScanStatus: () => http.contentScanStatus(),

  listTrash: () => http.listTrash(),
  listArchived: () => http.listArchived(),

  updateVideoInfo: (id, title, notes) => http.updateVideoInfo(id, title, notes),
  likeVideo: (id) => http.likeVideo(id),
  clearVideoLikes: (id) => http.clearVideoLikes(id),
  removeVideo: (id) => http.removeVideo(id),
  restoreVideo: (id) => http.restoreVideo(id),
  deleteVideoPermanently: (id) => http.deleteVideoPermanently(id),
  emptyTrash: () => http.emptyTrash(),
  clearArchivedVideos: () => http.clearArchivedVideos(),
  ignoreJavDuplicate: (key) => http.ignoreJavDuplicate(key),
  ignoreContentDuplicate: (hash) => http.ignoreContentDuplicate(hash),
  reverifyContentDuplicate: (hash) => http.reverifyContentDuplicate(hash),
  confirmContentDuplicate: (hash) => http.confirmContentDuplicate(hash),
  scanContentDuplicates: () => http.scanContentDuplicates(),
};
