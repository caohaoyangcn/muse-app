import type { Video } from "../../types";
import { useVirtualRows } from "../../hooks/useVirtualRows";
import { cn } from "../../lib/utils";
import BackupBadge from "../../ui/backup-badge";
import { NavIcon } from "../../ui/NavIcon";
import RestoredBadge from "../../ui/restored-badge";
import { coverURL } from "../../services/web";
import { videoMeta, videoName } from "../videoDisplay";

/** The poster ratio. Every card shares it, so a row always has one height
 *  regardless of how long the title is — the text sits over the artwork. */
const POSTER_ASPECT = "3 / 4";

function Thumb({ v }: { v: Video }) {
  if (v.status === "missing" && !v.volumeOffline) {
    return (
      <div className="flex size-full items-center justify-center bg-[#101013] text-[var(--danger)]">
        <NavIcon className="size-7 opacity-80" name="alert" />
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
      {missing ? (
        <span className="rounded-[5px] bg-[var(--danger)]/95 px-1.5 py-1 text-[10px] font-medium leading-none text-white">
          文件缺失
        </span>
      ) : null}
      {offline ? (
        <span className="rounded-[5px] bg-[var(--danger)]/95 px-1.5 py-1 text-[10px] font-medium leading-none text-white">
          磁盘未接入
        </span>
      ) : null}
      {sourceOffline ? (
        <span className="rounded-[5px] bg-[#e0b64f] px-1.5 py-1 text-[10px] font-medium leading-none text-black">
          网盘离线
        </span>
      ) : null}
    </div>
  );
}

function CardMenu({ onOpen, centered }: { onOpen: () => void; centered?: boolean }) {
  return (
    <button
      aria-label="更多操作"
      className={cn(
        "absolute right-2 z-20 flex size-8 items-center justify-center rounded-full bg-black/55 text-white active:bg-black/80",
        centered ? "top-1/2 -translate-y-1/2" : "top-2",
      )}
      onClick={(e) => {
        e.stopPropagation();
        onOpen();
      }}
      type="button"
    >
      <NavIcon className="size-4" name="more-horizontal" />
    </button>
  );
}

function OverlayMeta({ v }: { v: Video }) {
  const meta = v.status === "missing" && !v.volumeOffline ? "找不到文件" : videoMeta(v);
  return (
    <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black via-black/70 to-transparent px-2.5 pb-2.5 pt-10">
      {v.mosaicRestored || v.backedUp ? (
        <div className="mb-1 flex flex-wrap gap-1">
          {v.mosaicRestored ? <RestoredBadge /> : null}
          {v.backedUp ? <BackupBadge /> : null}
        </div>
      ) : null}
      <div className="line-clamp-2 text-[13px] font-semibold leading-[1.25] text-white [text-shadow:0_1px_8px_rgba(0,0,0,0.55)]">
        {videoName(v)}
      </div>
      <div className="mt-1 flex min-w-0 items-center gap-1.5 text-[10.5px] leading-none text-white/65">
        <span className="truncate">{meta}</span>
        {v.likeCount > 0 ? (
          <span className="ml-auto inline-flex shrink-0 items-center gap-0.5 tabular-nums">
            <NavIcon className="size-2.5" name="heart" />
            {v.likeCount}
          </span>
        ) : null}
      </div>
    </div>
  );
}

/**
 * The home poster wall. A portrait phone gets two full-width columns; wider
 * viewports add tracks rather than stretching a single card. The artwork is the
 * card — the title and metadata sit on a scrim over the bottom of the poster,
 * so every card is exactly the same shape and the screen reads as a shelf of
 * covers rather than a list of boxes.
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
  // Portrait phones settle on two columns; the track floor keeps that default
  // while a tablet or landscape phone earns more columns. Before the first
  // measurement the fallback is the two-column phone layout.
  const columns = virtual.columns > 0 ? virtual.columns : 2;
  const { startIndex, endIndex, windowed, offsetTop, totalHeight } = virtual;

  return (
    <div
      ref={virtual.containerRef}
      className="relative"
      style={windowed ? { height: totalHeight } : undefined}
    >
      <div
        ref={virtual.windowRef}
        className="grid gap-2.5"
        style={{
          gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
          ...(windowed ? { position: "absolute", top: offsetTop, left: 0, right: 0 } : null),
        }}
      >
        {videos.slice(startIndex, endIndex).map((v) => (
          <article className="relative" key={v.id}>
            <button
              className="block w-full overflow-hidden rounded-[var(--radius)] border border-white/[0.07] bg-[#101013] text-left transition-transform duration-150 active:scale-[0.985]"
              onClick={() => onOpen(v)}
              type="button"
            >
              <div className="relative" style={{ aspectRatio: POSTER_ASPECT }}>
                <Thumb v={v} />
                <StatusBadges v={v} />
                <OverlayMeta v={v} />
              </div>
            </button>
            <CardMenu onOpen={() => onMenu(v)} />
          </article>
        ))}
      </div>
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
      <div
        ref={virtual.windowRef}
        className="flex flex-col gap-2"
        style={windowed ? { position: "absolute", top: offsetTop, left: 0, right: 0 } : undefined}
      >
        {videos.slice(startIndex, endIndex).map((v) => (
          <article className="relative" key={v.id}>
            <button
              className="flex w-full items-center gap-3 rounded-[var(--radius)] border border-white/[0.07] bg-[var(--card)]/55 p-2 pr-11 text-left active:scale-[0.99]"
              onClick={() => onOpen(v)}
              type="button"
            >
              <div className="relative aspect-video w-[42%] shrink-0 overflow-hidden rounded-[7px] bg-[#101013]">
                <Thumb v={v} />
                <StatusBadges v={v} />
              </div>
              <div className="min-w-0 flex-1 py-0.5">
                <div className="flex min-w-0 items-center gap-1.5">
                  <div className="truncate text-[14px] font-medium">{videoName(v)}</div>
                  {v.mosaicRestored && <RestoredBadge />}
                  {v.backedUp && <BackupBadge />}
                </div>
                <div className="mt-1 flex min-w-0 items-center gap-1 text-[11px] text-[var(--muted)]">
                  <span className="truncate">
                    {v.status === "missing" && !v.volumeOffline ? "找不到文件" : videoMeta(v)}
                  </span>
                  {v.likeCount > 0 ? (
                    <span className="inline-flex shrink-0 items-center gap-0.5 tabular-nums">
                      <NavIcon className="size-2.5" name="heart" />
                      {v.likeCount}
                    </span>
                  ) : null}
                </div>
              </div>
            </button>
            <CardMenu centered onOpen={() => onMenu(v)} />
          </article>
        ))}
      </div>
    </div>
  );
}
