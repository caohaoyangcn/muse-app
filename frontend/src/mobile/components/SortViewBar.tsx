import type { CardSize, SortDir, SortKey, ViewMode } from "../../types";
import { cn } from "../../lib/utils";
import { MOBILE_CARD_SIZE } from "../prefs";

const SORT_LABELS: Record<SortKey, string> = {
  createdAt: "添加时间",
  lastOpenedAt: "最近播放",
  fileSize: "文件大小",
  fileName: "文件名",
  likeCount: "点赞数",
};

const SORT_KEYS = Object.keys(SORT_LABELS) as SortKey[];

// Thumbnail size is a continuous card width shared by both view modes (the
// grid reflows its columns, the list resizes its row thumbnail).
/** The library toolbar: sort field/direction, card size and grid/list switch. */
export function SortViewBar({
  sortKey,
  sortDir,
  viewMode,
  cardSize,
  onSortKey,
  onToggleDir,
  onViewMode,
  onCardSize,
}: {
  sortKey: SortKey;
  sortDir: SortDir;
  viewMode: ViewMode;
  cardSize: CardSize;
  onSortKey: (k: SortKey) => void;
  onToggleDir: () => void;
  onViewMode: (m: ViewMode) => void;
  onCardSize: (s: CardSize) => void;
}) {
  return (
    <div className="flex min-w-0 items-center gap-2 border-b border-[var(--border)] pb-2">
      <div className="scroll-area flex min-w-0 flex-1 gap-1.5 overflow-x-auto">
        {SORT_KEYS.map((k) => {
          const active = k === sortKey;
          return (
            <button
              className={cn(
                "shrink-0 rounded-full border px-3 py-1.5 text-xs",
                active
                  ? "border-transparent bg-[var(--accent)] text-white"
                  : "border-[var(--border)] bg-[var(--panel-2)] text-[var(--muted)]",
              )}
              key={k}
              onClick={() => (active ? onToggleDir() : onSortKey(k))}
              type="button"
            >
              {SORT_LABELS[k]}
              {active ? (sortDir === "asc" ? " ↑" : " ↓") : ""}
            </button>
          );
        })}
      </div>
      <div
        className="flex shrink-0 items-center gap-1.5"
        role="group"
        aria-label="卡片大小与视图"
      >
        <input
          type="range"
          className="muse-range w-20"
          min={MOBILE_CARD_SIZE.min}
          max={MOBILE_CARD_SIZE.max}
          step={1}
          value={cardSize}
          onChange={(e) => onCardSize(Number(e.target.value))}
          aria-label="作品卡片大小"
          aria-valuetext={`${cardSize} 像素`}
        />
        <div className="flex overflow-hidden rounded-full border border-[var(--border)]">
          {(["grid", "list"] as ViewMode[]).map((m, i) => (
            <button
              className={cn(
                "px-3 py-1.5 text-xs",
                i > 0 && "border-l border-[var(--border)]",
                m === viewMode
                  ? "bg-[var(--accent)] text-white"
                  : "bg-[var(--panel-2)] text-[var(--muted)]",
              )}
              key={m}
              aria-label={m === "grid" ? "网格视图" : "列表视图"}
              aria-pressed={m === viewMode}
              onClick={() => onViewMode(m)}
              type="button"
            >
              {m === "grid" ? "▦" : "☰"}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
