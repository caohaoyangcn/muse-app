import type { Video } from "../types";
import type { DropdownMenuOption } from "../ui/astryx";

/** What the shell does with a card's overflow-menu action. */
export interface VideoMenuActions {
  onPlay: (video: Video) => void;
  onChoosePlayer: (video: Video) => void;
  onDetails: (video: Video) => void;
  onLike: (video: Video) => void;
  onEdit: (video: Video) => void;
  onRemove: (video: Video) => void;
}

/**
 * The mobile card menu as MoreMenu items.
 *
 * MoreMenu has no footer, so the old action sheet's "源视频不会被删除" note is
 * gone; the removal confirmation still states it. The like count moves into
 * the item label because menu rows have no description slot.
 */
export function videoMenuItems(video: Video, actions: VideoMenuActions): DropdownMenuOption[] {
  const offline = video.volumeOffline === true;
  const remote = video.sourceId > 0;
  const sourceOffline = remote && video.sourceOffline === true;
  const missing = video.status === "missing" && !offline && !sourceOffline;
  const unavailable = missing || offline || sourceOffline;
  const unavailableLabel = offline
    ? "播放（磁盘未接入）"
    : sourceOffline
      ? "播放（网盘离线）"
      : missing
        ? "播放（文件缺失）"
        : "播放";

  return [
    {
      label: unavailable ? unavailableLabel : "播放",
      isDisabled: unavailable,
      onClick: () => actions.onPlay(video),
    },
    {
      label: remote ? "播放器选项（网盘不支持）" : "播放器选项",
      isDisabled: unavailable || remote,
      onClick: () => actions.onChoosePlayer(video),
    },
    { label: "查看详情", onClick: () => actions.onDetails(video) },
    {
      label: video.likeCount > 0 ? `点赞（${video.likeCount} 次）` : "点赞",
      onClick: () => actions.onLike(video),
    },
    { label: "编辑信息", onClick: () => actions.onEdit(video) },
    { type: "divider" },
    {
      label: "移入回收站",
      variant: "destructive",
      onClick: () => actions.onRemove(video),
    },
  ];
}
