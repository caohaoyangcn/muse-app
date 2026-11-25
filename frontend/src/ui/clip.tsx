import * as React from "react";
import { Play } from "lucide-react";

import { cn } from "../lib/utils";
import { IconButton } from "./astryx";

/**
 * 静音循环播放的短片段。
 *
 * 时间段精彩时刻曾经是 GIF（一个 `<img>` 就够了），现在是 MP4，需要 `<video>`
 * 表现得像一个 GIF：
 *
 * - `muted`：浏览器只自动播放静音媒体，这是自动播放的前提；
 * - `loop`：片段要一直循环；
 * - `playsInline`：iOS 上没有它就会抢全屏播放；
 * - `preload="metadata"`：列表里可能同时有十几个，先只取时长与首帧。
 *
 * 另外处理两件裸 `<video>` 会做错的事：
 *
 * 1. **自动播放可能被拒绝**（iOS 低电量模式、部分隐私设置）。`play()` 被拒绝时
 *    在画面上盖一个播放按钮，用户点一下就能看——而不是留一个永远不动的第一帧。
 * 2. **离屏的片段不该继续解码**。网格里同时播放十个 60 fps 视频是白烧 CPU 和电池，
 *    所以可见性由 IntersectionObserver 决定：滚进视口才播，滚出去就暂停。
 */
export function LoopingClip({
  src,
  label,
  className,
  fit = "contain",
  /** 生成器不写出轨道信息时，用这个尺寸占位，避免加载前后布局跳动。 */
  aspect = "aspect-video",
}: {
  src: string;
  /** 无障碍名称：读屏软件用它描述这段片段是什么。 */
  label: string;
  className?: string;
  /** `contain` 保留整帧（默认），`cover` 允许裁切填满容器。 */
  fit?: "contain" | "cover";
  /** 占位比例；传 `null` 表示由调用方给高度（大图查看器那种整屏场景）。 */
  aspect?: string | null;
}) {
  const videoRef = React.useRef<HTMLVideoElement | null>(null);
  const [blocked, setBlocked] = React.useState(false);

  React.useEffect(() => {
    const video = videoRef.current;
    if (!video) return undefined;

    setBlocked(false);
    let visible = true;
    const attempt = () => {
      if (!visible) return;
      const started = video.play();
      // 老浏览器返回 undefined；现代浏览器返回 Promise，失败即"被拒绝自动播放"。
      if (started && typeof started.then === "function") {
        started.then(() => setBlocked(false)).catch(() => setBlocked(true));
      }
    };

    let observer: IntersectionObserver | null = null;
    if (typeof IntersectionObserver === "undefined") {
      attempt();
    } else {
      observer = new IntersectionObserver(
        (entries) => {
          visible = entries.some((entry) => entry.isIntersecting);
          if (visible) attempt();
          else video.pause();
        },
        { threshold: 0.25 },
      );
      observer.observe(video);
    }

    return () => {
      observer?.disconnect();
      video.pause();
    };
  }, [src]);

  const start = React.useCallback(() => {
    const video = videoRef.current;
    if (!video) return;
    void video.play().then(() => setBlocked(false)).catch(() => setBlocked(true));
  }, []);

  return (
    <div
      className={cn(
        "relative w-full overflow-hidden bg-[#111114]",
        aspect === null ? "h-full" : aspect,
        className,
      )}
    >
      <video
        ref={videoRef}
        aria-label={label}
        className={cn(
          "block h-full w-full bg-[#111114]",
          fit === "cover" ? "object-cover" : "object-contain",
        )}
        loop
        muted
        playsInline
        preload="metadata"
        src={src}
      />
      {blocked && (
        <div className="absolute inset-0 flex items-center justify-center bg-black/25">
          <IconButton
            icon={<Play className="size-5" />}
            label="播放片段"
            onClick={start}
            tooltip="播放片段"
          />
        </div>
      )}
    </div>
  );
}
