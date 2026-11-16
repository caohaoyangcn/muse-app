import type { SortDir, SortKey } from "../../types";
import {
  Divider,
  Section,
  SegmentedControl,
  SegmentedControlItem,
  ToggleButton,
  ToggleButtonGroup,
  toggleChipClass,
} from "../../ui/astryx";
import { Icon } from "../../ui/icon";
import { BottomSheet } from "../../ui/sheet";
import { SheetHeading } from "./SheetHeading";
import { SORT_KEYS, SORT_LABELS } from "../sortOptions";

/**
 * The library's sort sheet — the toolbar's second control, separate from 筛选.
 *
 * Sorting and filtering are different promises: the filter changes *which*
 * videos exist for the phone, the sort only changes their order. Keeping them
 * in one sheet made the sort look like another condition, so it lives here on
 * its own, and every tap commits immediately (the rows behind the sheet
 * reorder as you choose).
 */
export function SortSheet({
  open,
  onClose,
  sortKey,
  sortDir,
  onSortKey,
  onSortDir,
}: {
  open: boolean;
  onClose: () => void;
  sortKey: SortKey;
  sortDir: SortDir;
  onSortKey: (key: SortKey) => void;
  onSortDir: (dir: SortDir) => void;
}) {
  return (
    <BottomSheet height="hug" onClose={onClose} open={open} title="排序">
      <div className="px-1">
        <Section className="px-2 py-3.5" padding={0} variant="transparent">
          <SheetHeading hint="只影响顺序，不影响筛选" title="排序字段" />
          <ToggleButtonGroup
            className="flex flex-wrap gap-1.5"
            label="排序字段"
            onChange={(value) => value && onSortKey(value as SortKey)}
            size="sm"
            type="single"
            value={sortKey}
          >
            {SORT_KEYS.map((key) => (
              <ToggleButton className={toggleChipClass} key={key} label={SORT_LABELS[key]} value={key} />
            ))}
          </ToggleButtonGroup>
        </Section>

        <Divider />

        <Section className="px-2 py-3.5" padding={0} variant="transparent">
          <SheetHeading hint={sortDir === "asc" ? "从小到大" : "从大到小"} title="排序方向" />
          <SegmentedControl
            label="排序方向"
            layout="hug"
            onChange={(value) => onSortDir(value as SortDir)}
            size="sm"
            value={sortDir}
          >
            <SegmentedControlItem
              icon={<Icon className="size-3" name="arrow-down" />}
              label="降序"
              value="desc"
            />
            <SegmentedControlItem
              icon={<Icon className="size-3" name="arrow-up" />}
              label="升序"
              value="asc"
            />
          </SegmentedControl>
        </Section>
      </div>
    </BottomSheet>
  );
}
