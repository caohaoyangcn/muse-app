import type { Video } from "../types";
import { MIME_VIDEO_DRAG, MIME_VIDEO_DRAG_MULTI } from "../types";
import { videoDisplayName } from "./videoName";

/**
 * Start a native drag for a video card/row.
 *
 * The id rides a dedicated MIME type so drop targets (sidebar groups) can tell
 * it apart from a group-reorder drag and treat it as a copy rather than a move.
 * When more than one card is selected and the dragged card is part of that
 * selection, the whole selection travels together via MIME_VIDEO_DRAG_MULTI so
 * a single drop batches them.
 *
 * A custom drag ghost (rounded chip with a 🎦 icon and the item label) replaces
 * the browser's default semi-transparent snapshot; a multi-selection shows the
 * batch count instead of one file name.
 */
export function beginVideoDrag(
  e: React.DragEvent,
  video: Video,
  selectedIds: Set<number>,
): void {
  e.dataTransfer.effectAllowed = "copy";
  const multi = selectedIds.size > 1 && selectedIds.has(video.id);
  if (multi) {
    e.dataTransfer.setData(MIME_VIDEO_DRAG_MULTI, Array.from(selectedIds).join(","));
    e.dataTransfer.setData("text/plain", `${selectedIds.size} 个视频`);
  } else {
    e.dataTransfer.setData(MIME_VIDEO_DRAG, String(video.id));
    // text/plain fallback so other contexts (e.g. external) see something.
    e.dataTransfer.setData("text/plain", videoDisplayName(video));
  }

  const ghost = document.createElement("div");
  ghost.style.cssText =
    "position:fixed;left:-10000px;top:-10000px;pointer-events:none;" +
    "display:flex;flex-direction:column;align-items:center;width:112px;border-radius:10px;" +
    "overflow:hidden;background:rgba(30,30,36,0.96);border:1px solid rgba(124,92,255,0.6);" +
    "box-shadow:0 6px 20px rgba(0,0,0,0.45);font-family:system-ui,sans-serif;";
  const icon = document.createElement("div");
  icon.textContent = "🎦";
  icon.style.cssText =
    "font-size:38px;line-height:1;padding:14px 0 10px;" +
    "width:100%;text-align:center;background:rgba(124,92,255,0.14);";
  const label = document.createElement("div");
  label.textContent = multi ? `${selectedIds.size} 个视频` : videoDisplayName(video);
  label.style.cssText =
    "width:100%;padding:7px 8px;text-align:center;font-size:11px;color:#e8e8ee;" +
    "white-space:nowrap;overflow:hidden;text-overflow:ellipsis;";
  ghost.append(icon, label);
  document.body.appendChild(ghost);
  // Offset the ghost so the pointer sits near the middle of it.
  e.dataTransfer.setDragImage(ghost, ghost.offsetWidth / 2, 0);
  window.addEventListener("dragend", () => ghost.remove(), { once: true });
}
