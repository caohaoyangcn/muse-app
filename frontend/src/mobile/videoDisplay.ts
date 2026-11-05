import type { Video } from "../types";
import { formatDuration, formatFileSize, videoResolution } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";

// Small display helpers shared across the mobile views. Kept out of the
// component files so React Fast Refresh can treat those as component-only.

/** The display name: the user's custom title, else the authoritative 番号, else
 *  the file name (the same rule the desktop uses). */
export function videoName(v: Video): string {
  return videoDisplayName(v);
}

/** The one-line metadata strip under a card or row. */
export function videoMeta(v: Video): string {
  return [formatDuration(v.durationMs), videoResolution(v), formatFileSize(v.fileSize)]
    .filter(Boolean)
    .join(" · ");
}
