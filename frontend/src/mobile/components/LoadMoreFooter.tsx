import { Button } from "../../ui/button";
import { PAGE_SIZE, type LoadMoreState } from "../useLoadMore";

/**
 * The end of a paged phone list.
 *
 * It carries the one control that is guaranteed to work when scrolling to the
 * end does not — a plain 加载更多 button — plus the count, so it is obvious how
 * much of the library is on screen. The element is also what the paging hook
 * watches: bringing it within a screen of the viewport loads the next page, so
 * the button is usually already replaced by fresh posters by the time it would
 * have been read.
 *
 * The count line is only drawn for lists long enough to be paged; a handful of
 * favorites needs no footer arithmetic.
 */
export function LoadMoreFooter({ state, total }: { state: LoadMoreState; total: number }) {
  // A short list is never paged, so it needs neither the button nor the count —
  // and no empty footer adding space under its last row.
  if (total <= PAGE_SIZE) return null;
  return (
    <div className="mt-3 flex flex-col items-center gap-2 pb-1 pt-2" ref={state.sentinelRef}>
      {state.hasMore ? (
        <Button className="h-9 shrink-0 rounded-full px-4 text-xs" onClick={state.loadMore} variant="ghost">
          加载更多
        </Button>
      ) : null}
      <span className="text-[11px] leading-none text-[var(--muted)] tabular-nums">
        {state.hasMore ? `已显示 ${state.visibleCount} / ${total} 部` : `已显示全部 ${total} 部`}
      </span>
    </div>
  );
}
