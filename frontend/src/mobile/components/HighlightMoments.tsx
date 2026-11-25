import { useState } from "react";

import type { Highlight } from "../../types";
import { highlightURL } from "../../utils/format";
import {
  highlightLabel,
  isHighlightReady,
  mobileVisibleHighlights,
} from "../../utils/highlightTime";
import { useVideoHighlights } from "../../views/highlights/useVideoHighlights";
import { ClickableCard, Spinner } from "../../ui/astryx";
import { Dialog, DialogCloseButton, DialogContent } from "../../ui/dialog";

/** 生成中的占位文案：时间段是 GIF，时刻是单帧截图。 */
function pendingLabel(h: Highlight): string {
  if (h.status === "queued") return "排队中";
  return h.kind === "range" ? "正在生成 GIF…" : "正在截取…";
}

/**
 * 详情页的「精彩时刻」（手机尺寸）。
 *
 * 只读：新增、重试、删除都留在桌面端，所以失败条目干脆不显示——一张点不动的
 * 失败卡只会让人以为手机上能处理。失败计数仍在桌面端的任务看板里。
 *
 * 浏览器收不到桌面端的事件，所以这里用 `pollWhilePending: true`：只有存在排队/
 * 生成中的条目时才每 3 秒重读一次（见 useVideoHighlights）。没有任何可见条目时
 * 整段不渲染——空的标题比没有这一段更难看。
 */
export function HighlightMoments({ videoId }: { videoId: number }) {
  const { items } = useVideoHighlights(videoId, { pollWhilePending: true });
  const visible = mobileVisibleHighlights(items);
  const [preview, setPreview] = useState<Highlight | null>(null);

  if (visible.length === 0) return null;

  // 轮询每次都会换一批对象，所以大图跟着"这一条还在不在"走，而不是跟着某个旧
  // 对象走：桌面端删掉之后，手机上开着的大图会自己关掉，而不是继续显示一张 404。
  const opened = preview ? visible.find((h) => h.id === preview.id) ?? null : null;

  return (
    <section className="mt-6 border-t border-[var(--border)] pt-4">
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="text-sm font-semibold">精彩时刻</h3>
        <span className="text-xs text-[var(--muted)]">{visible.length} 条</span>
      </div>

      <div className="mt-3 grid grid-cols-2 gap-2">
        {visible.map((h) => (
          <div
            className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)]"
            key={h.id}
          >
            {isHighlightReady(h) ? (
              <ClickableCard
                className="block w-full"
                label={`查看 ${highlightLabel(h)}`}
                onClick={() => setPreview(h)}
                padding={0}
              >
                <img
                  alt={highlightLabel(h)}
                  className="block aspect-video w-full bg-[#111114] object-contain"
                  loading="lazy"
                  src={highlightURL(h)}
                />
              </ClickableCard>
            ) : (
              <div className="flex aspect-video w-full items-center justify-center bg-[#111114]">
                <Spinner label={pendingLabel(h)} size="sm" />
              </div>
            )}
            <div className="px-2 py-1.5 text-xs text-[var(--muted)]">
              <span className="block truncate" title={highlightLabel(h)}>
                {highlightLabel(h)}
              </span>
            </div>
          </div>
        ))}
      </div>

      <Dialog
        label={opened ? highlightLabel(opened) : "精彩时刻"}
        onOpenChange={(next) => !next && setPreview(null)}
        open={opened !== null}
        padding={0}
        width="100vw"
      >
        <DialogContent>
          <div className="relative flex h-[100dvh] w-full items-center justify-center bg-black">
            <DialogCloseButton />
            {opened && (
              <img
                alt={highlightLabel(opened)}
                className="max-h-full max-w-full object-contain"
                src={highlightURL(opened)}
              />
            )}
          </div>
        </DialogContent>
      </Dialog>
    </section>
  );
}
