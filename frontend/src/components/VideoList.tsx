import { useRef } from "react";
import type { CardSize, Video } from "../types";
import { usePagedList } from "../hooks/usePagedList";
import { useRevealInPage } from "../hooks/useRevealInPage";
import { scaleBetween } from "../utils/cardSize";
import { DESKTOP_CARD_SIZE } from "../utils/viewPrefs";
import { formatDuration, formatFileSize, videoCoverURL, videoResolution } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import { cn } from "../lib/utils";
import {
  AspectRatio,
  Badge,
  CheckboxInput,
  ClickableCard,
  IconButton,
  Spinner,
  Text,
  compactBadgeClass,
} from "../ui/astryx";
import { CoverImage } from "../ui/cover";
import { Icon } from "../ui/icon";
import { Pagination } from "../ui/pagination";

/** Same density as the grid, so switching views keeps the same items. */
const PAGE_SIZE = 60;

/**
 * Compact list view: one row per video, one page at a time. The card-size
 * slider drives the row thumbnail, title size, inner gap and vertical padding
 * continuously, so the list scales with the same control as the grid. It mirrors
 * VideoCard's interactions — click opens, double-click plays, right-click opens
 * the context menu, the checkbox toggles multi-select, and rows are draggable.
 *
 * The row is a ClickableCard rather than a ListItem: ClickableCard's hidden
 * full-surface control lets the row stay click-anywhere while the checkbox and
 * like button remain independent nested controls. `memoryKey` identifies the
 * paged list so a return to it restores the page.
 */
export default function VideoList({
  videos,
  memoryKey,
  selectedIds,
  onToggleSelect,
  onOpen,
  onPlay,
  onContextMenu,
  onLike,
  cardSize,
}: {
  videos: Video[];
  memoryKey: string;
  selectedIds: Set<number>;
  onToggleSelect: (id: number) => void;
  onOpen: (video: Video) => void;
  onPlay: (video: Video) => void;
  onContextMenu: (e: React.MouseEvent, video: Video) => void;
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

  // Keyboard navigation can select a row that sits on another page.
  const selected = selectedIds.size === 1 ? [...selectedIds][0] : null;
  const revealIndex = selected == null ? null : videos.findIndex((v) => v.id === selected);
  const listRef = useRef<HTMLDivElement | null>(null);

  const paging = usePagedList({
    total: videos.length,
    listKey: memoryKey,
    pageSize: PAGE_SIZE,
    revealIndex: revealIndex != null && revealIndex >= 0 ? revealIndex : null,
  });
  useRevealInPage({
    listRef,
    items: videos,
    startIndex: paging.startIndex,
    endIndex: paging.endIndex,
    revealIndex: revealIndex != null && revealIndex >= 0 ? revealIndex : null,
  });

  const turnPage = (next: number) => {
    paging.goToPage(next);
    listRef.current?.scrollIntoView({ block: "start", inline: "nearest" });
  };

  return (
    <div className="scroll-mt-28" ref={listRef}>
      <div className="flex flex-col gap-1.5">
        {videos.slice(paging.startIndex, paging.endIndex).map((v) => {
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
            <ClickableCard
              className={cn(
                "group relative rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)] transition hover:border-[var(--accent)]",
                "scroll-mt-28",
                missing && "opacity-75",
                isSelected &&
                  "border-[var(--accent)] shadow-[0_0_0_1px_var(--accent),0_2px_10px_rgba(124,92,255,0.2)]",
              )}
              data-video-id={v.id}
              key={v.id}
              label={videoDisplayName(v)}
              onClick={() => onOpen(v)}
              onContextMenu={(event) => onContextMenu(event, v)}
              onDoubleClick={() => onPlay(v)}
              padding={0}
              variant="transparent"
            >
              <div
                className="flex min-w-0 items-center px-3"
                style={{ paddingTop: rowPad, paddingBottom: rowPad, columnGap: rowGap }}
              >
                <span
                  className="flex shrink-0 items-center gap-2"
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
                          <CoverImage
                            className="block size-full"
                            src={videoCoverURL(v)}
                            variant="landscape"
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

                <span
                  className="flex min-w-0 flex-1 items-center gap-2"
                  title={remote ? `${v.sourceName} · ${v.remotePath}` : v.path}
                >
                  <span className="min-w-0 font-medium" style={{ fontSize: `${titleSize.toFixed(1)}px` }}>
                    <Text color="inherit" maxLines={1} type="inherit">{videoDisplayName(v)}</Text>
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
                        tooltip="点赞（可多次点赞）"
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
              </div>
            </ClickableCard>
          );
        })}
      </div>
      <Pagination
        page={paging.page}
        pageCount={paging.pageCount}
        total={videos.length}
        unit="部"
        onPageChange={turnPage}
      />
    </div>
  );
}
