import { useState } from "react";

import type { TrashSnapshot, Video } from "../../types";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import { client } from "../../services/client";
import { errorMessage } from "../../services/errors";
import { invalidate } from "../../services/live";
import { videoDisplayName } from "../../utils/videoName";
import type { ViewNotify } from "../notify";

const DAY_MS = 24 * 60 * 60 * 1000;

/** 回收站与归档共用的取数、状态与操作。两个平台的壳只负责渲染。 */
export function useTrash(notify: ViewNotify) {
  const trash = useLiveQuery(client.listTrash, "trash", { onError: () => undefined });
  const archive = useLiveQuery(client.listArchived, "archive", { onError: () => undefined });

  const [mode, setMode] = useState<"trash" | "archive">("trash");
  const [busyId, setBusyId] = useState<number | null>(null);
  const [emptying, setEmptying] = useState(false);
  const [clearingArchive, setClearingArchive] = useState(false);

  const snapshot: TrashSnapshot | null = trash.data;
  const trashVideos = snapshot?.videos ?? [];
  const archivedVideos = archive.data ?? [];
  const videos = mode === "trash" ? trashVideos : archivedVideos;
  const retentionDays = snapshot?.retentionDays ?? 14;
  const totalSize = videos.reduce((sum, v) => sum + (v.fileSize > 0 ? v.fileSize : 0), 0);

  // Only the mode on screen decides what "loading" / "error" / "reload" mean:
  // a failed archive read must not blank a usable trash list.
  const active = mode === "trash" ? trash : archive;
  const loading = mode === "trash" ? trash.loading && !snapshot : archive.loading && archive.data === null;

  const restore = async (video: Video) => {
    setBusyId(video.id);
    try {
      await client.restoreVideo(video.id);
      invalidate();
      notify({ kind: "success", text: `已恢复“${videoDisplayName(video)}”` });
    } catch (e) {
      notify({ kind: "error", text: "恢复失败", details: [errorMessage(e, "未知错误")] });
    } finally {
      setBusyId(null);
    }
  };

  const purge = async (video: Video) => {
    setBusyId(video.id);
    try {
      await client.deleteVideoPermanently(video.id);
      invalidate();
      notify({
        kind: "info",
        text: `已彻底删除“${videoDisplayName(video)}”`,
        details: ["只删除 Muse 的记录与生成的图片，源视频未被删除"],
      });
    } catch (e) {
      notify({ kind: "error", text: "彻底删除失败", details: [errorMessage(e, "未知错误")] });
    } finally {
      setBusyId(null);
    }
  };

  const empty = async () => {
    setEmptying(true);
    try {
      const count = await client.emptyTrash();
      invalidate();
      notify({
        kind: "info",
        text: count > 0 ? `已清空回收站（${count} 项）` : "回收站已经是空的",
        details: count > 0 ? ["只删除 Muse 的记录与生成的图片，源视频未被删除"] : undefined,
      });
    } catch (e) {
      notify({ kind: "error", text: "清空回收站失败", details: [errorMessage(e, "未知错误")] });
    } finally {
      setEmptying(false);
    }
  };

  const clearArchive = async () => {
    setClearingArchive(true);
    try {
      const count = await client.clearArchivedVideos();
      invalidate();
      notify({
        kind: "info",
        text: count > 0 ? `已清空归档（${count} 项）` : "归档已经是空的",
        details: count > 0 ? ["只删除 Muse 的记录与生成的图片，网盘源文件未被删除"] : undefined,
      });
    } catch (e) {
      notify({ kind: "error", text: "清空归档失败", details: [errorMessage(e, "未知错误")] });
    } finally {
      setClearingArchive(false);
    }
  };

  return {
    mode,
    setMode,
    trashVideos,
    archivedVideos,
    videos,
    retentionDays,
    totalSize,
    loading,
    error: active.error,
    reload: active.reload,
    busyId,
    emptying,
    clearingArchive,
    restore,
    purge,
    empty,
    clearArchive,
  };
}

/** 归档按来源分组：同一网盘的归档条目排在一起，桌面端据此分节显示。 */
export function groupTrashBySource(videos: Video[]): { name: string; videos: Video[] }[] {
  const byName = new Map<string, Video[]>();
  for (const video of videos) {
    const name = video.sourceName || video.archiveSourceName || "已删除的网盘";
    const list = byName.get(name) ?? [];
    list.push(video);
    byName.set(name, list);
  }
  return [...byName.entries()].map(([name, list]) => ({ name, videos: list }));
}

/**
 * 距离自动彻底清理还有几天。`deletedAt <= 0` 的旧记录没有时间戳，按整段保留期算。
 */
export function remainingDays(deletedAt: number, retentionDays: number): number {
  if (deletedAt <= 0) return retentionDays;
  return Math.max(0, Math.ceil((deletedAt + retentionDays * DAY_MS - Date.now()) / DAY_MS));
}
