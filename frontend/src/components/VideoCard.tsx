import type { Video } from "../types";
import MetadataPopup from "./MetadataPopup";
import { formatDuration, formatFileSize, videoCoverURL, videoResolution } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import { cn } from "../lib/utils";
import {
  AspectRatio,
  Badge,
  CheckboxInput,
  ClickableCard,
  HoverCard,
  Spinner,
  Text,
  compactBadgeClass,
} from "../ui/astryx";
import { Button } from "../ui/button";
import { Icon } from "../ui/icon";

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

  return (
    <HoverCard
      content={<MetadataPopup video={video} />}
      delay={300}
      hideDelay={0}
      label="视频信息"
      placement="below"
      touchTrigger="none"
    >
    <ClickableCard
      className={cn(
        "group relative overflow-hidden transition hover:-translate-y-0.5 hover:border-[var(--accent)]",
        "scroll-mt-28",
        missing && "opacity-75",
        selected &&
          "border-[var(--accent)] shadow-[0_0_0_1px_var(--accent),0_4px_16px_rgba(124,92,255,0.25)]",
      )}
      data-video-id={video.id}
      draggable
      label={videoDisplayName(video)}
      onClick={() => onOpen(video)}
      onContextMenu={(event) => onContextMenu(event, video)}
      onDoubleClick={() => onPlay(video)}
      onDragStart={(event) => onDragStart(event, video)}
      padding={0}
    >
      <span
        className={cn(
          "absolute left-2 top-2 z-[5] opacity-0 transition group-hover:opacity-100",
          selected && "opacity-100",
        )}
        onDragStart={(event) => event.stopPropagation()}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <CheckboxInput
          isLabelHidden
          label={selected ? `取消选择 ${videoDisplayName(video)}` : `选择 ${videoDisplayName(video)}`}
          onChange={() => onToggleSelect(video.id)}
          size="sm"
          value={selected}
        />
      </span>
      <AspectRatio ratio={16 / 9}>
        <div className="relative size-full bg-[#111114]">
          {hasThumb ? (
            <img
              alt=""
              className="block size-full object-contain"
              draggable={false}
              src={videoCoverURL(video)}
            />
          ) : (
            <div className="flex size-full items-center justify-center opacity-60">
              {missing ? (
                <Icon className="size-7 text-[var(--danger)]" name="alert" />
              ) : (
                <Spinner size="lg" shade="subtle" />
              )}
            </div>
          )}
          {(duration || offline) && (
            <div className="absolute bottom-1.5 right-1.5 flex items-center gap-1">
              {duration ? (
                <Badge
                  className={cn(compactBadgeClass, "border-transparent bg-black/75 text-white")}
                  label={duration}
                />
              ) : null}
              {offline ? <Badge label="磁盘未接入" variant="error" /> : null}
            </div>
          )}
          {missing ? <Badge label="文件缺失" variant="error" /> : null}
          {remote ? (
            <Badge
              className={cn(compactBadgeClass, "absolute bottom-1.5 left-1.5 max-w-[calc(100%-16px)]")}
              label={sourceOffline ? "网盘离线" : `${video.sourceName || "网盘"}`}
              variant={sourceOffline ? "warning" : "neutral"}
            />
          ) : null}
        </div>
      </AspectRatio>
      <div className="px-3 pb-3 pt-2.5">
        <div className="flex min-w-0 items-center gap-1.5">
          {/* Text truncates and shows the full name on hover, so a long 译名 is
              never lost just because the card is narrow. */}
          <div className="min-w-0 font-medium">
            <Text color="inherit" maxLines={1} type="inherit">{videoDisplayName(video)}</Text>
          </div>
          {video.mosaicRestored ? (
            <Badge className={compactBadgeClass} label="马赛克破解" variant="purple" />
          ) : null}
          {video.backedUp ? (
            <Badge className={compactBadgeClass} label="已备份" variant="success" />
          ) : null}
        </div>
        <div className="mt-0.5 text-xs text-[var(--muted)]">
          {missing ? (
            "找不到文件"
          ) : (
            <>
              {meta}
              <Button
                aria-label="点赞"
                className="ml-2 inline-flex h-5 items-center gap-0.5 px-1.5 text-xs leading-none text-[var(--muted)] hover:text-[var(--accent)]"
                icon={<Icon className="size-3" name="heart" />}
                onClick={(event) => {
                  event.stopPropagation();
                  onLike(video.id);
                }}
                size="sm"
                tooltip="点赞（可多次点赞）"
                variant="ghost"
              >
                <span className="tabular-nums">{video.likeCount || 0}</span>
              </Button>
            </>
          )}
        </div>
      </div>
    </ClickableCard>
    </HoverCard>
  );
}
