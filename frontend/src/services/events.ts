import { EventsOn } from "../../wailsjs/runtime";
import { invalidate } from "./live";

/**
 * Desktop only: the single place that knows which backend events exist.
 *
 * Each one becomes a plain invalidation, so views never subscribe to event
 * names and a new event is wired here rather than in every panel that might
 * care. Progress events that carry their own payload (a content scan's running
 * status, a fingerprint confirmation's counters) stay with the view that draws
 * them; only their "the data changed" half belongs here.
 */
const EVENTS = [
  "thumbnail:generated", // a cover was written
  "jav:scrape:updated", // metadata or the scraped cover changed
  "jav:actors:updated", // a performer's profile was saved, or the backfill advanced
  "video:status:changed", // a file was found missing / present again
  "video:relinked", // a known file was found at a new path
  "video:volume", // a disk was attached or removed
  "video:missing:swept", // missing favorites were reconciled against an attached volume
  "video:filescan:done", // a 文件检测 pass finished: records may have been revived or cleaned up
  "video:liked",
  "video:removed", // a favorite left the library
  "group:changed",
  "watch:scanned", // a folder-watch pass imported or relinked files
];

/** Subscribes every backend event to the live-data revision. */
export function attachBackendEvents(): () => void {
  const detach = EVENTS.map((name) => EventsOn(name, () => invalidate()));
  return () => detach.forEach((off) => off());
}
