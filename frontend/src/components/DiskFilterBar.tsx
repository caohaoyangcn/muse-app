import type { DiskOption } from "../utils/diskFilter";
import { cn } from "../lib/utils";
import { ToggleButton, ToggleButtonGroup, toggleChipClass } from "../ui/astryx";
import { Icon } from "../ui/icon";

/**
 * Multi-select disk filter for the library list. Every chip is a volume the
 * loaded favorites live on; clicking toggles it and the list shows the union of
 * the selected disks. "全部" is the empty selection, which is also the state
 * the list starts in.
 *
 * The toggle group is Astryx's; this component only maps disks to its values.
 */
export default function DiskFilterBar({
  options,
  selected,
  onToggle,
  onClear,
  className,
}: {
  options: DiskOption[];
  selected: ReadonlySet<string>;
  onToggle: (option: DiskOption) => void;
  onClear: () => void;
  className?: string;
}) {
  return (
    <div
      className={cn("flex flex-nowrap items-center gap-2", className)}
      role="group"
      aria-label="按硬盘筛选"
    >
      <span className="text-xs text-[var(--muted)]">硬盘</span>
      <ToggleButton
        className={cn(toggleChipClass, "h-7 shrink-0")}
        isPressed={selected.size === 0}
        label="全部"
        onPressedChange={(pressed) => {
          if (pressed) onClear();
        }}
        size="sm"
        tooltip="显示所有硬盘的影片"
      >
        全部
      </ToggleButton>
      <ToggleButtonGroup
        className="flex min-w-0 flex-wrap items-center gap-1.5"
        label="按硬盘筛选"
        onChange={(values) => {
          const next = new Set(values);
          for (const option of options) {
            if (selected.has(option.key) !== next.has(option.key)) onToggle(option);
          }
        }}
        size="sm"
        type="multiple"
        value={[...selected]}
      >
        {options.map((option) => (
          <ToggleButton
            className={toggleChipClass}
            key={option.key}
            label={diskTitle(option)}
            tooltip={diskTitle(option)}
            value={option.key}
          >
            <span className="truncate">{option.label}</span>
            <span className="text-[11px] opacity-70">{option.count}</span>
            {option.offline ? <Icon className="size-3 text-[var(--danger)]" name="alert" /> : null}
          </ToggleButton>
        ))}
      </ToggleButtonGroup>
    </div>
  );
}

function diskTitle(option: DiskOption): string {
  const parts = [option.root || option.label, `${option.count} 个视频`];
  if (option.offline) parts.push("磁盘未接入");
  if (option.count === 0) parts.push("当前列表中没有该硬盘的影片");
  return parts.join(" · ");
}
