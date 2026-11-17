import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Page-based rendering for the app's lists.
 *
 * Every list — the desktop wall and compact rows, the phone's poster grid, the
 * actor index, the scrape board, the recycle bin and the duplicate worklists —
 * renders exactly one page and replaces it when the user turns the page. The
 * page number, not a measured height or an ever-growing "load more" count, is
 * the only rendering state, so the DOM stays bounded and the same contract
 * holds on both shells.
 *
 * Two things are easy to get subtly wrong and are encoded here:
 *
 * - Depth has to survive leaving the screen. Opening a video from page 4 and
 *   going back must land on page 4, so depth is remembered per list identity
 *   (filters, sort, keyword) in a small LRU map rather than in component state.
 * - The first render of a page often has no data yet (`total` 0), which clamps
 *   the page to 1. That clamp must never overwrite the remembered depth, or the
 *   restore is lost on the very visit that needed it. Depth is therefore only
 *   written on an explicit page change, never on mount or a clamp.
 */

/** Rows per page when a list does not choose its own density. */
export const DEFAULT_PAGE_SIZE = 50;

/** How many list identities keep their depth; the least recent is dropped. */
const PAGE_MEMORY_LIMIT = 64;

const PAGE_MEMORY = new Map<string, number>();

function rememberedPage(key: string): number | undefined {
  return PAGE_MEMORY.get(key);
}

function rememberPage(key: string, page: number): void {
  // Re-inserting keeps the map ordered by recent use, so the trim drops the
  // list the user has been away from longest.
  PAGE_MEMORY.delete(key);
  PAGE_MEMORY.set(key, page);
  while (PAGE_MEMORY.size > PAGE_MEMORY_LIMIT) {
    const oldest = PAGE_MEMORY.keys().next().value;
    if (oldest === undefined) break;
    PAGE_MEMORY.delete(oldest);
  }
}

export interface PagedListOptions {
  /** Items in the list after sorting and filtering. */
  total: number;
  /** Identity of the list (filters, sort, keyword), used for the depth memory. */
  listKey: string;
  /** Items rendered per page. */
  pageSize?: number;
  /** Item index a keyboard selection just moved to. When it leaves the current
   *  page, the page follows it; a manual page change never does. */
  revealIndex?: number | null;
}

export interface PagedListState {
  /** Current page, 1-based and always within `[1, pageCount]`. */
  page: number;
  pageCount: number;
  /** Inclusive start / exclusive end of the items to render. */
  startIndex: number;
  endIndex: number;
  hasPrev: boolean;
  hasNext: boolean;
  /** Jump to a page (clamped). */
  goToPage: (page: number) => void;
  prevPage: () => void;
  nextPage: () => void;
}

function pageCountOf(total: number, pageSize: number): number {
  return Math.max(1, Math.ceil(Math.max(0, total) / pageSize));
}

function clampPage(page: number, pageCount: number): number {
  return Math.min(Math.max(1, page), pageCount);
}

export function usePagedList({
  total,
  listKey,
  pageSize = DEFAULT_PAGE_SIZE,
  revealIndex = null,
}: PagedListOptions): PagedListState {
  // The key travels with the page, so a list that changes identity can never
  // read (or file) its depth under the wrong one. The remembered page is kept
  // unclamped: a screen that mounts before its data arrives has a page count of
  // 1, and clamping here would permanently lose a depth that is still valid
  // once the rows land. The rendered page is clamped on every read instead.
  const [staged, setStagedState] = useState(() => ({
    key: listKey,
    page: rememberedPage(listKey) ?? 1,
  }));
  // Mirrors let a burst of clicks see the previous click's result even before
  // React re-renders, and let the step maths use the latest total.
  const stagedRef = useRef(staged);
  const totalRef = useRef(total);
  totalRef.current = total;

  const pageCount = pageCountOf(total, pageSize);
  // What the screen should show for the list it currently has. A changed key
  // reads its own depth on this very render — no effect, so no frame showing
  // the previous list's page.
  const page =
    staged.key === listKey
      ? clampPage(staged.page, pageCount)
      : clampPage(rememberedPage(listKey) ?? 1, pageCount);

  const commit = useCallback(
    (target: number) => {
      const max = pageCountOf(totalRef.current, pageSize);
      const current = stagedRef.current;
      const from =
        current.key === listKey
          ? clampPage(current.page, max)
          : clampPage(rememberedPage(listKey) ?? 1, max);
      const next = clampPage(target, max);
      if (next === from && current.key === listKey) return;

      const stagedNext = { key: listKey, page: next };
      stagedRef.current = stagedNext;
      // Only an explicit page change is worth remembering (see the header).
      rememberPage(listKey, next);
      setStagedState(stagedNext);
    },
    [listKey, pageSize],
  );

  const goToPage = useCallback((target: number) => commit(target), [commit]);

  const prevPage = useCallback(() => {
    const current = stagedRef.current;
    const max = pageCountOf(totalRef.current, pageSize);
    const from =
      current.key === listKey
        ? clampPage(current.page, max)
        : clampPage(rememberedPage(listKey) ?? 1, max);
    commit(from - 1);
  }, [commit, listKey, pageSize]);

  const nextPage = useCallback(() => {
    const current = stagedRef.current;
    const max = pageCountOf(totalRef.current, pageSize);
    const from =
      current.key === listKey
        ? clampPage(current.page, max)
        : clampPage(rememberedPage(listKey) ?? 1, max);
    commit(from + 1);
  }, [commit, listKey, pageSize]);

  // Follow a keyboard selection that moved off the current page. The initial
  // value is recorded so mounting never overrides the restored page, and a
  // manual page change does not re-trigger because the index itself did not
  // change.
  const revealed = useRef<number | null>(revealIndex);
  useEffect(() => {
    if (revealIndex == null) {
      revealed.current = null;
      return;
    }
    if (revealIndex === revealed.current) return;
    revealed.current = revealIndex;
    if (revealIndex < 0 || revealIndex >= totalRef.current) return;
    const target = clampPage(Math.floor(revealIndex / pageSize) + 1, pageCountOf(totalRef.current, pageSize));
    if (target !== page) commit(target);
  }, [revealIndex, pageSize, page, commit]);

  const startIndex = (page - 1) * pageSize;
  return {
    page,
    pageCount,
    startIndex,
    endIndex: Math.min(total, startIndex + pageSize),
    hasPrev: page > 1,
    hasNext: page < pageCount,
    goToPage,
    prevPage,
    nextPage,
  };
}
