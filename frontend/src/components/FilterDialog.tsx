import type { DiskOption } from "../utils/diskFilter";
import {
  filterCount,
  type LibraryFilters,
  type ValueOption,
} from "../utils/libraryFilter";
import { Button } from "../ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader } from "../ui/dialog";
import { FilterSections } from "../views/library/FilterSections";

/**
 * The desktop app's filter surface. It mirrors the phone's FilterSheet: tags,
 * actors and disks, live selection (no draft), and closing never discards a
 * condition. Only the shell differs — a dialog here, a bottom sheet there.
 */
export default function FilterDialog({
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
    <Dialog open={open} onOpenChange={(next) => !next && onClose()} width={680}>
      <DialogContent>
        <DialogHeader
          title="筛选"
          subtitle={
            count === 0
              ? "未选择条件，显示全部收藏。同一栏内可多选。"
              : `已选 ${count} 个条件；同一栏内取并集，不同栏之间取交集。`
          }
        />
        <div className="max-h-[65vh] overflow-y-auto pr-1">
          <FilterSections
            actors={actors}
            disks={disks}
            filters={filters}
            onChange={onChange}
            open={open}
            tags={tags}
          />
        </div>
        <DialogFooter className="mt-3 border-t border-[var(--border)] pt-3">
          <Button disabled={count === 0} onClick={onReset}>
            重置
          </Button>
          <Button onClick={onClose} variant="primary">
            {count === 0 ? "完成" : `完成 · ${count} 项`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
