import { useState } from "react";

import { deleteVideoHighlight, retryVideoHighlight } from "../services/desktop";
import { invalidate } from "../services/live";
import type { Highlight, Video } from "../types";
import { highlightURL } from "../utils/format";
import { highlightLabel, isHighlightPending, isHighlightRange, isHighlightReady } from "../utils/highlightTime";
import { useVideoHighlights } from "../views/highlights/useVideoHighlights";
import { Banner, ClickableCard, Spinner } from "../ui/astryx";
import { Button } from "../ui/button";
import { LoopingClip } from "../ui/clip";
import { Dialog, DialogCloseButton, DialogContent } from "../ui/dialog";
import ConfirmDialog from "./ConfirmDialog";
import HighlightMomentDialog from "./HighlightMomentDialog";

/** 生成中的占位文案：时间段是视频，时刻是单帧截图。 */
function pendingLabel(h: Highlight): string {
  if (h.status === "queued") return "排队中";
  return h.kind === "range" ? "正在生成视频…" : "正在截取…";
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

/**
 * 详情页的「精彩时刻」：生成的截图与循环视频。
 *
 * 取数与状态在 `useVideoHighlights` 里（手机端共用），这里只负责桌面端的展示与
 * 写入口：新增、重试、删除都只有桌面能做（移动端是只读的局域网页面）。
 *
 * 桌面不轮询：`video:highlight:updated` 已经接进失效列表，生成完成会自动重读这
 * 一栏。读失败时用一个可重试的 Banner，而不是让整段消失——这里不像「类似影片」
 * 那样只是附属信息，用户可能正等着它。
 */
export default function HighlightMomentsSection({ video }: { video: Video }) {
  const { items, loading, error, reload } = useVideoHighlights(video.id);
  const [addOpen, setAddOpen] = useState(false);
  const [preview, setPreview] = useState<Highlight | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Highlight | null>(null);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  // 源文件读不到时仍然展示已生成的图片（它们是本地文件），只是不能再新增。
  // 文案与后端 worker 拒绝时给出的理由一致，用户不会在两个地方读到两种说法。
  const sourceUnavailableReason =
    video.volumeOffline
      ? "磁盘未接入，无法生成精彩时刻"
      : video.sourceOffline
        ? "网盘离线，无法生成精彩时刻"
        : video.status === "missing"
          ? "源文件缺失，无法生成精彩时刻"
          : undefined;

  const remove = async () => {
    if (!pendingDelete) return;
    const target = pendingDelete;
    setPendingDelete(null);
    setActionError(null);
    setBusyId(target.id);
    try {
      await deleteVideoHighlight(video.id, target.id);
      invalidate();
    } catch (e) {
      setActionError(errorText(e));
    } finally {
      setBusyId(null);
    }
  };

  const retry = async (h: Highlight) => {
    setActionError(null);
    setBusyId(h.id);
    try {
      await retryVideoHighlight(video.id, h.id);
      invalidate();
    } catch (e) {
      setActionError(errorText(e));
    } finally {
      setBusyId(null);
    }
  };

  return (
    <section className="border-t border-[var(--border)] pt-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold">精彩时刻</h3>
          <p className="mt-1 text-xs text-[var(--muted)]">
            {items.length > 0
              ? `${items.length} 条 · 截图与 1–20 秒循环视频`
              : "记录一个时刻生成截图，或记录 1–20 秒生成循环视频"}
          </p>
        </div>
        <Button
          disabled={sourceUnavailableReason !== undefined}
          onClick={() => setAddOpen(true)}
          size="sm"
          title={sourceUnavailableReason ?? "新增精彩时刻"}
          variant="primary"
        >
          新增精彩时刻
        </Button>
      </div>

      {sourceUnavailableReason && (
        <p className="mt-2 text-xs text-[var(--muted)]">{sourceUnavailableReason}</p>
      )}

      {error ? (
        <div className="mt-3">
          <Banner
            endContent={
              <Button onClick={reload} size="sm">
                重试
              </Button>
            }
            status="error"
            title={error}
          />
        </div>
      ) : null}

      {actionError ? (
        <div className="mt-3">
          <Banner status="error" title={actionError} />
        </div>
      ) : null}

      {loading && items.length === 0 ? (
        <div className="flex min-h-[120px] items-center justify-center">
          <Spinner label="正在读取精彩时刻…" />
        </div>
      ) : items.length === 0 ? (
        error ? null : <p className="mt-3 text-sm text-[var(--muted)]">还没有精彩时刻</p>
      ) : (
        <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {items.map((h) => (
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
                  {isHighlightRange(h) ? (
                    <LoopingClip label={highlightLabel(h)} src={highlightURL(h)} />
                  ) : (
                    <img
                      alt={highlightLabel(h)}
                      className="block aspect-video w-full bg-[#111114] object-contain"
                      loading="lazy"
                      src={highlightURL(h)}
                    />
                  )}
                </ClickableCard>
              ) : (
                <div className="flex aspect-video w-full flex-col items-center justify-center gap-1 bg-[#111114] px-3 text-center">
                  {h.status === "failed" ? (
                    <>
                      <span className="text-sm text-[var(--danger)]">生成失败</span>
                      {h.error && (
                        <span className="line-clamp-3 text-xs text-[var(--muted)]">{h.error}</span>
                      )}
                    </>
                  ) : (
                    <Spinner label={pendingLabel(h)} />
                  )}
                </div>
              )}

              <div className="flex items-center justify-between gap-2 px-2 py-1.5">
                <span className="min-w-0 truncate text-xs text-[var(--muted)]" title={highlightLabel(h)}>
                  {highlightLabel(h)}
                </span>
                <div className="flex shrink-0 items-center gap-1">
                  {h.status === "failed" && (
                    <Button
                      disabled={busyId === h.id}
                      onClick={() => void retry(h)}
                      size="sm"
                      variant="ghost"
                    >
                      重试
                    </Button>
                  )}
                  <Button
                    disabled={busyId === h.id}
                    onClick={() => setPendingDelete(h)}
                    size="sm"
                    title="删除这条精彩时刻（不影响源视频）"
                    variant="ghost"
                  >
                    删除
                  </Button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      <HighlightMomentDialog
        durationMs={video.durationMs}
        onClose={() => setAddOpen(false)}
        open={addOpen}
        sourceUnavailableReason={sourceUnavailableReason}
        videoId={video.id}
      />

      <Dialog
        label={preview ? highlightLabel(preview) : "精彩时刻"}
        onOpenChange={(next) => !next && setPreview(null)}
        open={preview !== null}
        width="min(1000px, calc(100vw - 48px))"
      >
        <DialogContent className="p-4">
          <DialogCloseButton />
          <div className="flex max-h-[calc(100vh-96px)] min-h-[240px] items-center justify-center overflow-auto rounded-[var(--radius)] bg-black/30 p-2">
            {preview &&
              (isHighlightRange(preview) ? (
                <LoopingClip
                  aspect={null}
                  className="h-[min(70vh,calc(100vh-160px))] rounded-[var(--radius)]"
                  fit="contain"
                  label={highlightLabel(preview)}
                  src={highlightURL(preview)}
                />
              ) : (
                <img
                  alt={highlightLabel(preview)}
                  className="max-h-[calc(100vh-128px)] max-w-full object-contain"
                  src={highlightURL(preview)}
                />
              ))}
          </div>
        </DialogContent>
      </Dialog>

      {pendingDelete && (
        <ConfirmDialog
          cancelLabel="取消"
          confirmLabel="删除"
          danger
          message={`${highlightLabel(pendingDelete)}：${
            isHighlightPending(pendingDelete)
              ? "删除会取消这次生成，"
              : "只删除这条生成的截图/视频记录与文件，"
          }源视频不受影响。`}
          onCancel={() => setPendingDelete(null)}
          onConfirm={() => void remove()}
          title="删除精彩时刻"
        />
      )}
    </section>
  );
}
