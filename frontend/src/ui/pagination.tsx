import { useEffect, useRef } from "react";

import { cn } from "../lib/utils";
import { pageShortcutItems } from "../utils/pagination";
import { Button } from "./button";

/**
 * The one pager for the app's lists.
 *
 * It is deliberately a plain presentational component: the list owns the page
 * state and the scroll container, so this file only names the current page and
 * asks for a different one. Keeping it that way lets the desktop shell and the
 * phone shell share the exact same control without the `ui/` layer depending on
 * a hook or a platform module.
 *
 * The shortcut strip offers the first page, five pages on either side of the
 * current one, and the last page. On a narrow phone that strip is wider than the
 * bar, so it scrolls horizontally and centres the current page whenever the
 * page changes.
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
  onPageChange,
  className,
}: {
  page: number;
  pageCount: number;
  /** Number of items across every page; the label prints it beside the page. */
  total: number;
  /** Measure word for `total` ("部", "位", "条", "项", "组"). */
  unit?: string;
  onPageChange: (next: number) => void;
  className?: string;
}) {
  const stripRef = useRef<HTMLDivElement | null>(null);
  const activeRef = useRef<HTMLButtonElement | null>(null);

  // Keep the current page in the middle of the shortcut strip. The strip is
  // wider than the phone screen, so without this a jump from the first to the
  // last page could leave the highlighted number off-screen.
  useEffect(() => {
    const strip = stripRef.current;
    const active = activeRef.current;
    if (!strip || !active) return;
    const stripRect = strip.getBoundingClientRect();
    const activeRect = active.getBoundingClientRect();
    if (stripRect.width <= 0 || activeRect.width <= 0) return;
    const delta =
      activeRect.left + activeRect.width / 2 - (stripRect.left + stripRect.width / 2);
    strip.scrollTo({ left: Math.max(0, strip.scrollLeft + delta), behavior: "smooth" });
  }, [page, pageCount]);

  // A list that fits on one page needs neither the control nor the count.
  if (pageCount <= 1) return null;
  const totalText = total > 0 ? ` · 共 ${total}${unit ? ` ${unit}` : ""}` : "";
  const items = pageShortcutItems(page, pageCount);

  return (
    <nav
      aria-label="分页"
      className={cn(
        "sticky bottom-0 z-[6] mt-3 flex flex-col gap-1 border-t border-[var(--border)] bg-[var(--bg)]/95 px-2 py-1.5 backdrop-blur",
        className,
      )}
    >
      <span
        aria-live="polite"
        className="text-center text-[11px] leading-none text-[var(--muted)] tabular-nums"
      >
        第 {page} / {pageCount} 页{totalText}
      </span>
      <div className="flex min-w-0 items-center gap-1">
        <Button
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
          size="sm"
          variant="ghost"
        >
          上一页
        </Button>
        <div
          className="min-w-0 flex-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
          ref={stripRef}
        >
          <div className="mx-auto flex w-max items-center gap-1">
            {items.map((item, index) => {
              if (item === "gap") {
                return (
                  <span
                    aria-hidden="true"
                    className="shrink-0 px-0.5 text-[11px] leading-none text-[var(--muted)]"
                    key={`gap-${index}`}
                  >
                    …
                  </span>
                );
              }
              const current = item === page;
              return (
                <Button
                  aria-current={current ? "page" : undefined}
                  aria-label={`第 ${item} 页`}
                  className={cn("min-w-7 shrink-0 px-1.5 tabular-nums", current && "font-semibold")}
                  key={item}
                  onClick={() => onPageChange(item)}
                  ref={current ? activeRef : undefined}
                  size="sm"
                  title={item === 1 ? "第一页" : item === pageCount ? "最后一页" : undefined}
                  variant={current ? "primary" : "ghost"}
                >
                  {item}
                </Button>
              );
            })}
          </div>
        </div>
        <Button
          disabled={page >= pageCount}
          onClick={() => onPageChange(page + 1)}
          size="sm"
          variant="ghost"
        >
          下一页
        </Button>
      </div>
    </nav>
  );
}
