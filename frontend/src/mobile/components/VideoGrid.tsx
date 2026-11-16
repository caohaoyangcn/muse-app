import type { Video } from "../../types";
import {
  AspectRatio,
  Badge,
  ClickableCard,
  Grid,
  List,
  ListItem,
  compactBadgeClass,
} from "../../ui/astryx";
import { Button } from "../../ui/button";
import { Icon } from "../../ui/icon";
import { useVirtualRows } from "../../hooks/useVirtualRows";
import { cn } from "../../lib/utils";
import { coverURL } from "../../services/web";
import { videoMeta, videoName } from "../videoDisplay";

/** The poster ratio. Every card shares it, so a row always has one height
 *  regardless of how long the title is — the text sits over the artwork. */
const POSTER_RATIO = 3 / 4;

function Thumb({ v }: { v: Video }) {
  if (v.status === "missing" && !v.volumeOffline) {
    return (
      <div className="flex size-full items-center justify-center bg-[#101013] text-[var(--danger)]">
        <Icon className="size-7 opacity-80" name="alert" />
      </div>
    );
  }
  return (
    <img
      alt=""
      className="size-full object-cover"
      draggable={false}
      loading="lazy"
      src={coverURL(v)}
    />
  );
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

function CardMenu({ onOpen }: { onOpen: () => void }) {
  return (
    <Button
      aria-label="更多操作"
      className="absolute right-2 top-2 z-20 size-8 rounded-full bg-black/55 p-0 text-white active:bg-black/80"
      onClick={(event) => {
        event.stopPropagation();
        onOpen();
      }}
      size="sm"
      variant="ghost"
    >
      <Icon className="size-4" name="more-horizontal" />
    </Button>
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
      <div className="line-clamp-2 text-[13px] font-semibold leading-[1.25] text-white [text-shadow:0_1px_8px_rgba(0,0,0,0.55)]">
        {videoName(v)}
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
 */
export function VideoGrid({
  videos,
  onOpen,
  onMenu,
}: {
  videos: Video[];
  onOpen: (v: Video) => void;
  onMenu: (v: Video) => void;
}) {
  const virtual = useVirtualRows({
    count: videos.length,
    minColumnWidth: 136,
    maxColumns: 4,
    measureKey: "cinematic",
  });
  const columns = virtual.columns > 0 ? virtual.columns : 2;
  const { startIndex, endIndex, windowed, offsetTop, totalHeight } = virtual;

  return (
    <div
      ref={virtual.containerRef}
      className="relative"
      style={windowed ? { height: totalHeight } : undefined}
    >
      <Grid
        ref={(el) => { virtual.windowRef.current = el; }}
        className="gap-2"
        columns={columns}
        style={windowed ? { position: "absolute", left: 0, right: 0, top: offsetTop } : undefined}
      >
        {videos.slice(startIndex, endIndex).map((v) => (
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
            <CardMenu onOpen={() => onMenu(v)} />
          </ClickableCard>
        ))}
      </Grid>
    </div>
  );
}

/** The compact list. Rows keep one line of title so the shared row windowing
 *  still measures a single uniform height. */
export function VideoList({
  videos,
  onOpen,
  onMenu,
}: {
  videos: Video[];
  onOpen: (v: Video) => void;
  onMenu: (v: Video) => void;
}) {
  const virtual = useVirtualRows({
    count: videos.length,
    fixedColumns: 1,
    measureKey: "cinematic-list",
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
        className="flex flex-col gap-2"
        style={windowed ? { position: "absolute", left: 0, right: 0, top: offsetTop } : undefined}
      >
        {videos.slice(startIndex, endIndex).map((v) => {
          const meta = v.status === "missing" && !v.volumeOffline ? "找不到文件" : videoMeta(v);
          return (
            <ListItem
              className={cn(
                "rounded-[var(--radius)] border border-white/[0.07] bg-[var(--card)]/55 p-2",
              )}
              description={
                <span className="flex h-4 min-w-0 items-center gap-1 text-[11px] text-[var(--muted)]">
                  <span className="truncate">{meta}</span>
                  {v.mosaicRestored ? <Badge className={compactBadgeClass} label="马赛克破解" variant="purple" /> : null}
                  {v.backedUp ? <Badge className={compactBadgeClass} label="已备份" variant="success" /> : null}
                </span>
              }
              endContent={
                <span className="flex shrink-0 items-center gap-1">
                  {v.likeCount > 0 ? (
                    <span className="inline-flex items-center gap-0.5 text-[11px] text-[var(--muted)] tabular-nums">
                      <Icon className="size-2.5" name="heart" />
                      {v.likeCount}
                    </span>
                  ) : null}
                  <Button
                    aria-label="更多操作"
                    className="size-8 rounded-full p-0 text-[var(--muted)]"
                    onClick={(event) => {
                      event.stopPropagation();
                      onMenu(v);
                    }}
                    size="sm"
                    variant="ghost"
                  >
                    <Icon className="size-4" name="more-horizontal" />
                  </Button>
                </span>
              }
              key={v.id}
              label={videoName(v)}
              onClick={() => onOpen(v)}
              startContent={
                <span className="relative aspect-video w-32 shrink-0 overflow-hidden rounded-[7px] bg-[#101013]">
                  <Thumb v={v} />
                  <StatusBadges v={v} />
                </span>
              }
            />
          );
        })}
      </List>
    </div>
  );
}
