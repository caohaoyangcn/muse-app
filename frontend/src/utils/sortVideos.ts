import type { SortDir, Video } from "../types";

/**
 * Ordering for the 最近播放 sort. Opened favorites compare by playback time,
 * respecting the selected direction; never-opened favorites always sort after
 * every opened one, so flipping to ascending does not drag a wall of
 * never-played rows to the top. Their block keeps the library's default
 * newest-added order.
 */
export function compareLastOpened(a: Video, b: Video, dir: SortDir): number {
  if (!a.lastOpenedAt && !b.lastOpenedAt) return b.createdAt - a.createdAt || b.id - a.id;
  if (!a.lastOpenedAt) return 1;
  if (!b.lastOpenedAt) return -1;
  const byTime = (a.lastOpenedAt - b.lastOpenedAt) * (dir === "asc" ? 1 : -1);
  return byTime || a.id - b.id;
}
