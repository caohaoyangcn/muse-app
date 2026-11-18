import { useRef } from "react";

import type { Video } from "../../types";
import {
  AspectRatio,
  Badge,
  ClickableCard,
  Grid,
  MoreMenu,
  Text,
  compactBadgeClass,
} from "../../ui/astryx";
import { CoverImage } from "../../ui/cover";
import { Icon } from "../../ui/icon";
import { Pagination } from "../../ui/pagination";
import { coverURL } from "../../services/web";
import { usePagedList } from "../../hooks/usePagedList";
import { videoMeta, videoName } from "../videoDisplay";
import { videoMenuItems, type VideoMenuActions } from "../videoMenu";

/** About three phone screens of posters, or four of rows. Grid and list share
 *  the size, so switching views keeps the same items on screen. */
const PAGE_SIZE = 24;

/** The poster ratio. Every card shares it, so a row always has one height
 *  regardless of how long the title is — the text sits over the artwork. */
const POSTER_RATIO = 3 / 4;

/** The narrowest poster the wall will draw. `Grid`'s responsive columns derive
 *  the count from it, so the wall reflows with the screen without anybody
 *  measuring it: two tracks on a phone, up to four when there is room. */
const MIN_POSTER_PX = 136;
const MAX_COLUMNS = 4;

function Thumb({ v }: { v: Video }) {
  if (v.status === "missing" && !v.volumeOffline) {
    return (
      <div className="flex size-full items-center justify-center bg-[#101013] text-[var(--danger)]">
        <Icon className="size-7 opacity-80" name="alert" />
      </div>
    );
  }
  return <CoverImage className="size-full" loading="lazy" src={coverURL(v)} variant="portrait" />;
}

/** Status pills that must be readable over artwork without a backing card. */
function StatusBadges({ v }: { v: Video }) {
  // A detached drive is not a missing file: the card keeps its artwork and adds
  // the warning (matching the desktop card).
  const offline = v.volumeOffline === true;
  const sourceOffline = v.sourceId > 0 && v.sourceOffline === true;
  const missing = !offline && v.status === "missing";
  if (!offline && !sourceOffline && !missing) return null;
  return (
    <div className="absolute left-2 top-2 flex max-w-[calc(100%-2.75rem)] flex-wrap gap-1">
      {missing ? <Badge label="文件缺失" variant="error" /> : null}
      {offline ? <Badge label="磁盘未接入" variant="error" /> : null}
      {sourceOffline ? <Badge label="网盘离线" variant="warning" /> : null}
    </div>
  );
}

function OverlayMeta({ v }: { v: Video }) {
  const meta = v.status === "missing" && !v.volumeOffline ? "找不到文件" : videoMeta(v);
  return (
    <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black via-black/70 to-transparent px-2.5 pb-2.5 pt-10">
      {v.mosaicRestored || v.backedUp ? (
        <div className="mb-1 flex flex-wrap gap-1">
          {v.mosaicRestored ? <Badge className={compactBadgeClass} label="马赛克破解" variant="purple" /> : null}
          {v.backedUp ? <Badge className={compactBadgeClass} label="已备份" variant="success" /> : null}
        </div>
      ) : null}
      {/* Text owns the clamp so a long 译名 ellipsizes and reveals itself on
          hover instead of being silently cut. */}
      <div className="text-[13px] font-semibold leading-[1.25] text-white [text-shadow:0_1px_8px_rgba(0,0,0,0.55)]">
        <Text color="inherit" maxLines={2} type="inherit">{videoName(v)}</Text>
      </div>
      <div className="mt-1 flex min-w-0 items-center gap-1.5 text-[10.5px] leading-none text-white/65">
        <span className="truncate">{meta}</span>
        {v.likeCount > 0 ? (
          <span className="ml-auto inline-flex shrink-0 items-center gap-0.5 tabular-nums">
            <Icon className="size-2.5" name="heart" />
            {v.likeCount}
          </span>
        ) : null}
      </div>
    </div>
  );
}

/**
 * The home poster wall. A portrait phone gets two full-width columns; wider
 * viewports add tracks rather than stretching a single card. Astryx's
 * ClickableCard supplies the interactive surface and nested-button handling,
 * while AspectRatio guarantees one poster shape for every row.
 *
 * The wall renders one page at a time (see usePagedList); `memoryKey` identifies
 * the current list so a return from a detail screen keeps the page.
 */
export function VideoGrid({
  videos,
  memoryKey,
  onOpen,
  actions,
  openMenuId,
  onMenuOpenChange,
}: {
  videos: Video[];
  memoryKey: string;
  onOpen: (v: Video) => void;
  actions: VideoMenuActions;
  openMenuId: number | null;
  onMenuOpenChange: (video: Video, open: boolean) => void;
}) {
  const paging = usePagedList({ total: videos.length, listKey: memoryKey, pageSize: PAGE_SIZE });
  const listRef = useRef<HTMLDivElement | null>(null);
  const turnPage = (next: number) => {
    paging.goToPage(next);
    listRef.current?.scrollIntoView({ block: "start" });
  };

  return (
    <div ref={listRef}>
      <Grid columns={{ minWidth: MIN_POSTER_PX, max: MAX_COLUMNS }} gap={2}>
        {videos.slice(paging.startIndex, paging.endIndex).map((v) => (
          <ClickableCard
            className="relative overflow-hidden border border-white/[0.07] bg-[#101013]"
            key={v.id}
            label={videoName(v)}
            onClick={() => onOpen(v)}
            padding={0}
            variant="transparent"
          >
            <AspectRatio ratio={POSTER_RATIO}>
              <div className="relative size-full">
                <Thumb v={v} />
                <StatusBadges v={v} />
                <OverlayMeta v={v} />
              </div>
            </AspectRatio>
            <div
              className="absolute right-2 top-2 z-20 rounded-full bg-black/55"
              onClick={(event) => event.stopPropagation()}
            >
              <MoreMenu
                icon={<Icon className="size-4 text-white" name="more-horizontal" />}
                isMenuOpen={openMenuId === v.id}
                items={videoMenuItems(v, actions)}
                label="更多操作"
                onOpenChange={(open) => onMenuOpenChange(v, open)}
                presentation="bottom-sheet"
                size="sm"
              />
            </div>
          </ClickableCard>
        ))}
      </Grid>
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

/** The compact list, paged the same way as the wall. One line of title per row
 *  keeps the rows scannable when a page is mostly long translated names. */
export function VideoList({
  videos,
  memoryKey,
  onOpen,
  actions,
  openMenuId,
  onMenuOpenChange,
}: {
  videos: Video[];
  memoryKey: string;
  onOpen: (v: Video) => void;
  actions: VideoMenuActions;
  openMenuId: number | null;
  onMenuOpenChange: (video: Video, open: boolean) => void;
}) {
  const paging = usePagedList({ total: videos.length, listKey: memoryKey, pageSize: PAGE_SIZE });
  const listRef = useRef<HTMLDivElement | null>(null);
  const turnPage = (next: number) => {
    paging.goToPage(next);
    listRef.current?.scrollIntoView({ block: "start" });
  };

  return (
    <div ref={listRef}>
      <div className="flex flex-col gap-2">
        {videos.slice(paging.startIndex, paging.endIndex).map((v) => {
          const meta = v.status === "missing" && !v.volumeOffline ? "找不到文件" : videoMeta(v);
          return (
            <ClickableCard
              className="rounded-[var(--radius)] border border-white/[0.07] bg-[var(--card)]/55"
              key={v.id}
              label={videoName(v)}
              onClick={() => onOpen(v)}
              padding={0}
              variant="transparent"
            >
              <div className="flex min-w-0 items-center gap-2 p-2">
                <span className="relative aspect-video w-32 shrink-0 overflow-hidden rounded-[7px] bg-[#101013]">
                  <Thumb v={v} />
                  <StatusBadges v={v} />
                </span>
                <div className="flex min-w-0 flex-1 flex-col">
                  <span className="min-w-0 text-sm font-medium">
                    <Text color="inherit" maxLines={1} type="inherit">{videoName(v)}</Text>
                  </span>
                  <span className="mt-1 flex h-4 min-w-0 items-center gap-1 text-[11px] text-[var(--muted)]">
                    <span className="truncate">{meta}</span>
                    {v.mosaicRestored ? <Badge className={compactBadgeClass} label="马赛克破解" variant="purple" /> : null}
                    {v.backedUp ? <Badge className={compactBadgeClass} label="已备份" variant="success" /> : null}
                  </span>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  {v.likeCount > 0 ? (
                    <span className="inline-flex items-center gap-0.5 text-[11px] text-[var(--muted)] tabular-nums">
                      <Icon className="size-2.5" name="heart" />
                      {v.likeCount}
                    </span>
                  ) : null}
                  <div
                    className="inline-flex shrink-0"
                    onClick={(event) => event.stopPropagation()}
                  >
                    <MoreMenu
                      icon={<Icon className="size-4 text-[var(--muted)]" name="more-horizontal" />}
                      isMenuOpen={openMenuId === v.id}
                      items={videoMenuItems(v, actions)}
                      label="更多操作"
                      onOpenChange={(open) => onMenuOpenChange(v, open)}
                      presentation="bottom-sheet"
                      size="sm"
                    />
                  </div>
                </div>
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
