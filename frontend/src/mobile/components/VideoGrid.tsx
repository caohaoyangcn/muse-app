import type { CardSize, Video } from "../../types";
import { formatDuration } from "../../utils/format";
import { scaleBetween } from "../../utils/cardSize";
import { useVirtualRows } from "../../hooks/useVirtualRows";
import BackupBadge from "../../components/BackupBadge";
import RestoredBadge from "../../components/RestoredBadge";
import { coverURL } from "../../services/http";
import { MOBILE_CARD_SIZE } from "../prefs";
import { videoMeta, videoName } from "../videoDisplay";

function Thumb({ v }: { v: Video }) {
  if (v.status === "missing" && !v.volumeOffline) {
    return (
      <div className="flex size-full items-center justify-center bg-[#111114] text-2xl opacity-60">
        ⚠
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

function Badges({ v }: { v: Video }) {
  const duration = formatDuration(v.durationMs);
  // A detached drive is not a missing file: the card stays normal and carries
  // only the warning (see the desktop VideoCard).
  const offline = v.volumeOffline === true;
  const sourceOffline = v.sourceId > 0 && v.sourceOffline === true;
  const missing = !offline && v.status === "missing";
  return (
    <>
      {offline || sourceOffline ? (
        // One chip only: the narrow mobile card has no room for the duration
        // beside the warning, and the warning is the more important of the two.
        <span className="absolute bottom-1.5 right-1.5 rounded bg-[var(--danger)]/90 px-2 py-0.5 text-[11px] text-white">
          {offline ? "⚠ 磁盘未接入" : "⚠ 网盘离线"}
        </span>
      ) : duration ? (
        <span className="absolute bottom-1.5 right-1.5 rounded bg-black/75 px-1.5 py-0.5 text-[11px] text-white">
          {duration}
        </span>
      ) : null}
      {missing ? (
        <span className="absolute left-1.5 top-1.5 rounded bg-[var(--danger)]/90 px-2 py-0.5 text-[11px] text-white">
          ⚠ 缺失
        </span>
      ) : null}
      {v.likeCount > 0 ? (
        <span className="absolute bottom-1.5 left-1.5 rounded bg-black/75 px-1.5 py-0.5 text-[11px] text-white">
          ♥ {v.likeCount}
        </span>
      ) : null}
    </>
  );
}

/** The per-card overflow button; the mobile replacement for right-click. */
function MenuButton({ onOpen }: { onOpen: () => void }) {
  return (
    <button
      aria-label="更多操作"
      className="absolute right-1 top-1 flex size-8 items-center justify-center rounded-full bg-black/60 text-base leading-none text-white backdrop-blur-sm active:bg-black/80"
      onClick={(e) => {
        e.stopPropagation();
        onOpen();
      }}
      type="button"
    >
      ⋯
    </button>
  );
}

/**
 * Fixed-width card grid. The slider sets the exact track width so every step
 * changes the cards, not only the column count; leftover width is centred.
 */
export function VideoGrid({
  videos,
  onOpen,
  onMenu,
  cardSize,
}: {
  videos: Video[];
  onOpen: (v: Video) => void;
  onMenu: (v: Video) => void;
  cardSize: CardSize;
}) {
  const virtual = useVirtualRows({
    count: videos.length,
    minColumnWidth: cardSize,
    measureKey: cardSize,
  });
  const { startIndex, endIndex, columns, windowed, offsetTop, totalHeight } = virtual;
  const gap = Math.round(scaleBetween(cardSize, MOBILE_CARD_SIZE, 10, 14));

  return (
    <div
      ref={virtual.containerRef}
      className="relative"
      style={windowed ? { height: totalHeight } : undefined}
    >
      <div
        ref={virtual.windowRef}
        className="grid"
        style={{
          gridTemplateColumns: `repeat(${columns > 0 ? columns : "auto-fill"}, min(${cardSize}px, 100%))`,
          justifyContent: "center",
          columnGap: gap,
          rowGap: gap,
          ...(windowed ? { position: "absolute", top: offsetTop, left: 0, right: 0 } : null),
        }}
      >
        {videos.slice(startIndex, endIndex).map((v) => (
          <button
            className="relative overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)] text-left active:scale-[0.98]"
            key={v.id}
            onClick={() => onOpen(v)}
            type="button"
          >
            <div className="relative aspect-video bg-[#111114]">
              <Thumb v={v} />
              <Badges v={v} />
              <MenuButton onOpen={() => onMenu(v)} />
            </div>
            <div className="px-2.5 pb-2.5 pt-2">
              <div className="flex min-w-0 items-center gap-1.5">
                <div className="truncate text-[13px] font-medium">{videoName(v)}</div>
                {v.mosaicRestored && <RestoredBadge />}
              {v.backedUp && <BackupBadge />}
                {v.backedUp && <BackupBadge />}
              </div>
              <div className="truncate text-xs text-[var(--muted)]">
                {v.status === "missing" && !v.volumeOffline ? "找不到文件" : videoMeta(v)}
              </div>
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

export function VideoList({
  videos,
  onOpen,
  onMenu,
  cardSize,
}: {
  videos: Video[];
  onOpen: (v: Video) => void;
  onMenu: (v: Video) => void;
  cardSize: CardSize;
}) {
  // The list has no column width to derive from, so the card width is scaled
  // into a row thumbnail and clamped to keep text readable.
  const thumbWidth = Math.min(150, Math.max(72, Math.round(cardSize * 0.6)));
  const virtual = useVirtualRows({
    count: videos.length,
    fixedColumns: 1,
    measureKey: cardSize,
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
          <button
            className="relative flex items-center gap-3 overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)] p-2 pr-11 text-left active:scale-[0.99]"
            key={v.id}
            onClick={() => onOpen(v)}
            type="button"
          >
            <div
              className="relative aspect-video shrink-0 overflow-hidden rounded-md bg-[#111114]"
              style={{ width: thumbWidth }}
            >
              <Thumb v={v} />
              <Badges v={v} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex min-w-0 items-center gap-1.5">
                <div className="truncate text-[14px] font-medium">{videoName(v)}</div>
                {v.mosaicRestored && <RestoredBadge />}
              {v.backedUp && <BackupBadge />}
                {v.backedUp && <BackupBadge />}
              </div>
              <div className="truncate text-xs text-[var(--muted)]">
                {v.status === "missing" && !v.volumeOffline ? "找不到文件" : videoMeta(v)}
              </div>
            </div>
            <MenuButton onOpen={() => onMenu(v)} />
          </button>
        ))}
      </div>
    </div>
  );
}
