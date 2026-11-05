import type { DiskOption } from "../utils/diskFilter";
import { cn } from "../lib/utils";
import { Button } from "./ui/button";

const ACTIVE_CHIP =
  "border-[var(--accent)] bg-[rgba(124,92,255,0.14)] text-[var(--text)]";

/**
 * Multi-select disk filter for the library list. Every chip is a volume the
 * loaded favorites live on; clicking toggles it and the list shows the union of
 * the selected disks. "全部" is the empty selection, which is also the state
 * the list starts in.
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
    <div className={cn("flex flex-nowrap items-center gap-2", className)} role="group" aria-label="按硬盘筛选">
      <span className="text-xs text-[var(--muted)]">硬盘</span>
      <Button
        size="sm"
        className={cn(
          "h-7 gap-1 rounded-full px-3 text-xs text-[var(--muted)]",
          selected.size === 0 && ACTIVE_CHIP,
        )}
        onClick={onClear}
        title="显示所有硬盘的影片"
        aria-pressed={selected.size === 0}
      >
        全部
      </Button>
      {options.map((option) => {
        const active = selected.has(option.key);
        return (
          <Button
            key={option.key}
            size="sm"
            className={cn(
              "h-7 gap-1 rounded-full px-3 text-xs text-[var(--muted)]",
              active && ACTIVE_CHIP,
            )}
            onClick={() => onToggle(option)}
            title={diskTitle(option)}
            aria-pressed={active}
          >
            {option.label}
            <span className="text-[11px] opacity-70">{option.count}</span>
            {option.offline && <span title="磁盘未接入">⚠</span>}
          </Button>
        );
      })}
    </div>
  );
}

function diskTitle(option: DiskOption): string {
  const parts = [option.root || option.label, `${option.count} 个视频`];
  if (option.offline) parts.push("磁盘未接入");
  if (option.count === 0) parts.push("当前列表中没有该硬盘的影片");
  return parts.join(" · ");
}
