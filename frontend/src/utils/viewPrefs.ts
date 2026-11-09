import type { CardSize, SortDir, SortKey, ViewMode } from "../types";
import { readStoredCardSize, writeStoredCardSize, type CardSizeRange } from "./cardSize";

// Library display preferences. They live in localStorage rather than the
// backend settings store: they are per-device UI choices with no server-side
// meaning, so a blocked/full store just falls back to the defaults.

const VIEW_STORAGE = "muse.view";
// Kept from the old 小 / 中 / 大 control so an existing preference migrates.
const CARD_SIZE_STORAGE = "muse.iconSize";
const SORT_KEY_STORAGE = "muse.sortKey";
const SORT_DIR_STORAGE = "muse.sortDir";

// Fixed pixel widths rather than minimum widths: the grid uses fixed tracks,
// so the slider position is the real card width. The default reproduces the
// old "中" card at the default 1100px window (3 columns of ~250px).
export const DESKTOP_CARD_SIZE: CardSizeRange = {
  min: 300,
  max: 750,
  defaultValue: 500,
  legacy: { sm: 300, md: 500, lg: 700 },
};

/** Read a stored preference, ignoring unknown or missing values. */
function readStored<T extends string>(key: string, fallback: T, allowed: readonly T[]): T {
  try {
    const raw = localStorage.getItem(key);
    return raw && (allowed as readonly string[]).includes(raw) ? (raw as T) : fallback;
  } catch {
    return fallback;
  }
}

function writeStored(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* storage unavailable; the choice just won't persist */
  }
}

export const readViewMode = (): ViewMode => readStored<ViewMode>(VIEW_STORAGE, "grid", ["grid", "list"]);
export const writeViewMode = (v: ViewMode) => writeStored(VIEW_STORAGE, v);

export const readCardSize = (): CardSize => readStoredCardSize(CARD_SIZE_STORAGE, DESKTOP_CARD_SIZE);
export const writeCardSize = (s: CardSize) => writeStoredCardSize(CARD_SIZE_STORAGE, s, DESKTOP_CARD_SIZE);

// Sort is a field plus a direction, stored as two values so a hand-edited or
// half-written store can still fall back per axis instead of losing both.
export const readSortKey = (): SortKey =>
  readStored<SortKey>(SORT_KEY_STORAGE, "createdAt", [
    "createdAt",
    "lastOpenedAt",
    "fileSize",
    "fileName",
    "likeCount",
  ]);
export const writeSortKey = (k: SortKey) => writeStored(SORT_KEY_STORAGE, k);

export const readSortDir = (): SortDir => readStored<SortDir>(SORT_DIR_STORAGE, "desc", ["asc", "desc"]);
export const writeSortDir = (d: SortDir) => writeStored(SORT_DIR_STORAGE, d);
