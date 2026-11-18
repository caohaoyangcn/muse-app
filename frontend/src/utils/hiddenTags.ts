import type { ValueOption } from "./libraryFilter";

/**
 * Tags the user removed from the sidebar's quick list. This is a per-device UI
 * preference, not a library fact: hiding a tag never changes which videos it
 * belongs to, and the tag stays available in the full filter dialog.
 *
 * Stored as a JSON string array in localStorage so a blocked/full store only
 * loses the preference, never the library.
 */
const HIDDEN_TAGS_STORAGE = "muse.hiddenTags";

/** Trim, drop blanks and deduplicate while keeping the first-seen order. */
export function normalizeHiddenTags(values: readonly string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const value of values) {
    const tag = value.trim();
    if (tag === "" || seen.has(tag)) continue;
    seen.add(tag);
    out.push(tag);
  }
  return out;
}

/** Add a tag to the hidden set, or remove it again. */
export function toggleHiddenTag(hidden: readonly string[], tag: string): string[] {
  return hidden.includes(tag)
    ? hidden.filter((name) => name !== tag)
    : [...hidden, tag];
}

/**
 * Removes hidden options from the quick list. The full filter surface keeps
 * using the unfiltered options, so hiding never makes a tag unfilterable.
 */
export function withoutHiddenTags(
  options: readonly ValueOption[],
  hidden: readonly string[],
): ValueOption[] {
  if (hidden.length === 0) return [...options];
  const hiddenSet = new Set(hidden);
  return options.filter((option) => !hiddenSet.has(option.name));
}

export function readHiddenTags(): string[] {
  try {
    const raw = localStorage.getItem(HIDDEN_TAGS_STORAGE);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return normalizeHiddenTags(parsed.filter((value): value is string => typeof value === "string"));
  } catch {
    return [];
  }
}

export function writeHiddenTags(hidden: readonly string[]): void {
  try {
    localStorage.setItem(HIDDEN_TAGS_STORAGE, JSON.stringify(normalizeHiddenTags(hidden)));
  } catch {
    // Private mode / storage disabled: the choice simply isn't persisted.
  }
}
