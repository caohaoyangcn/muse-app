import { useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { Video } from "../types";
import {
  formatBitrate,
  formatDuration,
  formatFileSize,
  formatFrameRate,
  videoResolution,
} from "../utils/format";
import { cn } from "../lib/utils";
import { videoDisplayName } from "../utils/videoName";
import RestoredBadge from "./RestoredBadge";

const GAP = 8;

/**
 * Hover tooltip that shows a video file's metadata.
 */
export default function MetadataPopup({
  video,
  show,
  anchorEl,
}: {
  video: Video;
  show: boolean;
  anchorEl: HTMLElement | null;
}) {
  const rows: [string, string][] = (
    [
      ["路径", video.path],
      ["时长", formatDuration(video.durationMs)],
      ["分辨率", videoResolution(video)],
      ["帧率", formatFrameRate(video.frameRate)],
      ["比特率", formatBitrate(video.bitrate)],
      ["大小", formatFileSize(video.fileSize)],
    ] as [string, string][]
  ).filter((e) => e[1] !== "");

  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ left: number; top: number }>({ left: 0, top: 0 });
  const [placed, setPlaced] = useState(false);

  useLayoutEffect(() => {
    if (!show) {
      setPlaced(false);
      return;
    }
    if (!anchorEl || !ref.current) return;
    const anchor = anchorEl.getBoundingClientRect();
    const el = ref.current.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    let top = anchor.bottom + GAP;
    if (top + el.height > vh - 4) top = Math.max(4, anchor.top - GAP - el.height);
    let left = anchor.left;
    if (left + el.width > vw - 4) left = Math.max(4, vw - el.width - 4);
    setPos({ left, top });
    setPlaced(true);
  }, [show, anchorEl, rows.length]);

  if (!anchorEl) return null;

  const visible = show && placed;
  return createPortal(
    <div
      ref={ref}
      className={cn(
        "fixed z-40 min-w-[230px] max-w-[340px] rounded-[var(--radius)] border border-[var(--border)] bg-[rgba(30,30,37,0.97)] p-3.5 opacity-0 shadow-[0_10px_30px_rgba(0,0,0,0.5)] transition duration-300 pointer-events-none",
        visible && "translate-y-0 opacity-100",
        !visible && "invisible -translate-y-1",
      )}
      style={{ left: pos.left, top: pos.top }}
    >
      <div className="mb-2 flex min-w-0 items-center gap-1.5">
        <div className="min-w-0 break-all font-semibold">{videoDisplayName(video)}</div>
        {video.mosaicRestored && <RestoredBadge />}
      </div>
      <dl className="m-0">
        {rows.map(([k, v]) => (
          <div className="flex gap-2.5 py-0.5 text-xs" key={k}>
            <dt className="w-[54px] shrink-0 whitespace-nowrap text-[var(--muted)]">{k}</dt>
            <dd className="m-0 break-all whitespace-pre-wrap text-[var(--text)]">{v}</dd>
          </div>
        ))}
      </dl>
    </div>,
    document.body,
  );
}
