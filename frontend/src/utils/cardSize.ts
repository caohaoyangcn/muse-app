import type { CardSize } from "../types";

// Card size is the actual card width in CSS pixels. Each client defines its
// own range and default (see viewPrefs / mobile prefs) because a useful width
// on a desktop window is not a useful width on a phone. The stored value is an
// integer so the slider never produces sub-pixel churn.

export interface CardSizeRange {
  min: number;
  max: number;
  defaultValue: CardSize;
  /** Values written by the old 小 / 中 / 大 control, mapped to pixel widths. */
  legacy: Record<"sm" | "md" | "lg", CardSize>;
}

/** Round and clamp a value into the range; non-finite input falls back. */
export function clampCardSize(size: number, range: CardSizeRange): CardSize {
  if (!Number.isFinite(size)) return range.defaultValue;
  return Math.min(range.max, Math.max(range.min, Math.round(size)));
}

/** Where `size` sits in the range, clamped to 0..1. */
export function sizeFraction(size: CardSize, range: CardSizeRange): number {
  const span = range.max - range.min;
  if (span <= 0) return 0;
  return Math.min(1, Math.max(0, (size - range.min) / span));
}

/** Interpolate between `from` and `to` according to the size's range position. */
export function scaleBetween(
  size: CardSize,
  range: CardSizeRange,
  from: number,
  to: number,
): number {
  return from + sizeFraction(size, range) * (to - from);
}

/**
 * Read a persisted card size. Old builds stored "sm" / "md" / "lg"; those map
 * to pixel widths so an existing preference survives the slider change.
 * Missing, malformed or out-of-range values fall back to the default.
 */
export function readStoredCardSize(key: string, range: CardSizeRange): CardSize {
  try {
    const raw = localStorage.getItem(key)?.trim();
    if (!raw) return range.defaultValue;
    if (raw === "sm" || raw === "md" || raw === "lg") return range.legacy[raw];
    const parsed = Number(raw);
    if (!Number.isFinite(parsed)) return range.defaultValue;
    return clampCardSize(parsed, range);
  } catch {
    return range.defaultValue;
  }
}

export function writeStoredCardSize(key: string, size: CardSize, range: CardSizeRange) {
  try {
    localStorage.setItem(key, String(clampCardSize(size, range)));
  } catch {
    /* storage unavailable; the choice just won't persist */
  }
}
