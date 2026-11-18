import { IconButton } from "../../ui/astryx";
import { Button } from "../../ui/button";
import { Icon } from "../../ui/icon";
import type { DiskOption } from "../../utils/diskFilter";
import {
  buildFilterChips,
  chipKindLabel,
  removeFilterChip,
  type FilterChip,
  type LibraryFilters,
} from "../../utils/libraryFilter";
import { cn } from "../../lib/utils";

/**
 * The applied conditions as removable chips. Shared so the desktop toolbar and
 * the phone home screen show the exact same set: a condition the user can see
 * is a condition the user can remove, including one whose option disappeared
 * from the current list.
 */
export function FilterChips({
  filters,
  disks,
  onChange,
  onClear,
  className,
}: {
  filters: LibraryFilters;
  disks: readonly DiskOption[];
  onChange: (next: LibraryFilters) => void;
  onClear?: () => void;
  className?: string;
}) {
  const chips = buildFilterChips(filters, disks);
  if (chips.length === 0) return null;
  return (
    <div className={cn("flex items-center gap-2", className)}>
      <div className="flex min-w-0 flex-1 gap-1.5 overflow-x-auto">
        {chips.map((chip) => (
          <AppliedChip
            chip={chip}
            key={chip.id}
            onRemove={() => onChange(removeFilterChip(filters, chip))}
          />
        ))}
      </div>
      {onClear ? (
        <Button
          className="h-7 shrink-0 rounded-full px-2 text-[11px] text-[var(--muted)]"
          onClick={onClear}
          size="sm"
          variant="ghost"
        >
          清除
        </Button>
      ) : null}
    </div>
  );
}

function AppliedChip({ chip, onRemove }: { chip: FilterChip; onRemove: () => void }) {
  return (
    <span className="inline-flex h-7 shrink-0 items-center gap-1 rounded-full border border-[rgba(124,92,255,0.45)] bg-[rgba(124,92,255,0.14)] pl-2.5 pr-1">
      <span className="text-[10px] leading-none text-[var(--muted)]">{chipKindLabel(chip.kind)}</span>
      <span className="max-w-[9rem] truncate text-[11px] leading-none text-[var(--text)]">
        {chip.label}
      </span>
      <IconButton
        className="size-5 min-w-0 shrink-0 rounded-full p-0 text-[var(--muted)]"
        icon={<Icon className="size-3" name="x" />}
        label={`移除筛选：${chipKindLabel(chip.kind)} ${chip.label}`}
        onClick={onRemove}
        size="sm"
        tooltip="移除筛选"
        variant="ghost"
      />
    </span>
  );
}
