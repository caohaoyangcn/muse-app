import type { CardSize, Video } from "../types";
import { useVirtualRows } from "../hooks/useVirtualRows";
import RestoredBadge from "./RestoredBadge";
import { scaleBetween } from "../utils/cardSize";
import { DESKTOP_CARD_SIZE } from "../utils/viewPrefs";
import { formatDuration, formatFileSize, videoCoverURL, videoResolution } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import { cn } from "../lib/utils";

/**
 * Compact list view: one row per video. The card-size slider drives the row
 * thumbnail, title size, inner gap and vertical padding continuously, so the
 * list scales with the same control as the grid. It mirrors VideoCard's
 * interactions — click selects (double-click opens), right-click opens the
 * context menu, the checkbox toggles multi-select, and rows are draggable
 * (single or multi-selection) — so both views share the exact same handlers.
 */
export default function VideoList({
  videos,
  selectedIds,
  onToggleSelect,
  onOpen,
  onPlay,
  onContextMenu,
  onDragStart,
  onLike,
  cardSize,
}: {
  videos: Video[];
  selectedIds: Set<number>;
  onToggleSelect: (id: number) => void;
  onOpen: (video: Video) => void;
  onPlay: (video: Video) => void;
  onContextMenu: (e: React.MouseEvent, video: Video) => void;
  onDragStart: (e: React.DragEvent, video: Video) => void;
  onLike: (id: number) => void;
  cardSize: CardSize;
}) {
  // The row has no column width to derive from, so the target card width is
  // scaled into the row's own metrics. Ratios/bounds are calibrated so the
  // default size reproduces the old "中" row (96px thumbnail, 14px title).
  const thumbWidth = Math.min(160, Math.max(72, Math.round(cardSize * 0.38)));
  const titleSize = scaleBetween(cardSize, DESKTOP_CARD_SIZE, 13, 16);
  const rowGap = Math.round(scaleBetween(cardSize, DESKTOP_CARD_SIZE, 10, 16));
  const rowPad = Math.round(scaleBetween(cardSize, DESKTOP_CARD_SIZE, 4, 8));

  // Keyboard navigation can select a row that sits outside the window.
  const selected = selectedIds.size === 1 ? [...selectedIds][0] : null;
  const revealIndex = selected == null ? null : videos.findIndex((v) => v.id === selected);

  const virtual = useVirtualRows({
    count: videos.length,
    fixedColumns: 1,
    measureKey: cardSize,
    revealIndex: revealIndex != null && revealIndex >= 0 ? revealIndex : null,
  });
  const { startIndex, endIndex, windowed, offsetTop, totalHeight } = virtual;

  return (
    <div
      ref={virtual.containerRef}
      className="relative"
      style={windowed ? { height: totalHeight } : undefined}
    >
      <div
        ref={virtual.windowRef}
        className="flex flex-col gap-1.5"
        style={windowed ? { position: "absolute", top: offsetTop, left: 0, right: 0 } : undefined}
      >
        {videos.slice(startIndex, endIndex).map((v) => {
          // A detached volume is not a missing file: the row stays normal and
          // carries only the warning (see VideoCard).
          const offline = v.volumeOffline === true;
          const remote = v.sourceId > 0;
          const sourceOffline = remote && v.sourceOffline === true;
          const missing = !offline && v.status === "missing";
          const isSelected = selectedIds.has(v.id);
          const hasThumb = !missing && v.thumbnailPath !== "";
          const duration = formatDuration(v.durationMs);
          const meta = [duration, videoResolution(v), formatFileSize(v.fileSize)]
            .filter(Boolean)
            .join(" · ");

          const handleRowDragStart = (e: React.DragEvent) => onDragStart(e, v);

          return (
            <div
              key={v.id}
              className={cn(
                "relative flex cursor-pointer items-center rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)] px-3 transition hover:border-[var(--accent)]",
                missing && "opacity-75",
                isSelected &&
                  "border-[var(--accent)] shadow-[0_0_0_1px_var(--accent),0_2px_10px_rgba(124,92,255,0.2)]",
              )}
              style={{ columnGap: rowGap, paddingTop: rowPad, paddingBottom: rowPad }}
              draggable
              data-video-id={v.id}
              onClick={() => onOpen(v)}
              onDoubleClick={() => onPlay(v)}
              onContextMenu={(e) => onContextMenu(e, v)}
              onDragStart={handleRowDragStart}
              title={remote ? `${v.sourceName} · ${v.remotePath}` : v.path}
            >
              <button
                type="button"
                className={cn(
                  "flex h-5 w-5 shrink-0 items-center justify-center rounded-[5px] border border-white/55 bg-black/35 p-0 text-[13px] leading-none text-white",
                  isSelected && "border-[var(--accent)] bg-[var(--accent)]",
                )}
                aria-label={isSelected ? `取消选择 ${videoDisplayName(v)}` : `选择 ${videoDisplayName(v)}`}
                aria-pressed={isSelected}
                title={isSelected ? "取消选择" : "选择"}
                onClick={(e) => {
                  e.stopPropagation();
                  onToggleSelect(v.id);
                }}
                onDragStart={(e) => e.stopPropagation()}
                onMouseDown={(e) => e.stopPropagation()}
              >
                {isSelected ? "✓" : ""}
              </button>
              <div
                className="relative aspect-video shrink-0 overflow-hidden rounded-md bg-[#111114]"
                style={{ width: thumbWidth }}
              >
                {hasThumb ? (
                  <img className="block h-full w-full object-contain" src={videoCoverURL(v)} alt="" draggable={false} />
                ) : (
                  <div className="flex h-full w-full items-center justify-center text-[28px] opacity-50">{missing ? "⚠" : <span className="h-[22px] w-[22px] animate-spin rounded-full border-[3px] border-[var(--border)] border-t-[var(--accent)]" />}</div>
                )}
                {(duration || offline) && (
                  <div className="absolute bottom-1.5 right-1.5 flex items-center gap-1">
                    {duration && (
                      <span className="rounded bg-black/75 px-1.5 py-0.5 text-[11px] text-white">{duration}</span>
                    )}
                    {offline && (
                      <span
                        className="rounded bg-[rgba(229,83,75,0.9)] px-2 py-0.5 text-[11px] text-white"
                        title={`${v.volumeRoot || "磁盘"} 未接入：${v.path}`}
                      >
                        ⚠ 磁盘未接入
                      </span>
                    )}
                  </div>
                )}
                {missing && <span className="absolute left-1.5 top-1.5 rounded bg-[rgba(229,83,75,0.9)] px-2 py-0.5 text-[11px] text-white">⚠ 缺失</span>}
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex min-w-0 items-center gap-2">
                  <div
                    className="truncate font-medium"
                    style={{ fontSize: `${titleSize.toFixed(1)}px` }}
                  >
                    {videoDisplayName(v)}
                  </div>
                  {v.mosaicRestored && <RestoredBadge />}
                  {remote && (
                    <span
                      className={cn(
                        "shrink-0 rounded border px-1.5 py-0.5 text-[10px] leading-none",
                        sourceOffline
                          ? "border-[#e0b64f]/50 text-[#e0b64f]"
                          : "border-[var(--border)] text-[var(--muted)]",
                      )}
                      title={v.remotePath}
                    >
                      {sourceOffline ? "网盘离线" : `☁ ${v.sourceName || "网盘"}`}
                    </span>
                  )}
                </div>
              </div>
              <div className="shrink-0 whitespace-nowrap text-xs text-[var(--muted)]">
                {missing ? (
                  "找不到文件"
                ) : (
                  <>
                    {meta}
                    <button
                      type="button"
                      className="ml-2 inline-flex items-center gap-0.5 border-0 bg-transparent px-1.5 py-0.5 text-xs leading-none text-[var(--muted)] hover:bg-[rgba(124,92,255,0.1)] hover:text-[var(--accent)]"
                      title="点赞（可多次点赞）"
                      aria-label="点赞"
                      onClick={(e) => {
                        e.stopPropagation();
                        onLike(v.id);
                      }}
                      onMouseDown={(e) => e.stopPropagation()}
                    >
                      ♥ <span className="tabular-nums">{v.likeCount || 0}</span>
                    </button>
                  </>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
