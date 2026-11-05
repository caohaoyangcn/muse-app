import { useRef, useState } from "react";
import type { Video } from "../types";
import MetadataPopup from "./MetadataPopup";
import BackupBadge from "./BackupBadge";
import RestoredBadge from "./RestoredBadge";
import { formatDuration, formatFileSize, videoCoverURL, videoResolution } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import { cn } from "../lib/utils";

const POPUP_DELAY_MS = 300;

export default function VideoCard({
  video,
  selected,
  onToggleSelect,
  onOpen,
  onPlay,
  onContextMenu,
  onDragStart,
  onLike,
}: {
  video: Video;
  selected: boolean;
  onToggleSelect: (id: number) => void;
  onOpen: (video: Video) => void;
  onPlay: (video: Video) => void;
  onContextMenu: (e: React.MouseEvent, video: Video) => void;
  onDragStart: (e: React.DragEvent, video: Video) => void;
  onLike: (id: number) => void;
}) {
  // A favorite on an unplugged drive is not a missing file: the card keeps its
  // thumbnail and metadata, and the only addition is the warning badge. The
  // detached volume takes precedence over a stale "missing" status.
  const offline = video.volumeOffline === true;
  const remote = video.sourceId > 0;
  const sourceOffline = remote && video.sourceOffline === true;
  const missing = !offline && video.status === "missing";
  const hasThumb = !missing && video.thumbnailPath !== "";
  const duration = formatDuration(video.durationMs);
  const meta = [duration, videoResolution(video), formatFileSize(video.fileSize)]
    .filter(Boolean)
    .join(" · ");

  // Hover popup: show only after POPUP_DELAY_MS, so a quick sweep over a card
  // never flashes the metadata tooltip. Leaving the card cancels the timer.
  const [showMeta, setShowMeta] = useState(false);
  const hoverTimer = useRef<number | null>(null);
  const cardRef = useRef<HTMLDivElement>(null);
  const cancelHoverTimer = () => {
    if (hoverTimer.current != null) {
      window.clearTimeout(hoverTimer.current);
      hoverTimer.current = null;
    }
  };
  const onMouseEnter = () => {
    cancelHoverTimer();
    hoverTimer.current = window.setTimeout(() => setShowMeta(true), POPUP_DELAY_MS);
  };
  const onMouseLeave = () => {
    cancelHoverTimer();
    setShowMeta(false);
  };

  return (
    <div
      ref={cardRef}
      className={cn(
        "group relative cursor-pointer overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)] transition hover:-translate-y-0.5 hover:border-[var(--accent)]",
        missing && "opacity-75",
        selected && "border-[var(--accent)] shadow-[0_0_0_1px_var(--accent),0_4px_16px_rgba(124,92,255,0.25)]",
      )}
      draggable
      data-video-id={video.id}
      onClick={() => onOpen(video)}
      onDoubleClick={() => onPlay(video)}
      onContextMenu={(e) => onContextMenu(e, video)}
      onDragStart={(e) => onDragStart(e, video)}
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <button
        type="button"
        className={cn(
          "absolute left-2 top-2 z-[5] flex h-5 w-5 items-center justify-center rounded-[5px] border border-white/55 bg-black/35 p-0 text-[13px] leading-none text-white opacity-0 transition hover:bg-[rgba(124,92,255,0.55)] group-hover:opacity-100",
          selected && "border-[var(--accent)] bg-[var(--accent)] opacity-100 hover:bg-[var(--accent-hover)]",
        )}
        aria-label={selected ? `取消选择 ${videoDisplayName(video)}` : `选择 ${videoDisplayName(video)}`}
        aria-pressed={selected}
        title={selected ? "取消选择" : "选择"}
        onClick={(e) => {
          e.stopPropagation();
          onToggleSelect(video.id);
        }}
        onDragStart={(e) => e.stopPropagation()}
        onMouseDown={(e) => e.stopPropagation()}
      >
        {selected ? "✓" : ""}
      </button>
      <div className="relative aspect-video bg-[#111114]">
        {hasThumb ? (
          <img className="block h-full w-full object-contain" src={videoCoverURL(video)} alt="" draggable={false} />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-[28px] opacity-50">
            {missing ? "⚠" : <span className="h-[22px] w-[22px] animate-spin rounded-full border-[3px] border-[var(--border)] border-t-[var(--accent)]" />}
          </div>
        )}
        {(duration || offline) && (
          <div className="absolute bottom-1.5 right-1.5 flex items-center gap-1">
            {duration && (
              <span className="rounded bg-black/75 px-1.5 py-0.5 text-[11px] text-white">{duration}</span>
            )}
            {offline && (
              <span
                className="rounded bg-[rgba(229,83,75,0.9)] px-2 py-0.5 text-[11px] text-white"
                title={`${video.volumeRoot || "磁盘"} 未接入：${video.path}`}
              >
                ⚠ 磁盘未接入
              </span>
            )}
          </div>
        )}
        {missing && <span className="absolute left-1.5 top-1.5 rounded bg-[rgba(229,83,75,0.9)] px-2 py-0.5 text-[11px] text-white">⚠ 文件缺失</span>}
        {remote && (
          <span
            className={cn(
              "absolute bottom-1.5 left-1.5 max-w-[calc(100%-16px)] truncate rounded px-2 py-0.5 text-[11px] text-white",
              sourceOffline ? "bg-[rgba(224,182,79,0.92)] text-black" : "bg-black/70",
            )}
            title={`${video.sourceName || "网盘"}${sourceOffline ? "（离线）" : ""}
${video.remotePath}`}
          >
            {sourceOffline ? "⚠ 网盘离线" : `☁ ${video.sourceName || "网盘"}`}
          </span>
        )}
      </div>
      <div className="px-3 pb-3 pt-2.5">
        <div className="flex min-w-0 items-center gap-1.5">
          <div className="truncate font-medium">{videoDisplayName(video)}</div>
          {video.mosaicRestored && <RestoredBadge />}
          {video.backedUp && <BackupBadge />}
        </div>
        <div className="mt-0.5 text-xs text-[var(--muted)]">
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
                  onLike(video.id);
                }}
                onMouseDown={(e) => e.stopPropagation()}
              >
                ♥ <span className="tabular-nums">{video.likeCount || 0}</span>
              </button>
            </>
          )}
        </div>
      </div>
      <MetadataPopup video={video} show={showMeta} anchorEl={cardRef.current} />
    </div>
  );
}
