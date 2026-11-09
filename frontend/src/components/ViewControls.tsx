import type { CardSize, ViewMode } from "../types";
import { Grid2X2, List, Minus, Plus } from "lucide-react";
import { cn } from "../lib/utils";
import { DESKTOP_CARD_SIZE } from "../utils/viewPrefs";
import { Button } from "./ui/button";

/**
 * The "how dense / which shape" half of the list toolbar: continuous card width
 * and the grid/list toggle. In the SortBar it sits at the right when there is
 * room and moves to a second line at the left when the window is narrow or
 * zoomed in. A view without sort chips — 最近播放 — renders it standalone.
 */
export default function ViewControls({
  view,
  onViewChange,
  cardSize,
  onCardSizeChange,
}: {
  view: ViewMode;
  onViewChange: (view: ViewMode) => void;
  cardSize: CardSize;
  onCardSizeChange: (size: CardSize) => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <div className="flex items-center gap-1" title={`卡片大小：${cardSize}px`}>
        <Minus className="h-3 w-3 shrink-0 text-[var(--muted)]" aria-hidden />
        <input
          type="range"
          className="muse-range w-[120px]"
          min={DESKTOP_CARD_SIZE.min}
          max={DESKTOP_CARD_SIZE.max}
          step={1}
          value={cardSize}
          onChange={(e) => onCardSizeChange(Number(e.target.value))}
          aria-label="作品卡片大小"
          aria-valuetext={`${cardSize} 像素`}
        />
        <Plus className="h-3 w-3 shrink-0 text-[var(--muted)]" aria-hidden />
      </div>
      <div
        className="flex overflow-hidden rounded-full border border-[var(--border)] bg-[var(--panel-2)]"
        role="group"
        aria-label="视图切换"
      >
        <Button
          variant="ghost"
          size="sm"
          className={cn(
            "h-7 rounded-none border-0 px-3 text-xs",
            view === "grid" && "bg-[rgba(124,92,255,0.14)] text-[var(--text)]",
          )}
          onClick={() => onViewChange("grid")}
          title="网格视图"
          aria-label="网格视图"
          aria-pressed={view === "grid"}
        >
          <Grid2X2 className="h-3.5 w-3.5" />
          网格
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className={cn(
            "h-7 rounded-none border-0 border-l border-l-[var(--border)] px-3 text-xs",
            view === "list" && "bg-[rgba(124,92,255,0.14)] text-[var(--text)]",
          )}
          onClick={() => onViewChange("list")}
          title="列表视图"
          aria-label="列表视图"
          aria-pressed={view === "list"}
        >
          <List className="h-3.5 w-3.5" />
          列表
        </Button>
      </div>
    </div>
  );
}
