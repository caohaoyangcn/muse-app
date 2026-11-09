import type { Video } from "../types";

const DAY_MS = 24 * 60 * 60 * 1000;

// Elapsed-time windows, not calendar days: the sidebar labels are "1 天内 /
// 3 天内 / 7 天内 / 14 天内", which read naturally as rolling durations.
const BUCKETS = [
  { key: "1d", label: "1 天内访问过", maxAgeMs: DAY_MS },
  { key: "3d", label: "3 天内", maxAgeMs: 3 * DAY_MS },
  { key: "7d", label: "7 天内", maxAgeMs: 7 * DAY_MS },
  { key: "14d", label: "14 天内", maxAgeMs: 14 * DAY_MS },
  { key: "older", label: "更早之前", maxAgeMs: Number.POSITIVE_INFINITY },
] as const;

export type RecentPlayedGroupKey = (typeof BUCKETS)[number]["key"];

export type RecentPlayedGroup = {
  key: RecentPlayedGroupKey;
  label: string;
  videos: Video[];
};

/**
 * Splits a playlist into the 最近播放 age buckets. The input is expected
 * newest-played first (the view sorts that way); each bucket keeps that order.
 * Never-played rows are ignored — they have no place in a playback history and
 * would otherwise land in 更早之前.
 *
 * A future timestamp (clock skew) is treated as age 0, so it stays in the
 * newest bucket instead of disappearing.
 */
export function groupRecentlyPlayed(videos: Video[], now = Date.now()): RecentPlayedGroup[] {
  const groups: RecentPlayedGroup[] = BUCKETS.map((bucket) => ({
    key: bucket.key,
    label: bucket.label,
    videos: [],
  }));

  for (const video of videos) {
    if (video.lastOpenedAt <= 0) continue;
    const age = Math.max(0, now - video.lastOpenedAt);
    const index = BUCKETS.findIndex((bucket) => age < bucket.maxAgeMs);
    groups[index < 0 ? groups.length - 1 : index].videos.push(video);
  }

  return groups.filter((group) => group.videos.length > 0);
}
