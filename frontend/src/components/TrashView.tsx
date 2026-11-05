import { useState } from "react";
import { Trash2, RotateCcw, SlidersHorizontal } from "lucide-react";
import type { TrashSnapshot, Video } from "../types";
import { useLiveQuery } from "../hooks/useLiveQuery";
import { invalidate } from "../services/live";
import {
  clearArchivedVideos,
  deleteVideoPermanently,
  emptyTrash,
  listArchived,
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
  const archive = useLiveQuery(listArchived, "archive", {
    onError: () => undefined,
  });
  const [mode, setMode] = useState<"trash" | "archive">("trash");
  const [busyId, setBusyId] = useState<number | null>(null);
  const [emptying, setEmptying] = useState(false);
  const [clearingArchive, setClearingArchive] = useState(false);
  const [purgeTarget, setPurgeTarget] = useState<Video | null>(null);
  const [emptyTarget, setEmptyTarget] = useState(false);
  const [clearArchiveTarget, setClearArchiveTarget] = useState(false);

  const snapshot: TrashSnapshot | null = trash.data;
  const trashVideos = snapshot?.videos ?? [];
  const archivedVideos = archive.data ?? [];
  const videos = mode === "trash" ? trashVideos : archivedVideos;
  const retentionDays = snapshot?.retentionDays ?? 14;
  const totalSize = videos.reduce((sum, v) => sum + (v.fileSize > 0 ? v.fileSize : 0), 0);
  const activeLoading = mode === "trash" ? trash.loading && !snapshot : archive.loading && archive.data === null;
  const activeError = mode === "trash" ? trash.error : archive.error;
  const activeReload = mode === "trash" ? trash.reload : archive.reload;
  const groupedArchive = groupBySource(archivedVideos);

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
        details: ["只删除 Muse 的记录与生成的图片，源视频未被删除"],
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
        details: count > 0 ? ["只删除 Muse 的记录与生成的图片，源视频未被删除"] : undefined,
      });
    } catch (e) {
      onNotify({ kind: "error", message: "清空回收站失败", details: [String(e)] });
    } finally {
      setEmptying(false);
    }
  };

  const clearArchive = async () => {
    setClearArchiveTarget(false);
    setClearingArchive(true);
    try {
      const count = await clearArchivedVideos();
      invalidate();
      onNotify({
        kind: "info",
        message: count > 0 ? `已清空归档（${count} 项）` : "归档已经是空的",
        details: count > 0 ? ["只删除 Muse 的记录与生成的图片，网盘源文件未被删除"] : undefined,
      });
    } catch (e) {
      onNotify({ kind: "error", message: "清空归档失败", details: [String(e)] });
    } finally {
      setClearingArchive(false);
    }
  };

  if (activeLoading) {
    return <div className="flex min-h-[320px] items-center justify-center text-[var(--muted)]">加载中…</div>;
  }
  if (activeError && videos.length === 0) {
    return (
      <div className="flex min-h-[320px] flex-col items-center justify-center gap-3 text-center text-[var(--muted)]">
        <div>回收站加载失败</div>
        <Button onClick={activeReload}>重试</Button>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-col">
      <div className="sticky top-0 z-[5] -mx-5 mb-3 border-b border-[var(--border)] bg-[var(--bg)] px-5">
        <div className="flex min-h-10 flex-wrap items-center gap-x-3 gap-y-1">
          <h2 className="m-0 text-[15px] font-semibold tracking-[0.1px]">归档与回收站</h2>
          <div className="flex items-center gap-1 text-xs" role="tablist" aria-label="归档与回收站">
            <button
              type="button"
              role="tab"
              aria-selected={mode === "archive"}
              className={cn(
                "rounded px-2 py-1 transition-colors",
                mode === "archive" ? "bg-[var(--panel-2)] text-[var(--text)]" : "text-[var(--muted)] hover:text-[var(--text)]",
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
                "rounded px-2 py-1 transition-colors",
                mode === "trash" ? "bg-[var(--panel-2)] text-[var(--text)]" : "text-[var(--muted)] hover:text-[var(--text)]",
              )}
              onClick={() => setMode("trash")}
            >
              回收站 {trashVideos.length}
            </button>
          </div>
          <span className="text-xs text-[var(--muted)]">
            {videos.length} 项
            {totalSize > 0 ? ` · ${formatFileSize(totalSize)}` : ""}
          </span>
          {mode === "trash" && (
            <button
              className="inline-flex items-center gap-1 text-xs text-[var(--muted)] underline-offset-2 hover:text-[var(--text)] hover:underline"
              onClick={() => onOpenSettings("trash")}
              type="button"
            >
              <SlidersHorizontal className="h-3 w-3" aria-hidden />
              保留 {retentionDays} 天
            </button>
          )}
          <div className="ml-auto flex items-center gap-2">
            {mode === "trash" ? (
              <Button
                variant="danger"
                size="sm"
                disabled={videos.length === 0 || emptying}
                onClick={() => setEmptyTarget(true)}
              >
                <Trash2 className="h-3.5 w-3.5" aria-hidden />
                {emptying ? "清空中…" : "清空回收站"}
              </Button>
            ) : (
              <Button
                variant="danger"
                size="sm"
                disabled={videos.length === 0 || clearingArchive}
                onClick={() => setClearArchiveTarget(true)}
              >
                <Trash2 className="h-3.5 w-3.5" aria-hidden />
                {clearingArchive ? "清理中…" : "清空归档"}
              </Button>
            )}
          </div>
        </div>
      </div>

      {videos.length === 0 ? (
        <div className="flex h-full min-h-[320px] flex-col items-center justify-center text-center text-[var(--muted)]">
          <div className="mb-1.5 text-xl text-[var(--text)]">
            {mode === "archive" ? "归档是空的" : "回收站是空的"}
          </div>
          <div className="max-w-[420px] leading-relaxed">
            {mode === "archive"
              ? "停用网盘来源后，它的收藏会长期保留在这里。归档不会自动清理，也不会出现在片库、分组和查重里。"
              : `从收藏中移除的影片会先来到这里，保留 ${retentionDays} 天后自动彻底清理。期间可以随时恢复，分组、点赞和刮削资料都还在。`}
          </div>
        </div>
      ) : mode === "archive" ? (
        <div className="flex flex-col gap-3">
          {groupedArchive.map((group) => (
            <section key={group.name}>
              <h3 className="mb-1.5 flex items-center gap-2 text-xs font-medium text-[var(--muted)]">
                <span>{group.name}</span>
                <span className="font-normal">· {group.videos.length} 项</span>
              </h3>
              <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
                {group.videos.map((video) => (
                  <TrashRow
                    key={video.id}
                    archived
                    busy={busyId === video.id}
                    retentionDays={0}
                    video={video}
                    onPurge={() => setPurgeTarget(video)}
                    onRestore={() => void restore(video)}
                  />
                ))}
              </div>
            </section>
          ))}
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
        这里只保存收藏记录与已生成的缩略图/封面，不复制视频内容。普通回收站条目按保留期自动清理；
        归档长期保留，直到你点「清空归档」或彻底删除。源视频与网盘文件都不会被删除。
      </p>

      {purgeTarget && (
        <ConfirmDialog
          title="彻底删除"
          message={`确定彻底删除“${videoDisplayName(purgeTarget)}”吗？\n\n这条收藏记录与生成的缩略图/封面都会被删除；原始视频或网盘文件不会被删除。此操作无法撤销。`}
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
          message={`确定彻底删除回收站里的 ${trashVideos.length} 项收藏吗？\n\n只删除 Muse 的收藏记录与生成的缩略图/封面；原始视频文件不会被删除。此操作无法撤销。`}
          confirmLabel="清空回收站"
          cancelLabel="取消"
          danger
          onConfirm={() => void empty()}
          onCancel={() => setEmptyTarget(false)}
        />
      )}

      {clearArchiveTarget && (
        <ConfirmDialog
          title="清空归档"
          message={`确定彻底删除归档里的 ${archivedVideos.length} 项收藏吗？\n\n只删除 Muse 的收藏记录与生成的缩略图/封面；网盘源文件不会被删除。此操作无法撤销。`}
          confirmLabel="清空归档"
          cancelLabel="取消"
          danger
          onConfirm={() => void clearArchive()}
          onCancel={() => setClearArchiveTarget(false)}
        />
      )}
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
  const groupNames = video.trashGroupNames ?? [];
  const deletedAt = video.deletedAt > 0 ? formatDateTime(video.deletedAt) : "未知时间";
  const remaining = remainingDays(video.deletedAt, retentionDays);
  const restoreBlocked = archived && !video.sourceEnabled;
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
                : archived
                  ? "bg-[rgba(124,92,255,0.16)] text-[#cdc2ff]"
                  : "bg-[var(--panel-2)] text-[var(--muted)]",
            )}
          >
            {reason}
          </span>
        </div>
        <div className="mt-1 text-xs text-[var(--muted)]">
          {formatFileSize(video.fileSize) ? `${formatFileSize(video.fileSize)} · ` : ""}
          {archived
            ? `归档于 ${deletedAt} · 长期保留${video.sourceName ? ` · ${video.sourceName}` : ""}`
            : `移入于 ${deletedAt} · ${remaining > 0 ? `${remaining} 天后自动清理` : "即将清理"}`}
        </div>
        {groupNames.length > 0 && (
          <div className="mt-1 truncate text-xs text-[var(--muted)]">
            原分组：{groupNames.join(" / ")}
          </div>
        )}
        <div className="mt-1 truncate text-[11px] text-[var(--muted)]" title={video.remotePath || video.path}>
          {video.remotePath || video.path}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button
          size="sm"
          disabled={busy || restoreBlocked}
          title={restoreBlocked ? "先在设置中启用该网盘，再恢复这条收藏" : undefined}
          onClick={onRestore}
        >
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

function groupBySource(videos: Video[]): { name: string; videos: Video[] }[] {
  const byName = new Map<string, Video[]>();
  for (const video of videos) {
    const name = video.sourceName || video.archiveSourceName || "已删除的网盘";
    const list = byName.get(name) ?? [];
    list.push(video);
    byName.set(name, list);
  }
  return [...byName.entries()].map(([name, list]) => ({ name, videos: list }));
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
