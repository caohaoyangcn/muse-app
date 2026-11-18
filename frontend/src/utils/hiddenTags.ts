import type { ValueOption } from "./libraryFilter";

/**
 * Tags the user removed from the sidebar's quick list. This is a per-device UI
 * preference, not a library fact: hiding a tag never changes which videos it
 * belongs to, and the tag stays available in the full filter dialog.
 *
 * The list itself is persisted by the backend settings store (not localStorage),
 * so rebuilding the executable or resetting a webview profile cannot bring
 * every hidden tag back.
 */

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
