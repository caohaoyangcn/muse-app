import type { CardSize, Video } from "../types";
import { useVirtualRows } from "../hooks/useVirtualRows";
import { scaleBetween } from "../utils/cardSize";
import { DESKTOP_CARD_SIZE } from "../utils/viewPrefs";
import { formatDuration, formatFileSize, videoCoverURL, videoResolution } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import { cn } from "../lib/utils";
import {
  AspectRatio,
  Badge,
  CheckboxInput,
  IconButton,
  List,
  ListItem,
  Spinner,
  compactBadgeClass,
} from "../ui/astryx";
import { Icon } from "../ui/icon";

/**
 * Compact list view: one row per video. The card-size slider drives the row
 * thumbnail, title size, inner gap and vertical padding continuously, so the
 * list scales with the same control as the grid. It mirrors VideoCard's
 * interactions — click opens, double-click plays, right-click opens the
 * context menu, the checkbox toggles multi-select, and rows are draggable.
 *
 * Astryx's ListItem supplies the row's hidden interactive surface and nested
 * control handling; the virtualizer only decides which rows are mounted.
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
      <List
        ref={(el) => { virtual.windowRef.current = el; }}
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

          return (
            <ListItem
              align="center"
              className={cn(
                "group relative border border-[var(--border)] bg-[var(--card)] transition hover:border-[var(--accent)]",
                missing && "opacity-75",
                isSelected &&
                  "border-[var(--accent)] shadow-[0_0_0_1px_var(--accent),0_2px_10px_rgba(124,92,255,0.2)]",
              )}
              data-video-id={v.id}
              draggable
              endContent={
                <span className="flex shrink-0 items-center gap-2 whitespace-nowrap text-xs text-[var(--muted)]">
                  {missing ? (
                    "找不到文件"
                  ) : (
                    <>
                      {meta}
                      <IconButton
                        className="h-5 w-5 p-0 text-[var(--muted)] hover:text-[var(--accent)]"
                        icon={<Icon className="size-3" name="heart" />}
                        label={`点赞（${v.likeCount || 0}）`}
                        onClick={(event) => {
                          event.stopPropagation();
                          onLike(v.id);
                        }}
                        size="sm"
                        variant="ghost"
                      />
                      <span className="tabular-nums">{v.likeCount || 0}</span>
                    </>
                  )}
                </span>
              }
              isSelected={isSelected}
              key={v.id}
              label={
                <span
                  className="flex min-w-0 items-center gap-2"
                  title={remote ? `${v.sourceName} · ${v.remotePath}` : v.path}
                >
                  <span className="truncate font-medium" style={{ fontSize: `${titleSize.toFixed(1)}px` }}>
                    {videoDisplayName(v)}
                  </span>
                  {v.mosaicRestored ? (
                    <Badge className={compactBadgeClass} label="马赛克破解" variant="purple" />
                  ) : null}
                  {v.backedUp ? (
                    <Badge className={compactBadgeClass} label="已备份" variant="success" />
                  ) : null}
                  {remote ? (
                    <Badge
                      className={compactBadgeClass}
                      label={sourceOffline ? "网盘离线" : (v.sourceName || "网盘")}
                      variant={sourceOffline ? "warning" : "neutral"}
                    />
                  ) : null}
                </span>
              }
              onClick={() => onOpen(v)}
              onContextMenu={(event) => onContextMenu(event, v)}
              onDoubleClick={() => onPlay(v)}
              onDragStart={(event) => onDragStart(event, v)}
              startContent={
                <span
                  className="flex shrink-0 items-center gap-2"
                  onDragStart={(event) => event.stopPropagation()}
                  onMouseDown={(event) => event.stopPropagation()}
                >
                  <CheckboxInput
                    isLabelHidden
                    label={isSelected ? `取消选择 ${videoDisplayName(v)}` : `选择 ${videoDisplayName(v)}`}
                    onChange={() => onToggleSelect(v.id)}
                    size="sm"
                    value={isSelected}
                  />
                  <span
                    className="relative block shrink-0 overflow-hidden rounded-md bg-[#111114]"
                    style={{ width: thumbWidth }}
                  >
                    <AspectRatio ratio={16 / 9}>
                      <span className="relative block size-full">
                        {hasThumb ? (
                          <img
                            alt=""
                            className="block size-full object-contain"
                            draggable={false}
                            src={videoCoverURL(v)}
                          />
                        ) : (
                          <span className="flex size-full items-center justify-center opacity-60">
                            {missing ? (
                              <Icon className="size-5 text-[var(--danger)]" name="alert" />
                            ) : (
                              <Spinner shade="subtle" size="md" />
                            )}
                          </span>
                        )}
                        {(duration || offline) && (
                          <span className="absolute bottom-1 right-1 flex items-center gap-1">
                            {duration ? (
                              <Badge
                                className={cn(compactBadgeClass, "border-transparent bg-black/75 text-white")}
                                label={duration}
                              />
                            ) : null}
                            {offline ? <Badge label="磁盘未接入" variant="error" /> : null}
                          </span>
                        )}
                        {missing ? (
                          <Badge className="absolute left-1 top-1" label="文件缺失" variant="error" />
                        ) : null}
                      </span>
                    </AspectRatio>
                  </span>
                </span>
              }
              style={{ paddingTop: rowPad, paddingBottom: rowPad, columnGap: rowGap }}
            />
          );
        })}
      </List>
    </div>
  );
}
