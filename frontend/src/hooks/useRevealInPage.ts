import { useEffect, useRef, type RefObject } from "react";

/**
 * Scrolls a keyboard-selected item into view once its page is mounted.
 *
 * The desktop grid and list let the arrow keys move the single selection across
 * the whole (paged) list. `usePagedList` turns the page when the selection moves
 * off the current one; this hook waits for that item to appear and then brings
 * it into the viewport. It acts only on a change of `revealIndex`, so manually
 * turning a page never yanks the pane to a selection left on another page.
 */
export function useRevealInPage({
  listRef,
  items,
  startIndex,
  endIndex,
  revealIndex,
}: {
  /** The rendered page's wrapper, searched for `data-video-id`. */
  listRef: RefObject<HTMLElement | null>;
  /** The full list, indexed by `revealIndex`. */
  items: readonly { id: number }[];
  startIndex: number;
  endIndex: number;
  revealIndex: number | null;
}): void {
  const pending = useRef<number | null>(null);
  const firstRun = useRef(true);

  useEffect(() => {
    if (firstRun.current) {
      firstRun.current = false;
      return;
    }
    if (revealIndex != null) pending.current = revealIndex;
  }, [revealIndex]);

  useEffect(() => {
    const target = pending.current;
    if (target == null || target < startIndex || target >= endIndex) return;
    const item = items[target];
    if (!item) return;
    const element = listRef.current?.querySelector<HTMLElement>(`[data-video-id="${item.id}"]`);
    if (!element) return;
    pending.current = null;
    element.scrollIntoView({ block: "nearest" });
  }, [listRef, items, startIndex, endIndex]);
}
