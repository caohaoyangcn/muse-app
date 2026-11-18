import type { CardSize, SortDir, SortKey, ViewMode } from "../types";
import { cn } from "../lib/utils";
import { SORT_FIELDS } from "../utils/sortOptions";
import { ToggleButton, ToggleButtonGroup, Toolbar, toggleChipClass } from "../ui/astryx";
import { Icon } from "../ui/icon";
import ViewControls from "./ViewControls";

/**
 * Toolbar row for sort options. Clicking a field sorts by it (with that field's
 * default direction); clicking the active field again flips between ascending
 * and descending; the active field shows an arrow.
 *
 * Astryx's ToggleButtonGroup owns the single-selection/keyboard/ARIA behaviour;
 * ViewControls owns density and grid/list.
 */
export default function SortBar({
  sortBy,
  sortDir,
  onChange,
  view,
  onViewChange,
  cardSize,
  onCardSizeChange,
  showViewControls = true,
  className,
}: {
  sortBy: SortKey;
  sortDir: SortDir;
  onChange: (key: SortKey, dir: SortDir) => void;
  view: ViewMode;
  onViewChange: (view: ViewMode) => void;
  cardSize: CardSize;
  onCardSizeChange: (size: CardSize) => void;
  showViewControls?: boolean;
  className?: string;
}) {
  const select = (value: string | null) => {
    if (value === null) {
      const dir: SortDir = sortDir === "asc" ? "desc" : "asc";
      onChange(sortBy, dir);
      return;
    }
    const option = SORT_FIELDS.find((candidate) => candidate.key === value);
    if (option) onChange(option.key, option.defaultDir);
  };

  return (
    <Toolbar
      className={cn("mb-3", className)}
      endContent={
        showViewControls ? (
          <ViewControls
            cardSize={cardSize}
            onCardSizeChange={onCardSizeChange}
            onViewChange={onViewChange}
            view={view}
          />
        ) : undefined
      }
      label="排序与视图"
      size="sm"
      startContent={
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-2">
          <span className="text-xs text-[var(--muted)]">排序</span>
          <ToggleButtonGroup
            className="flex flex-wrap"
            label="排序字段"
            onChange={select}
            size="sm"
            type="single"
            value={sortBy}
          >
            {SORT_FIELDS.map((option) => {
              const active = option.key === sortBy;
              return (
                <ToggleButton
                  icon={
                    active ? (
                      <Icon
                        className="size-3"
                        name={sortDir === "asc" ? "arrow-up" : "arrow-down"}
                      />
                    ) : undefined
                  }
                  className={toggleChipClass}
                  key={option.key}
                  label={option.label}
                  tooltip={
                    active
                      ? sortDir === "asc"
                        ? `按${option.label}升序，点击改为降序`
                        : `按${option.label}降序，点击改为升序`
                      : `按${option.label}排序`
                  }
                  value={option.key}
                />
              );
            })}
          </ToggleButtonGroup>
        </div>
      }
      variant="transparent"
    />
  );
}
