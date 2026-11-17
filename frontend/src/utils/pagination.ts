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
