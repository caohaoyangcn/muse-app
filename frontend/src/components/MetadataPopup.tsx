import type { Video } from "../types";
import {
  formatBitrate,
  formatDuration,
  formatFileSize,
  formatFrameRate,
  videoResolution,
} from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import { Badge } from "../ui/astryx";

/**
 * Hover tooltip that shows a video file's metadata.
 */
export default function MetadataPopup({ video }: { video: Video }) {
  const remote = video.sourceId > 0;
  const rows: [string, string][] = (
    [
      [remote ? "网盘" : "路径", remote ? `${video.sourceName || "网盘"} · ${video.remotePath}` : video.path],
      ["时长", formatDuration(video.durationMs)],
      ["分辨率", videoResolution(video)],
      ["帧率", formatFrameRate(video.frameRate)],
      ["比特率", formatBitrate(video.bitrate)],
      ["大小", formatFileSize(video.fileSize)],
    ] as [string, string][]
  ).filter((e) => e[1] !== "");

  return (
    <div className="pointer-events-none min-w-[230px] max-w-[340px] rounded-[var(--radius)] border border-[var(--border)] bg-[rgba(30,30,37,0.97)] p-3.5 shadow-[0_10px_30px_rgba(0,0,0,0.5)]">
      <div className="mb-2 flex min-w-0 items-center gap-1.5">
        <div className="min-w-0 break-all font-semibold">{videoDisplayName(video)}</div>
        {video.mosaicRestored && <Badge label="马赛克破解" variant="purple" />}
        {video.backedUp && <Badge label="已备份" variant="success" />}
      </div>
      <dl className="m-0">
        {rows.map(([k, v]) => (
          <div className="flex gap-2.5 py-0.5 text-xs" key={k}>
            <dt className="w-[54px] shrink-0 whitespace-nowrap text-[var(--muted)]">{k}</dt>
            <dd className="m-0 break-all whitespace-pre-wrap text-[var(--text)]">{v}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
