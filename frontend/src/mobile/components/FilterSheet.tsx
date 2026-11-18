import type { DiskOption } from "../../utils/diskFilter";
import { IconButton } from "../../ui/astryx";
import { Button } from "../../ui/button";
import { Icon } from "../../ui/icon";
import { BottomSheet } from "../../ui/sheet";
import {
  filterCount,
  type LibraryFilters,
  type ValueOption,
} from "../../utils/libraryFilter";
import { FilterSections } from "../../views/library/FilterSections";

/**
 * The library's filter sheet — conditions only. Ordering lives in SortSheet:
 * a filter decides *which* videos the phone has, a sort only their order, and
 * one sheet for both made the sort read like another condition.
 *
 * Every tap edits the live selection in the parent, not a private draft. That
 * is deliberate: closing the sheet with the drag handle, the backdrop or the
 * Android back gesture can therefore never discard choices — there is no second
 * copy to fall out of sync. "应用" only closes the sheet; the home screen has
 * already been showing the filtered rows and the chips behind it.
 *
 * Tags, actors and disks share their sections with the desktop dialog
 * (`views/library/FilterSections`), so the two clients cannot drift apart.
 * Actor and tag lists can be huge, so each has its own search box and page.
 */
export function FilterSheet({
  open,
  onClose,
  tags,
  actors,
  disks,
  filters,
  onChange,
  onReset,
}: {
  open: boolean;
  onClose: () => void;
  tags: ValueOption[];
  actors: ValueOption[];
  disks: DiskOption[];
  filters: LibraryFilters;
  onChange: (next: LibraryFilters) => void;
  onReset: () => void;
}) {
  const count = filterCount(filters);

  return (
    <BottomSheet
      headerEnd={
        <IconButton
          className="size-8 p-0 text-[var(--muted)]"
          icon={<Icon className="size-4" name="x" />}
          label="关闭筛选"
          onClick={onClose}
          size="sm"
          tooltip="关闭筛选"
          variant="ghost"
        />
      }
      height="hug"
      onClose={onClose}
      open={open}
      title="筛选"
    >
      <div className="px-1">
        <p className="px-2 pb-1 pt-0.5 text-[11px] leading-relaxed text-[var(--muted)]">
          {count === 0
            ? "未选择条件，显示全部收藏。同一栏内可多选。"
            : `已选 ${count} 个条件；同一栏内取并集，不同栏之间取交集。`}
        </p>

        <FilterSections
          actors={actors}
          disks={disks}
          filters={filters}
          onChange={onChange}
          open={open}
          tags={tags}
        />

        {/* Live selection: Astryx owns the controls; closing the sheet keeps
            them because onChange has already committed the state. */}
        <div className="sticky bottom-0 z-10 -mx-1 mt-1 flex items-center gap-2 border-t border-[var(--border)] bg-[var(--panel)] px-3 pb-3 pt-3">
          <Button
            className="h-10 shrink-0 px-4 text-sm"
            disabled={count === 0}
            onClick={onReset}
            variant="ghost"
          >
            重置
          </Button>
          <Button className="h-10 flex-1 text-sm" onClick={onClose} variant="primary">
            {count === 0 ? "应用" : `应用 · ${count} 项`}
          </Button>
        </div>
      </div>
    </BottomSheet>
  );
}
