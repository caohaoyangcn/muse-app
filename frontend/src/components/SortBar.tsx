import type { CardSize, SortDir, SortKey, ViewMode } from "../types";
import { ArrowDown, ArrowUp } from "lucide-react";
import { cn } from "../lib/utils";
import { Button } from "./ui/button";
import ViewControls from "./ViewControls";

// The sort fields with the direction each one starts in when first selected
// (newest first, largest first, A→Z).
const OPTIONS: { key: SortKey; label: string; defaultDir: SortDir }[] = [
  { key: "createdAt", label: "添加时间", defaultDir: "desc" },
  { key: "lastOpenedAt", label: "最近播放", defaultDir: "desc" },
  { key: "fileSize", label: "文件大小", defaultDir: "desc" },
  { key: "fileName", label: "文件名", defaultDir: "asc" },
  { key: "likeCount", label: "点赞数", defaultDir: "desc" },
];

/**
 * Toolbar row for sort options. Clicking a field sorts by it (with that field's
 * default direction); clicking the active field again flips between ascending
 * and descending; the active field shows an arrow.
 *
 * The card-size slider and the grid/list toggle live in ViewControls. They stay
 * at the end of this row by default; a view that pins them next to its title
 * passes `showViewControls={false}`.
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
  return (
    <div className={cn("mb-3 flex items-center gap-2", className)} role="toolbar" aria-label="排序与视图">
      <span className="text-xs text-[var(--muted)]">排序</span>
      {OPTIONS.map((opt) => {
        const active = opt.key === sortBy;
        const dir: SortDir = active ? sortDir : opt.defaultDir;
        return (
          <Button
            key={opt.key}
            size="sm"
            className={cn(
              "h-7 rounded-full px-3 text-xs text-[var(--muted)]",
              active && "border-[var(--accent)] bg-[rgba(124,92,255,0.14)] text-[var(--text)]",
            )}
            onClick={() =>
              active ? onChange(opt.key, dir === "asc" ? "desc" : "asc") : onChange(opt.key, opt.defaultDir)
            }
            title={
              active
                ? dir === "asc"
                  ? `按${opt.label}升序，点击改为降序`
                  : `按${opt.label}降序，点击改为升序`
                : `按${opt.label}排序`
            }
          >
            {opt.label}
            {active && (dir === "asc" ? <ArrowUp className="h-3 w-3" /> : <ArrowDown className="h-3 w-3" />)}
          </Button>
        );
      })}
      {showViewControls && (
        <div className="ml-auto flex items-center gap-2">
          <ViewControls
            view={view}
            onViewChange={onViewChange}
            cardSize={cardSize}
            onCardSizeChange={onCardSizeChange}
          />
        </div>
      )}
    </div>
  );
}
