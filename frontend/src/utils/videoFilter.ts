import type { Video } from "../types";

/**
 * In-place keyword filter for an already-loaded video list (a group's films, or
 * one actor's films). The desktop library header search stays a whole-library
 * query; this is the narrower "narrow what is in front of me" input, so it runs
 * client-side over the fully loaded list and needs no debounce.
 *
 * The fields mirror the backend's `SearchVideos` LIKE (`file_name`, `path`) and
 * add the user's own title and the authoritative 番号, so the local filter can
 * never hide a label the user can see.
 */
export function videoMatchesKeyword(video: Video, keyword: string): boolean {
  const needle = keyword.trim().toLowerCase();
  if (!needle) return true;
  if (video.title && video.title.toLowerCase().includes(needle)) return true;
  if (video.javCode && video.javCode.toLowerCase().includes(needle)) return true;
  if (video.fileName.toLowerCase().includes(needle)) return true;
  return video.path.toLowerCase().includes(needle);
}

/** `filterVideosByKeyword(videos, "")` returns the original array untouched. */
export function filterVideosByKeyword(videos: Video[], keyword: string): Video[] {
  if (!keyword.trim()) return videos;
  return videos.filter((v) => videoMatchesKeyword(v, keyword));
}
