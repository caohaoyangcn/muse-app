import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

/**
 * Row windowing for the video grid and list.
 *
 * The library mounts every favorite, so a large collection puts ~10 DOM nodes
 * per video into the layout tree, and every reflow — a window-resize drag, a
 * font or gap change — costs time proportional to that node count. This hook
 * keeps only the rows near the viewport mounted and reserves the rest of the
 * height with a spacer, leaving the scrollbar and scroll position unchanged.
 *
 * Layout metrics are measured from the rendered DOM (item height, gaps,
 * available width) rather than recomputed from the Tailwind classes: `rem`
 * resolves against the root font size, which differs between the desktop
 * (14px) and mobile (16px) client, so the class names alone do not tell us the
 * pixels.
 *
 * Windowing is row-based, so a grid window always begins at a row boundary and
 * its columns line up with the rows above and below it.
 */

/** Pixels rendered above and below the viewport. Without it a fast scroll can
 *  reach a blank strip before the next window commits. */
const OVERSCAN_PX = 600;

/** Lists this short render in full: windowing saves nothing and would only
 *  change their layout. */
const WINDOW_THRESHOLD = 100;

/** Items rendered while the metrics are still unknown. The measuring layout
 *  effect replaces this with the real window before the browser paints, so it
 *  is never visible; it only has to be cheap, contain a measurable item and be
 *  tall enough that the scroller stays scrollable. */
const STARTER_ITEMS = 24;

export interface VirtualRowsOptions {
  /** Items to lay out (already sorted and filtered by the caller). */
  count: number;
  /** Items per row for a fixed layout — a single-column list passes 1. */
  fixedColumns?: number;
  /** Fixed card width in px. When set, the column count follows the
   *  container width using the same rule as `auto-fill` tracks. */
  minColumnWidth?: number;
  /** Re-measure when this changes (the caller passes its current card size). */
  measureKey?: string | number;
  /** Item index to keep mounted. When it changes to something outside the
   *  window, the scroller moves it into view — keyboard navigation selects
   *  items that may never have been rendered. */
  revealIndex?: number | null;
}

export interface VirtualRowsState {
  /** Attach to the element wrapping the grid/list: it carries the full
   *  computed scroll height while windowing. */
  containerRef: React.RefObject<HTMLDivElement | null>;
  /** Attach to the grid/list element itself (the one that lays out items). */
  windowRef: React.RefObject<HTMLDivElement | null>;
  /** Measured columns per row, or 0 while still unknown. */
  columns: number;
  /** Inclusive start / exclusive end of the items to render. */
  startIndex: number;
  endIndex: number;
  /** Offset of the rendered window inside the container, in px. */
  offsetTop: number;
  /** Total height of every row, in px (0 when rendered in full). */
  totalHeight: number;
  /** True once the rows are actually being windowed. */
  windowed: boolean;
  /** Scroll so the item at `index` is vertically centred. Keyboard navigation
   *  needs this because its target may not be mounted. */
  scrollToIndex: (index: number) => void;
}

interface Metrics {
  columns: number;
  itemHeight: number;
  rowGap: number;
  columnGap: number;
}

interface RowRange {
  first: number;
  last: number;
}

const NO_METRICS: Metrics = { columns: 0, itemHeight: 0, rowGap: 0, columnGap: 0 };

/** The nearest scrollable ancestor, by style. Both hosts of the grid are
 *  scrollable: the desktop `<main>` and the mobile `.scroll-area`. */
function findScroller(from: HTMLElement | null): HTMLElement | null {
  let cur = from?.parentElement ?? null;
  while (cur) {
    const style = getComputedStyle(cur);
    const overflow = style.overflowY || style.overflow;
    if (overflow === "auto" || overflow === "scroll") return cur;
    cur = cur.parentElement;
  }
  return null;
}

function px(value: string): number {
  const n = Number.parseFloat(value);
  return Number.isFinite(n) ? n : 0;
}

function sameMetrics(a: Metrics, b: Metrics): boolean {
  return (
    a.columns === b.columns &&
    a.itemHeight === b.itemHeight &&
    a.rowGap === b.rowGap &&
    a.columnGap === b.columnGap
  );
}

export function useVirtualRows({
  count,
  fixedColumns,
  minColumnWidth,
  measureKey,
  revealIndex,
}: VirtualRowsOptions): VirtualRowsState {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const windowRef = useRef<HTMLDivElement | null>(null);
  const scrollerRef = useRef<HTMLElement | null>(null);
  // Offset of the container inside the scroller's content box — the height of
  // whatever sits above the grid (sticky bar, sort bar). Without it the window
  // would be computed against the wrong scroll origin.
  const offsetRef = useRef(0);
  // Mirror of the committed metrics, so event handlers read current values
  // without being re-created on every metric change.
  const metricsRef = useRef<Metrics>(NO_METRICS);

  const [metrics, setMetrics] = useState<Metrics>(NO_METRICS);
  const [rows, setRows] = useState<RowRange | null>(null);

  const rowHeight = metrics.itemHeight > 0 ? metrics.itemHeight + metrics.rowGap : 0;
  const totalRows = metrics.columns > 0 ? Math.ceil(count / metrics.columns) : 0;
  const measurable = rowHeight > 0 && metrics.columns > 0 && totalRows > 0;
  const windowed = count > WINDOW_THRESHOLD && measurable && rows !== null;

  /** Which rows fall inside the viewport plus the overscan. Reads only live
   *  scroll geometry, so it is cheap enough to run once per scroll frame. */
  const computeRows = useCallback((m: Metrics): RowRange | null => {
    const scroller = scrollerRef.current;
    const container = containerRef.current;
    if (!scroller || !container) return null;
    const height = m.itemHeight > 0 ? m.itemHeight + m.rowGap : 0;
    const total = m.columns > 0 ? Math.ceil(count / m.columns) : 0;
    if (height <= 0 || m.columns <= 0 || total <= 0) return null;

    const scrollerRect = scroller.getBoundingClientRect();
    const containerRect = container.getBoundingClientRect();
    offsetRef.current = containerRect.top - scrollerRect.top + scroller.scrollTop;

    const viewTop = scroller.scrollTop - offsetRef.current;
    const viewBottom = viewTop + scroller.clientHeight;
    const first = Math.max(0, Math.floor((viewTop - OVERSCAN_PX) / height));
    const last = Math.min(total, Math.ceil((viewBottom + OVERSCAN_PX) / height));
    return { first, last: Math.max(last, first + 1) };
  }, [count]);

  /** Re-read every layout metric and recompute the window in one pass. Metrics
   *  and rows must be committed together: computing rows from the previous
   *  render's metrics would paint one frame of the wrong window. */
  const fullSync = useCallback(() => {
    const el = windowRef.current;
    if (!el) return;

    const style = getComputedStyle(el);
    const rowGap = px(style.rowGap);
    const columnGap = px(style.columnGap);

    // The item height is uniform, so one rendered item measures them all.
    let itemHeight = metricsRef.current.itemHeight;
    const item = el.firstElementChild;
    if (item) {
      const height = item.getBoundingClientRect().height;
      if (height > 0) itemHeight = height;
    }

    let columns = 0;
    if (fixedColumns && fixedColumns > 0) {
      columns = fixedColumns;
    } else if (minColumnWidth && minColumnWidth > 0) {
      // An auto-fill track list fits as many fixed/minimum tracks as the
      // available width allows; the rule is the same either way.
      const width = el.clientWidth;
      columns = Math.max(1, Math.floor((width + columnGap) / (minColumnWidth + columnGap)));
    }

    const next: Metrics = { columns, itemHeight, rowGap, columnGap };
    if (!sameMetrics(metricsRef.current, next)) {
      metricsRef.current = next;
      setMetrics(next);
    }
    const range = computeRows(next);
    setRows((prev) =>
      prev && range && prev.first === range.first && prev.last === range.last ? prev : range,
    );
  }, [computeRows, fixedColumns, minColumnWidth]);

  // Measure before paint, so the starter render is never shown: the layout
  // effect commits the real window in the same frame. Also re-run when the
  // item count changes — a search can shrink a long list to a short one, which
  // changes the row count and invalidates the current window.
  const hasItems = count > 0;
  useLayoutEffect(() => {
    fullSync();
  }, [fullSync, measureKey, hasItems, count]);

  // Track the scroll container for the lifetime of the grid: recompute on
  // scroll, and re-measure on resize (which changes both the viewport height
  // and the container width, and so the column count).
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const scroller = findScroller(container);
    scrollerRef.current = scroller;
    if (!scroller) return;

    let frame = 0;
    const schedule = () => {
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        const range = computeRows(metricsRef.current);
        setRows((prev) =>
          prev && range && prev.first === range.first && prev.last === range.last ? prev : range,
        );
      });
    };

    scroller.addEventListener("scroll", schedule, { passive: true });
    const observer = new ResizeObserver(() => fullSync());
    observer.observe(scroller);
    observer.observe(container);
    return () => {
      if (frame) cancelAnimationFrame(frame);
      scroller.removeEventListener("scroll", schedule);
      observer.disconnect();
    };
  }, [computeRows, fullSync]);

  const scrollToIndex = useCallback(
    (index: number) => {
      const scroller = scrollerRef.current;
      if (!scroller || rowHeight <= 0 || metrics.columns <= 0) return;
      const row = Math.floor(index / metrics.columns);
      const centred = offsetRef.current + row * rowHeight - (scroller.clientHeight - rowHeight) / 2;
      const max = Math.max(0, scroller.scrollHeight - scroller.clientHeight);
      scroller.scrollTop = Math.max(0, Math.min(centred, max));
    },
    [metrics.columns, rowHeight],
  );

  // Clamp the window to the current row count. `rows` can lag one render behind
  // a shrinking list (a search result replacing the full library), and an
  // inverted range would slice to an empty grid.
  const range = rows ?? { first: 0, last: 0 };
  const firstRow = Math.min(range.first, Math.max(0, totalRows - 1));
  const lastRow = Math.min(Math.max(range.last, firstRow + 1), totalRows);
  const startIndex = windowed ? firstRow * metrics.columns : 0;
  const endIndex = windowed
    ? Math.min(count, lastRow * metrics.columns)
    : count > WINDOW_THRESHOLD
      ? Math.min(count, STARTER_ITEMS)
      : count;

  // Keyboard navigation selects an item the window may never have mounted, and
  // even a mounted item can sit in the overscan outside the viewport. Follow
  // the revealed index whenever it is not visible; only a change of index acts,
  // so this never fights the user's own scrolling, and the first pass is
  // skipped so a restored scroll position is not yanked to a stale selection.
  const revealed = useRef<number | null | undefined>(undefined);
  useEffect(() => {
    const previous = revealed.current;
    revealed.current = revealIndex ?? null;
    if (previous === undefined || revealIndex == null || previous === revealIndex) return;
    if (rowHeight <= 0 || metrics.columns <= 0) return;

    const scroller = scrollerRef.current;
    if (!scroller) return;
    const row = Math.floor(revealIndex / metrics.columns);
    const top = offsetRef.current + row * rowHeight - scroller.scrollTop;
    const bottom = top + rowHeight;
    if (top >= 0 && bottom <= scroller.clientHeight) return;
    scrollToIndex(revealIndex);
  }, [revealIndex, metrics.columns, rowHeight, scrollToIndex]);

  return {
    containerRef,
    windowRef,
    columns: metrics.columns,
    startIndex,
    endIndex,
    // Uses the clamped row so the offset always matches the rendered slice.
    offsetTop: windowed ? firstRow * rowHeight : 0,
    // Rows are separated by the gap, so the final row contributes no trailing gap.
    totalHeight: windowed ? totalRows * rowHeight - metrics.rowGap : 0,
    windowed,
    scrollToIndex,
  };
}
