import type { Video } from "../../types";
import { videoName } from "../videoDisplay";
import { BottomSheet, SheetAction } from "./Sheet";

/**
 * The card overflow menu. The desktop equivalent is the right-click
 * ContextMenu; on touch the same actions get a bottom sheet and a plain tap.
 * Desktop-only actions (open in system player, reveal in file manager, copy
 * path) are intentionally absent — they act on the server's filesystem, not the
 * phone.
 */
export function VideoActionSheet({
  video,
  open,
  onDetails,
  onPlay,
  onChoosePlayer,
  onLike,
  onEdit,
  onRemove,
  onClose,
}: {
  video: Video | null;
  open: boolean;
  onDetails: () => void;
  onPlay: () => void;
  onChoosePlayer: () => void;
  onLike: () => void;
  onEdit: () => void;
  onRemove: () => void;
  onClose: () => void;
}) {
  const offline = video?.volumeOffline === true;
  const remote = (video?.sourceId ?? 0) > 0;
  const sourceOffline = remote && video?.sourceOffline === true;
  const missing = video?.status === "missing" && !offline && !sourceOffline;
  const unavailable = missing || offline || sourceOffline;
  return (
    <BottomSheet onClose={onClose} open={open && !!video} title={video ? videoName(video) : undefined}>
      <SheetAction disabled={unavailable} label="播放" onClick={onPlay} />
      <SheetAction disabled={unavailable || remote} label="播放器选项" onClick={onChoosePlayer} />
      <SheetAction label="查看详情" onClick={onDetails} />
      <SheetAction
        label="点赞"
        hint={video && video.likeCount > 0 ? String(video.likeCount) : undefined}
        onClick={onLike}
      />
      <SheetAction label="编辑信息" onClick={onEdit} />
      <SheetAction danger label="移入回收站" onClick={onRemove} />
      {offline ? (
        <p className="px-3 py-2 text-xs text-[var(--muted)]">
          磁盘 {video?.volumeRoot || ""} 未接入，插回移动硬盘后即可播放；移入回收站后可恢复；源视频不会被删除。
        </p>
      ) : sourceOffline ? (
        <p className="px-3 py-2 text-xs text-[var(--muted)]">
          网盘「{video?.sourceName || "连接"}」当前离线，恢复后即可播放；移入回收站后可恢复；网盘文件不会被删除。
        </p>
      ) : missing ? (
        <p className="px-3 py-2 text-xs text-[var(--muted)]">
          文件缺失，无法播放；移入回收站后可恢复；源视频不会被删除。
        </p>
      ) : null}
    </BottomSheet>
  );
}
