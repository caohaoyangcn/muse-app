import { cn } from "../lib/utils";
import { Button } from "./button";

/**
 * The one pager for the app's lists.
 *
 * It is deliberately a plain presentational component: the list owns the page
 * state and the scroll container, so this file only names the current page and
 * exposes the two buttons. Keeping it that way lets the desktop shell and the
 * phone shell share the exact same control without the `ui/` layer depending on
 * a hook or a platform module.
 *
 * The bar sticks to the bottom of the scroll pane, so the way out of a long
 * page is always on screen; the caller scrolls the pane back to the list top
 * when the page changes.
 */
export function Pagination({
  page,
  pageCount,
  total,
  unit,
  onPrev,
  onNext,
  className,
}: {
  page: number;
  pageCount: number;
  /** Number of items across every page; the label prints it beside the page. */
  total: number;
  /** Measure word for `total` ("部", "位", "条", "项", "组"). */
  unit?: string;
  onPrev: () => void;
  onNext: () => void;
  className?: string;
}) {
  // A list that fits on one page needs neither the control nor the count.
  if (pageCount <= 1) return null;
  const totalText = total > 0 ? ` · 共 ${total}${unit ? ` ${unit}` : ""}` : "";

  return (
    <nav
      aria-label="分页"
      className={cn(
        "sticky bottom-0 z-[6] mt-3 flex items-center justify-center gap-2 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)]/95 px-2 py-1.5 backdrop-blur",
        className,
      )}
    >
      <Button disabled={page <= 1} onClick={onPrev} size="sm" variant="ghost">
        上一页
      </Button>
      <span
        aria-live="polite"
        className="min-w-0 truncate px-1 text-center text-[11px] leading-none text-[var(--muted)] tabular-nums"
      >
        第 {page} / {pageCount} 页{totalText}
      </span>
      <Button disabled={page >= pageCount} onClick={onNext} size="sm" variant="ghost">
        下一页
      </Button>
    </nav>
  );
}
