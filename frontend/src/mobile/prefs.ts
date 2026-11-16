import type { SortDir, SortKey, ViewMode } from "../types";

// Mobile display preferences. They live in localStorage (not the backend
// settings store) because they are per-device UI choices; the desktop app
// keeps its own keys, so the two clients tune their density independently.
const SORT_KEY_STORAGE = "muse.mobile.sortKey";
const SORT_DIR_STORAGE = "muse.mobile.sortDir";
const VIEW_MODE_STORAGE = "muse.mobile.viewMode";

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
