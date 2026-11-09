// Per-device UI zoom, persisted in localStorage like the other display
// preferences. The app is a WebView rather than a browser page, so there is no
// native page-zoom command to call: the whole document is scaled with CSS zoom.
const ZOOM_KEY = "muse.zoom";

export const ZOOM_LEVELS = [0.8, 0.9, 1, 1.1, 1.25, 1.5] as const;
export type ZoomLevel = (typeof ZOOM_LEVELS)[number];
const DEFAULT_ZOOM: ZoomLevel = 1;

function isZoomLevel(value: number): value is ZoomLevel {
  return (ZOOM_LEVELS as readonly number[]).includes(value);
}

export function readZoom(): ZoomLevel {
  try {
    const stored = Number(localStorage.getItem(ZOOM_KEY));
    return isZoomLevel(stored) ? stored : DEFAULT_ZOOM;
  } catch {
    return DEFAULT_ZOOM;
  }
}

export function writeZoom(level: ZoomLevel): void {
  try {
    localStorage.setItem(ZOOM_KEY, String(level));
  } catch {
    /* storage unavailable; the zoom still applies for this session */
  }
}

/**
 * Publishes the zoom to CSS. `style.css` applies it to `html` and divides the
 * document's layout box by the same factor, so the shell fills the window at
 * every level instead of shrinking inside it or clipping at the bottom.
 */
export function applyZoom(level: ZoomLevel): void {
  document.documentElement.style.setProperty("--ui-zoom", String(level));
}

export function stepZoom(current: ZoomLevel, direction: -1 | 1): ZoomLevel {
  const index = ZOOM_LEVELS.indexOf(current);
  const from = index < 0 ? ZOOM_LEVELS.indexOf(DEFAULT_ZOOM) : index;
  const next = Math.min(ZOOM_LEVELS.length - 1, Math.max(0, from + direction));
  return ZOOM_LEVELS[next];
}
