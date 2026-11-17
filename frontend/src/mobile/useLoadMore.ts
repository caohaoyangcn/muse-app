import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Incremental rendering for the phone's video lists.
 *
 * The desktop windows its grid by measurement (`useVirtualRows`): the rows near
 * the viewport are mounted and the rest of the height is held by an absolutely
 * positioned grid. That keeps a library of a thousand favorites cheap, but on a
 * phone it leans on two things that are not dependable there — layout metrics
 * measured back out of the DOM, and an absolutely positioned list inside the
 * touch scroller. iOS in particular does not paint absolute descendants reliably
 * far down a momentum-scrolled pane, which reached the user as a list that
 * simply stopped after the first screens: nothing new below, and no control to
 * ask for more.
 *
 * The phone appends in pages instead. The first page is laid out normally, the
 * next one loads when the footer comes within a screen of the viewport, and the
 * footer's 加载更多 button is the control that always works even if scrolling
 * alone does not. Nothing is measured, nothing is absolutely positioned, and DOM
 * growth is bounded by how far the user actually pages.
 */

/** Items per page: about eight phone screens of posters, or a dozen of rows. */
export const PAGE_SIZE = 60;

/** How far before the footer (px) the next page starts loading — about one
 *  screen, so a fast flick never reaches the end of the mounted list. */
const PRELOAD_PX = 900;

/**
 * How deep each list was paged, keyed by its identity (filters, sort, keyword).
 * The shell's scroll memory can only restore an offset the pane is tall enough
 * for, so without this a return from a detail screen would show the first page
 * in a short pane and lose the position the user came back to.
 */
const PAGE_MEMORY = new Map<string, number>();
/** Only the screens the user is hopping between need their depth remembered. */
const PAGE_MEMORY_LIMIT = 16;

function rememberedPage(key: string, pageSize: number): number {
  const stored = PAGE_MEMORY.get(key);
  return stored !== undefined && stored > pageSize ? stored : pageSize;
}

function rememberPage(key: string, limit: number): void {
  // Re-inserting keeps the map ordered by recent use, so the trim drops the
  // list the user has been away from longest.
  PAGE_MEMORY.delete(key);
  PAGE_MEMORY.set(key, limit);
  while (PAGE_MEMORY.size > PAGE_MEMORY_LIMIT) {
    const oldest = PAGE_MEMORY.keys().next().value;
    if (oldest === undefined) break;
    PAGE_MEMORY.delete(oldest);
  }
}

export interface LoadMoreState {
  /** How many items to render: the loaded pages, never more than `total`. */
  visibleCount: number;
  /** True while some of the list is still behind the button. */
  hasMore: boolean;
  loadMore: () => void;
  /** Attach to the footer element: reaching it loads the next page. */
  sentinelRef: React.RefObject<HTMLDivElement | null>;
}

export function useLoadMore({
  total,
  memoryKey,
  pageSize = PAGE_SIZE,
}: {
  total: number;
  /** Identity of the list being paged (filters, sort and keyword). */
  memoryKey: string;
  pageSize?: number;
}): LoadMoreState {
  // The key travels with the limit, so a list that changes identity can never
  // read (or file) its depth under the wrong one.
  const [paged, setPaged] = useState(() => ({
    key: memoryKey,
    limit: rememberedPage(memoryKey, pageSize),
  }));
  const sentinelRef = useRef<HTMLDivElement | null>(null);

  // A changed identity reads its own remembered depth on this very render —
  // no effect and therefore no frame showing the previous list's page count.
  const limit = paged.key === memoryKey ? paged.limit : rememberedPage(memoryKey, pageSize);
  const visibleCount = Math.min(limit, total);
  const hasMore = visibleCount < total;

  // Mirrors for the observer, which is created once per list and must act on the
  // current answer rather than the one it closed over.
  const hasMoreRef = useRef(hasMore);
  const totalRef = useRef(total);
  useEffect(() => {
    hasMoreRef.current = hasMore;
  }, [hasMore]);
  useEffect(() => {
    totalRef.current = total;
  }, [total]);

  const loadMore = useCallback(() => {
    if (!hasMoreRef.current) return;
    setPaged((current) => {
      const from = current.key === memoryKey ? current.limit : rememberedPage(memoryKey, pageSize);
      const next = Math.min(from + pageSize, totalRef.current);
      return next === from ? current : { key: memoryKey, limit: next };
    });
  }, [memoryKey, pageSize]);

  // Keep the depth for the scroll memory described on PAGE_MEMORY. `limit` and
  // `memoryKey` are always the pair for the list on screen, so this cannot file
  // one list's depth under another.
  useEffect(() => {
    if (limit > pageSize) rememberPage(memoryKey, limit);
  }, [limit, pageSize, memoryKey]);

  // Load the next page as the footer approaches the viewport. Reachability is
  // measured by IntersectionObserver rather than by scroll events, so it behaves
  // the same during an iOS momentum fling, where scroll callbacks are throttled.
  // The observer is rebuilt whenever the footer can appear or disappear — data
  // arriving after the first render, the last page being loaded — because a
  // sentinel that mounts later would otherwise never be watched.
  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { rootMargin: `${PRELOAD_PX}px 0px` },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasMore, loadMore, total]);

  return { visibleCount, hasMore, loadMore, sentinelRef };
}
