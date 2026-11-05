import { useState } from "react";

import type { TrashSnapshot, Video } from "../../types";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import { invalidate } from "../../services/live";
import {
  ApiError,
  coverURL,
  deleteVideoPermanently,
  emptyTrash,
  listTrash,
  restoreVideo,
} from "../api";
import { videoName } from "../videoDisplay";
import { formatFileSize } from "../../utils/format";
import { ConfirmSheet } from "./Sheet";
import { EmptyState, ErrorState, Loading } from "./States";

const DAY_MS = 24 * 60 * 60 * 1000;

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
  const { data, loading, error, reload } = useLiveQuery(listTrash, "trash");
  const [target, setTarget] = useState<Video | null>(null);
  const [emptyOpen, setEmptyOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  const snapshot: TrashSnapshot | null = data;
  const videos = snapshot?.videos ?? [];
  const retentionDays = snapshot?.retentionDays ?? 14;
  const totalSize = videos.reduce((sum, v) => sum + Math.max(0, v.fileSize), 0);

  if (loading && !snapshot) return <Loading label="加载回收站…" />;
  if (error && !snapshot) return <ErrorState message={error} onRetry={reload} />;

  const restore = async (video: Video) => {
    setBusy(true);
    try {
      await restoreVideo(video.id);
      invalidate();
      onToast(`已恢复「${videoName(video)}」`);
    } catch (e) {
      onToast(e instanceof ApiError ? e.message : "恢复失败", "error");
    } finally {
      setBusy(false);
    }
  };

  const purge = async () => {
    if (!target) return;
    const video = target;
    setTarget(null);
    setBusy(true);
    try {
      await deleteVideoPermanently(video.id);
      invalidate();
      onToast(`已彻底删除「${videoName(video)}」`);
    } catch (e) {
      onToast(e instanceof ApiError ? e.message : "彻底删除失败", "error");
    } finally {
      setBusy(false);
    }
  };

  const empty = async () => {
    setEmptyOpen(false);
    setBusy(true);
    try {
      const count = await emptyTrash();
      invalidate();
      onToast(count > 0 ? `已清空回收站（${count} 项）` : "回收站已经是空的");
    } catch (e) {
      onToast(e instanceof ApiError ? e.message : "清空回收站失败", "error");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      <div className="flex items-center gap-2 border-b border-[var(--border)] px-4 py-2.5 text-xs text-[var(--muted)]">
        <span>
          {videos.length} 项{totalSize > 0 ? ` · ${formatFileSize(totalSize)}` : ""} · 保留 {retentionDays} 天
        </span>
        <button
          className="ml-auto rounded-full border border-[var(--border)] bg-[var(--panel-2)] px-3 py-1.5 text-xs text-[var(--danger)] disabled:opacity-40"
          disabled={videos.length === 0 || busy}
          onClick={() => setEmptyOpen(true)}
          type="button"
        >
          清空回收站
        </button>
      </div>

      {videos.length === 0 ? (
        <EmptyState
          hint={`从收藏中移除的影片会先来到这里，保留 ${retentionDays} 天后自动彻底清理；期间可以随时恢复。`}
          title="回收站是空的"
        />
      ) : (
        <div className="flex flex-col">
          {videos.map((video) => (
            <TrashRow
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
        回收站不复制原视频；彻底删除或超期清理只删除 Muse 的收藏记录、生成的缩略图/封面与网盘缓存，不会删除源视频。
      </p>

      <ConfirmSheet
        busy={busy}
        confirmLabel="彻底删除"
        danger
        message="只删除 Muse 的收藏记录、生成的缩略图/封面与网盘缓存；原始视频文件不会被删除。此操作无法撤销。"
        onClose={() => setTarget(null)}
        onConfirm={() => void purge()}
        open={target !== null}
        title={`彻底删除「${target ? videoName(target) : ""}」？`}
      />

      <ConfirmSheet
        busy={busy}
        confirmLabel="清空回收站"
        danger
        message={`将彻底删除回收站里的 ${videos.length} 项收藏。只删除 Muse 的记录与生成的图片/缓存，原始视频文件不会被删除。`}
        onClose={() => setEmptyOpen(false)}
        onConfirm={() => void empty()}
        open={emptyOpen}
        title="清空回收站？"
      />
    </div>
  );
}

function TrashRow({
  video,
  retentionDays,
  busy,
  onRestore,
  onPurge,
}: {
  video: Video;
  retentionDays: number;
  busy: boolean;
  onRestore: () => void;
  onPurge: () => void;
}) {
  const reason = video.deleteReason === "missing" ? "文件缺失自动移入" : "手动移入";
  const remaining = remainingDays(video.deletedAt, retentionDays);
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
          {reason} · {remaining > 0 ? `${remaining} 天后清理` : "即将清理"}
        </div>
        {(video.trashGroupNames?.length ?? 0) > 0 && (
          <div className="mt-0.5 truncate text-[11px] text-[var(--muted)]">
            原分组：{video.trashGroupNames.join(" / ")}
          </div>
        )}
        <div className="mt-1.5 flex gap-2">
          <button
            className="rounded-full border border-[var(--border)] px-3 py-1 text-xs active:bg-[var(--panel-2)] disabled:opacity-40"
            disabled={busy}
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

function remainingDays(deletedAt: number, retentionDays: number): number {
  if (deletedAt <= 0) return retentionDays;
  return Math.max(0, Math.ceil((deletedAt + retentionDays * DAY_MS - Date.now()) / DAY_MS));
}
