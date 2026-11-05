import { useState } from "react";
import { Trash2, RotateCcw, SlidersHorizontal } from "lucide-react";
import type { TrashSnapshot, Video } from "../types";
import { useLiveQuery } from "../hooks/useLiveQuery";
import { invalidate } from "../services/live";
import {
  deleteVideoPermanently,
  emptyTrash,
  listTrash,
  restoreVideo,
} from "../services/api";
import { formatFileSize, videoCoverURL } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import ConfirmDialog from "./ConfirmDialog";
import type { ToastItem } from "./Toasts";
import { Button } from "./ui/button";
import { cn } from "../lib/utils";

const DAY_MS = 24 * 60 * 60 * 1000;

/**
 * 回收站: favorites removed from the library, kept restorable for the retention
 * period. This is not the filesystem recycle bin — nothing here ever holds or
 * deletes a source video. Rows keep their id, cover, group memberships and
 * scraped metadata, so 恢复 puts the exact favorite back.
 */
export default function TrashView({
  onNotify,
  onOpenSettings,
}: {
  onNotify: (toast: Omit<ToastItem, "id">, ttl?: number) => void;
  onOpenSettings: (section?: string) => void;
}) {
  const trash = useLiveQuery(listTrash, "trash", {
    onError: () => undefined,
  });
  const [busyId, setBusyId] = useState<number | null>(null);
  const [emptying, setEmptying] = useState(false);
  const [purgeTarget, setPurgeTarget] = useState<Video | null>(null);
  const [emptyTarget, setEmptyTarget] = useState(false);

  const snapshot: TrashSnapshot | null = trash.data;
  const videos = snapshot?.videos ?? [];
  const retentionDays = snapshot?.retentionDays ?? 14;
  const totalSize = videos.reduce((sum, v) => sum + (v.fileSize > 0 ? v.fileSize : 0), 0);

  const restore = async (video: Video) => {
    setBusyId(video.id);
    try {
      await restoreVideo(video.id);
      invalidate();
      onNotify({ kind: "success", message: `已恢复“${videoDisplayName(video)}”` });
    } catch (e) {
      onNotify({ kind: "error", message: "恢复失败", details: [String(e)] });
    } finally {
      setBusyId(null);
    }
  };

  const purge = async (video: Video) => {
    setPurgeTarget(null);
    setBusyId(video.id);
    try {
      await deleteVideoPermanently(video.id);
      invalidate();
      onNotify({
        kind: "info",
        message: `已彻底删除“${videoDisplayName(video)}”`,
        details: ["只删除 Muse 的记录与生成的图片/缓存，源视频未被删除"],
      });
    } catch (e) {
      onNotify({ kind: "error", message: "彻底删除失败", details: [String(e)] });
    } finally {
      setBusyId(null);
    }
  };

  const empty = async () => {
    setEmptyTarget(false);
    setEmptying(true);
    try {
      const count = await emptyTrash();
      invalidate();
      onNotify({
        kind: "info",
        message: count > 0 ? `已清空回收站（${count} 项）` : "回收站已经是空的",
        details: count > 0 ? ["只删除 Muse 的记录与生成的图片/缓存，源视频未被删除"] : undefined,
      });
    } catch (e) {
      onNotify({ kind: "error", message: "清空回收站失败", details: [String(e)] });
    } finally {
      setEmptying(false);
    }
  };

  if (trash.loading && !snapshot) {
    return <div className="flex min-h-[320px] items-center justify-center text-[var(--muted)]">加载中…</div>;
  }
  if (trash.error && !snapshot) {
    return (
      <div className="flex min-h-[320px] flex-col items-center justify-center gap-3 text-center text-[var(--muted)]">
        <div>回收站加载失败</div>
        <Button onClick={trash.reload}>重试</Button>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-col">
      <div className="sticky top-0 z-[5] -mx-5 mb-3 border-b border-[var(--border)] bg-[var(--bg)] px-5">
        <div className="flex min-h-10 flex-wrap items-center gap-x-3 gap-y-1">
          <h2 className="m-0 text-[15px] font-semibold tracking-[0.1px]">回收站</h2>
          <span className="text-xs text-[var(--muted)]">
            {videos.length} 项
            {totalSize > 0 ? ` · ${formatFileSize(totalSize)}` : ""}
          </span>
          <button
            className="inline-flex items-center gap-1 text-xs text-[var(--muted)] underline-offset-2 hover:text-[var(--text)] hover:underline"
            onClick={() => onOpenSettings("trash")}
            type="button"
          >
            <SlidersHorizontal className="h-3 w-3" aria-hidden />
            保留 {retentionDays} 天
          </button>
          <div className="ml-auto flex items-center gap-2">
            <Button
              variant="danger"
              size="sm"
              disabled={videos.length === 0 || emptying}
              onClick={() => setEmptyTarget(true)}
            >
              <Trash2 className="h-3.5 w-3.5" aria-hidden />
              {emptying ? "清空中…" : "清空回收站"}
            </Button>
          </div>
        </div>
      </div>

      {videos.length === 0 ? (
        <div className="flex h-full min-h-[320px] flex-col items-center justify-center text-center text-[var(--muted)]">
          <div className="mb-1.5 text-xl text-[var(--text)]">回收站是空的</div>
          <div className="max-w-[420px] leading-relaxed">
            从收藏中移除的影片会先来到这里，保留 {retentionDays} 天后自动彻底清理。
            期间可以随时恢复，分组、点赞和刮削资料都还在。
          </div>
        </div>
      ) : (
        <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
          {videos.map((video) => (
            <TrashRow
              key={video.id}
              busy={busyId === video.id}
              retentionDays={retentionDays}
              video={video}
              onPurge={() => setPurgeTarget(video)}
              onRestore={() => void restore(video)}
            />
          ))}
        </div>
      )}

      <p className="mt-3 text-xs leading-relaxed text-[var(--muted)]">
        回收站只保存收藏记录与已生成的缩略图/封面，不复制原视频。超过保留期的条目会在应用启动或后台定时检查时自动清理，
        源视频与网盘文件都不会被删除。
      </p>

      {purgeTarget && (
        <ConfirmDialog
          title="彻底删除"
          message={`确定彻底删除“${videoDisplayName(purgeTarget)}”吗？\n\n这条收藏记录、生成的缩略图/封面与网盘缓存都会被删除；原始视频文件不会被删除。此操作无法撤销。`}
          confirmLabel="彻底删除"
          cancelLabel="取消"
          danger
          onConfirm={() => void purge(purgeTarget)}
          onCancel={() => setPurgeTarget(null)}
        />
      )}

      {emptyTarget && (
        <ConfirmDialog
          title="清空回收站"
          message={`确定彻底删除回收站里的 ${videos.length} 项收藏吗？\n\n只删除 Muse 的收藏记录、生成的缩略图/封面与网盘缓存；原始视频文件不会被删除。此操作无法撤销。`}
          confirmLabel="清空回收站"
          cancelLabel="取消"
          danger
          onConfirm={() => void empty()}
          onCancel={() => setEmptyTarget(false)}
        />
      )}
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
  const groupNames = video.trashGroupNames ?? [];
  const deletedAt = video.deletedAt > 0 ? formatDateTime(video.deletedAt) : "未知时间";
  const remaining = remainingDays(video.deletedAt, retentionDays);
  return (
    <div className="flex items-center gap-4 border-b border-[var(--border)] bg-[var(--panel)] p-3 last:border-b-0">
      <div className="relative h-[64px] w-[112px] shrink-0 overflow-hidden rounded-[6px] bg-[#111114]">
        {video.thumbnailPath ? (
          <img
            alt=""
            className="block h-full w-full object-contain"
            draggable={false}
            src={videoCoverURL(video)}
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-xl opacity-40">🎞</div>
        )}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate font-medium">{videoDisplayName(video)}</span>
          <span
            className={cn(
              "shrink-0 rounded-full px-2 py-0.5 text-[11px]",
              video.deleteReason === "missing"
                ? "bg-[rgba(224,182,79,0.16)] text-[#e0b64f]"
                : "bg-[var(--panel-2)] text-[var(--muted)]",
            )}
          >
            {reason}
          </span>
        </div>
        <div className="mt-1 text-xs text-[var(--muted)]">
          {formatFileSize(video.fileSize) ? `${formatFileSize(video.fileSize)} · ` : ""}
          移入于 {deletedAt} · {remaining > 0 ? `${remaining} 天后自动清理` : "即将清理"}
        </div>
        {groupNames.length > 0 && (
          <div className="mt-1 truncate text-xs text-[var(--muted)]">
            原分组：{groupNames.join(" / ")}
          </div>
        )}
        <div className="mt-1 truncate text-[11px] text-[var(--muted)]" title={video.path}>
          {video.path}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button size="sm" disabled={busy} onClick={onRestore}>
          <RotateCcw className="h-3.5 w-3.5" aria-hidden />
          恢复
        </Button>
        <Button variant="danger" size="sm" disabled={busy} onClick={onPurge}>
          彻底删除
        </Button>
      </div>
    </div>
  );
}

function remainingDays(deletedAt: number, retentionDays: number): number {
  if (deletedAt <= 0) return retentionDays;
  const expiresAt = deletedAt + retentionDays * DAY_MS;
  return Math.max(0, Math.ceil((expiresAt - Date.now()) / DAY_MS));
}

function formatDateTime(ms: number): string {
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
