/**
 * The page numbers a pager offers as shortcuts.
 *
 * The first page, the current page's five neighbours on each side, and the last
 * page. Duplicates collapse, so a short list or a page near either end does not
 * repeat a number; the range is clamped rather than padded. This is deliberately
 * not a "page 1 … 5 6 7 … last" model: the shortcut strip is a fixed-size window
 * the user can learn, and the first/last entries are always reachable.
 */
export function pageShortcuts(page: number, pageCount: number): number[] {
  const last = Math.max(1, pageCount);
  const current = Math.min(Math.max(1, page), last);
  const pages = new Set<number>([1]);
  for (let p = Math.max(1, current - 5); p <= Math.min(last, current + 5); p += 1) {
    pages.add(p);
  }
  pages.add(last);
  return [...pages].sort((a, b) => a - b);
}

/** One entry per gap or page number in the shortcut strip. */
export type PageShortcut = number | "gap";

/**
 * The shortcut strip with the skipped ranges made visible.
 *
 * The pager cannot draw a button for every page, so a jump from 1 to 5 leaves a
 * silent hole. Inserting a decorative mark there is what tells the reader that
 * pages 2–4 exist but are not offered; consecutive numbers stay adjacent.
 */
export function pageShortcutItems(page: number, pageCount: number): PageShortcut[] {
  const numbers = pageShortcuts(page, pageCount);
  const items: PageShortcut[] = [];
  numbers.forEach((number, index) => {
    if (index > 0 && number > numbers[index - 1] + 1) items.push("gap");
    items.push(number);
  });
  return items;
}
