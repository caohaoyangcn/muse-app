import type { CardSize, SortDir, SortKey, ViewMode } from "../types";
import { readStoredCardSize, writeStoredCardSize, type CardSizeRange } from "../utils/cardSize";

// Mobile display preferences. They live in localStorage (not the backend
// settings store) because they are per-device UI choices; the desktop app
// keeps its own keys, so the two clients tune their density independently.
const SORT_KEY_STORAGE = "muse.mobile.sortKey";
const SORT_DIR_STORAGE = "muse.mobile.sortDir";
const VIEW_MODE_STORAGE = "muse.mobile.viewMode";
// Kept from the old 小 / 中 / 大 control so an existing preference migrates.
const CARD_SIZE_STORAGE = "muse.mobile.iconSize";

// The phone grid uses fixed-width tracks, so this is the real card width. The
// default reproduces the old "中" two-column layout on a 375px screen; the max
// is wide enough to yield a single near-full-width card.
export const MOBILE_CARD_SIZE: CardSizeRange = {
  min: 150,
  max: 450,
  defaultValue: 300,
  legacy: { sm: 150, md: 300, lg: 450 },
};

function read<T extends string>(key: string, fallback: T, allowed: readonly T[]): T {
  try {
    const raw = localStorage.getItem(key);
    return raw && (allowed as readonly string[]).includes(raw) ? (raw as T) : fallback;
  } catch {
    return fallback;
  }
}

function write(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Private mode / storage disabled: the preference simply isn't persisted.
  }
}

export const readSortKey = (): SortKey =>
  read<SortKey>(SORT_KEY_STORAGE, "createdAt", [
    "createdAt",
    "lastOpenedAt",
    "fileSize",
    "fileName",
    "likeCount",
  ]);
export const writeSortKey = (k: SortKey) => write(SORT_KEY_STORAGE, k);

export const readSortDir = (): SortDir => read<SortDir>(SORT_DIR_STORAGE, "desc", ["asc", "desc"]);
export const writeSortDir = (d: SortDir) => write(SORT_DIR_STORAGE, d);

export const readViewMode = (): ViewMode => read<ViewMode>(VIEW_MODE_STORAGE, "grid", ["grid", "list"]);
export const writeViewMode = (m: ViewMode) => write(VIEW_MODE_STORAGE, m);

export const readCardSize = (): CardSize => readStoredCardSize(CARD_SIZE_STORAGE, MOBILE_CARD_SIZE);
export const writeCardSize = (s: CardSize) => writeStoredCardSize(CARD_SIZE_STORAGE, s, MOBILE_CARD_SIZE);
