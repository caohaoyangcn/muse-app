import { useState } from "react";

import type { Video } from "../../types";
import { coverURL } from "../../services/http";
import { videoName } from "../videoDisplay";
import { formatFileSize } from "../../utils/format";
import { cn } from "../../lib/utils";
import { ConfirmSheet } from "../../ui/sheet";
import { EmptyState, ErrorState, Loading } from "./States";
import { remainingDays, useTrash } from "../../views/trash/useTrash";

/**
 * The phone's recycle bin. Removing a favorite anywhere (desktop or phone) only
 * moves it here; restoring it brings back the exact record with its groups,
 * likes and scraped metadata. 彻底删除 never touches the source video.
 */
export function TrashView({
  onToast,
}: {
  onToast: (message: string, tone?: "info" | "error") => void;
}) {
  const {
    mode,
    setMode,
    trashVideos,
    archivedVideos,
    videos,
    retentionDays,
    totalSize,
    loading,
    error,
    reload,
    busyId,
    emptying,
    clearingArchive,
    restore,
    purge,
    empty,
    clearArchive,
  } = useTrash((notice) =>
    onToast(
      // The phone has no detail rows, so the first detail rides along with the
      // message when there is one.
      notice.details?.length ? `${notice.text}：${notice.details[0]}` : notice.text,
      notice.kind === "error" ? "error" : "info",
    ),
  );

  // Which sheet is open is presentation, not data.
  const [target, setTarget] = useState<Video | null>(null);
  const [emptyOpen, setEmptyOpen] = useState(false);
  const [archiveOpen, setArchiveOpen] = useState(false);
  // The phone disables every row while one write runs.
  const busy = busyId !== null || emptying || clearingArchive;

  if (loading) return <Loading label={mode === "archive" ? "加载归档…" : "加载回收站…"} />;
  if (error && videos.length === 0) {
    return <ErrorState message={error} onRetry={reload} />;
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      <div className="border-b border-[var(--border)] px-4 py-2.5">
        <div className="flex items-center gap-1 text-xs" role="tablist" aria-label="归档与回收站">
          <button
            type="button"
            role="tab"
            aria-selected={mode === "archive"}
            className={cn(
              "rounded-full px-3 py-1.5",
              mode === "archive" ? "bg-[var(--panel-2)] text-[var(--text)]" : "text-[var(--muted)]",
            )}
            onClick={() => setMode("archive")}
          >
            归档 {archivedVideos.length}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={mode === "trash"}
            className={cn(
              "rounded-full px-3 py-1.5",
              mode === "trash" ? "bg-[var(--panel-2)] text-[var(--text)]" : "text-[var(--muted)]",
            )}
            onClick={() => setMode("trash")}
          >
            回收站 {trashVideos.length}
          </button>
        </div>
        <div className="mt-2 flex items-center gap-2 text-[11px] text-[var(--muted)]">
          <span>
            {videos.length} 项{totalSize > 0 ? ` · ${formatFileSize(totalSize)}` : ""}
            {mode === "trash" ? ` · 保留 ${retentionDays} 天` : " · 长期保留"}
          </span>
          <button
            className="ml-auto rounded-full border border-[var(--border)] bg-[var(--panel-2)] px-3 py-1.5 text-xs text-[var(--danger)] disabled:opacity-40"
            disabled={videos.length === 0 || busy}
            onClick={() => (mode === "trash" ? setEmptyOpen(true) : setArchiveOpen(true))}
            type="button"
          >
            {mode === "trash" ? "清空回收站" : "清空归档"}
          </button>
        </div>
      </div>

      {videos.length === 0 ? (
        <EmptyState
          hint={
            mode === "archive"
              ? "停用网盘来源后，它的收藏会长期保留在这里，不参与普通回收站的自动清理。"
              : `从收藏中移除的影片会先来到这里，保留 ${retentionDays} 天后自动彻底清理；期间可以随时恢复。`
          }
          title={mode === "archive" ? "归档是空的" : "回收站是空的"}
        />
      ) : (
        <div className="flex flex-col">
          {videos.map((video) => (
            <TrashRow
              archived={mode === "archive"}
              busy={busy}
              key={video.id}
              retentionDays={retentionDays}
              video={video}
              onPurge={() => setTarget(video)}
              onRestore={() => void restore(video)}
            />
          ))}
        </div>
      )}

      <p className="px-4 py-4 text-[11px] leading-relaxed text-[var(--muted)]">
        这里不复制原视频；彻底删除或超期清理只删除 Muse 的收藏记录与生成的缩略图/封面，不会删除源视频或网盘文件。
        归档长期保留，直到你清空归档或单条彻底删除。
      </p>

      <ConfirmSheet
        busy={busy}
        confirmLabel="彻底删除"
        danger
        message="只删除 Muse 的收藏记录与生成的缩略图/封面；原始视频文件不会被删除。此操作无法撤销。"
        onClose={() => setTarget(null)}
        onConfirm={() => {
          const video = target;
          setTarget(null);
          if (video) void purge(video);
        }}
        open={target !== null}
        title={`彻底删除「${target ? videoName(target) : ""}」？`}
      />

      <ConfirmSheet
        busy={busy}
        confirmLabel="清空回收站"
        danger
        message={`将彻底删除回收站里的 ${trashVideos.length} 项收藏。只删除 Muse 的记录与生成的图片，原始视频文件不会被删除。`}
        onClose={() => setEmptyOpen(false)}
        onConfirm={() => {
          setEmptyOpen(false);
          void empty();
        }}
        open={emptyOpen}
        title="清空回收站？"
      />

      <ConfirmSheet
        busy={busy}
        confirmLabel="清空归档"
        danger
        message={`将彻底删除归档里的 ${archivedVideos.length} 项收藏。只删除 Muse 的记录与生成的图片，网盘源文件不会被删除。`}
        onClose={() => setArchiveOpen(false)}
        onConfirm={() => {
          setArchiveOpen(false);
          void clearArchive();
        }}
        open={archiveOpen}
        title="清空归档？"
      />
    </div>
  );
}

function TrashRow({
  video,
  retentionDays,
  busy,
  archived = false,
  onRestore,
  onPurge,
}: {
  video: Video;
  retentionDays: number;
  busy: boolean;
  archived?: boolean;
  onRestore: () => void;
  onPurge: () => void;
}) {
  const reason =
    video.deleteReason === "missing"
      ? "文件缺失自动移入"
      : video.deleteReason === "archived"
        ? "网盘归档"
        : "手动移入";
  const remaining = remainingDays(video.deletedAt, retentionDays);
  const restoreBlocked = video.sourceId > 0 && !video.sourceEnabled;
  return (
    <div className="flex gap-3 border-b border-[var(--border)] px-4 py-3">
      <div className="h-[60px] w-[104px] shrink-0 overflow-hidden rounded-[6px] bg-[#111114]">
        {video.thumbnailPath ? (
          <img alt="" className="h-full w-full object-contain" loading="lazy" src={coverURL(video)} />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-lg opacity-40">🎞</div>
        )}
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{videoName(video)}</div>
        <div className="mt-0.5 text-[11px] text-[var(--muted)]">
          {archived
            ? `${reason} · 长期保留${video.sourceName ? ` · ${video.sourceName}` : ""}`
            : `${reason} · ${remaining > 0 ? `${remaining} 天后清理` : "即将清理"}`}
        </div>
        {(video.trashGroupNames?.length ?? 0) > 0 && (
          <div className="mt-0.5 truncate text-[11px] text-[var(--muted)]">
            原分组：{video.trashGroupNames.join(" / ")}
          </div>
        )}
        <div className="mt-1.5 flex gap-2">
          <button
            className="rounded-full border border-[var(--border)] px-3 py-1 text-xs active:bg-[var(--panel-2)] disabled:opacity-40"
            disabled={busy || restoreBlocked}
            onClick={onRestore}
            type="button"
          >
            恢复
          </button>
          <button
            className="rounded-full border border-[rgba(229,83,75,0.45)] px-3 py-1 text-xs text-[var(--danger)] active:bg-[rgba(229,83,75,0.12)] disabled:opacity-40"
            disabled={busy}
            onClick={onPurge}
            type="button"
          >
            彻底删除
          </button>
        </div>
      </div>
    </div>
  );
}
