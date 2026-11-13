import { useCallback, useState } from "react";

import type { JavDuplicateGroup, Video } from "../../types";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import { client } from "../../services/client";
import { errorMessage } from "../../services/errors";
import { invalidate } from "../../services/live";
import type { ViewNotify } from "../notify";

// Stable identity for the empty worklist.
const NO_GROUPS: JavDuplicateGroup[] = [];

export type JavDuplicatesOptions = {
  notify: ViewNotify;
  /**
   * 桌面端：逐条向后端复核版本文件是否还在。删除重复通常发生在系统的文件管理器里，
   * 存的可用性标记可能是旧的。浏览器没有这个端点，移动端不传，列表照后端给的标记显示。
   */
  refreshVersion?: (id: number) => Promise<Video>;
  /** 有版本被移入回收站后通知壳：桌面据此关掉对应详情页并清出选中集。 */
  onResolved?: (removedIds: number[]) => void;
};

/**
 * 番号查重的工作清单：同一部作品被收藏了多个版本。
 *
 * 两个结局都由这里实现，因为两条平台的分支只在渲染上不同：保留一个版本（其余记录
 * 移入回收站，文件一律不动），或保留全部（记下决定，该作品不再被报出来）。
 */
export function useJavDuplicates({ notify, refreshVersion, onResolved }: JavDuplicatesOptions) {
  const [includeIgnored, setIncludeIgnored] = useState(false);
  const [busy, setBusy] = useState(false);

  const fetchGroups = useCallback(async () => {
    const list = (await client.listJavDuplicates(includeIgnored)) ?? NO_GROUPS;
    if (!refreshVersion) return list;
    const ids = [...new Set(list.flatMap((g) => g.videos.map((v) => v.id)))];
    const updates = new Map<number, Video>();
    await Promise.all(
      ids.map(async (id) => {
        try {
          updates.set(id, await refreshVersion(id));
        } catch {
          // A failed check leaves the stored flag in place.
        }
      }),
    );
    if (updates.size > 0) {
      for (const group of list) {
        group.videos = group.videos.map((v) => updates.get(v.id) ?? v);
      }
    }
    return list;
  }, [includeIgnored, refreshVersion]);

  const { data, loading, error, reload } = useLiveQuery(
    fetchGroups,
    `jav-duplicates:${includeIgnored}`,
  );
  const groups = data ?? NO_GROUPS;
  const activeCount = groups.filter((g) => !g.ignored).length;

  /** 保留一个版本：同一作品的其余记录移入回收站，底层文件不动。 */
  const keepVersion = async (group: JavDuplicateGroup, keep: Video) => {
    const removed = group.videos.filter((v) => v.id !== keep.id);
    setBusy(true);
    try {
      for (const v of removed) await client.removeVideo(v.id);
      notify({
        kind: "success",
        text:
          removed.length === 1
            ? `已保留「${keep.fileName}」，移入回收站 1 个重复版本`
            : `已保留「${keep.fileName}」，移入回收站 ${removed.length} 个重复版本`,
      });
      // 收藏集变了，其它已挂载的视图要跟着更新。
      invalidate();
      onResolved?.(removed.map((v) => v.id));
    } catch (e) {
      notify({ kind: "error", text: `移入回收站失败：${errorMessage(e, "未知错误")}` });
      reload();
    } finally {
      setBusy(false);
    }
  };

  /** 保留全部版本，并记下决定，让这部作品不再出现在清单里。 */
  const keepAll = async (group: JavDuplicateGroup) => {
    try {
      await client.ignoreJavDuplicate(group.key);
      notify({
        kind: "success",
        text: group.videos.length > 2 ? "已保留全部版本" : "已保留两者",
      });
      reload();
    } catch (e) {
      notify({ kind: "error", text: `忽略重复提示失败：${errorMessage(e, "未知错误")}` });
    }
  };

  return {
    groups,
    activeCount,
    loading,
    error,
    reload,
    includeIgnored,
    setIncludeIgnored,
    busy,
    keepVersion,
    keepAll,
  };
}

/** 按番号或文件名筛选，纯展示逻辑，留给各壳自己决定用不用。 */
export function filterJavGroups(groups: JavDuplicateGroup[], keyword: string): JavDuplicateGroup[] {
  const needle = keyword.trim().toLowerCase();
  if (!needle) return groups;
  return groups.filter(
    (group) =>
      group.code.toLowerCase().includes(needle) ||
      group.videos.some((v) => v.fileName.toLowerCase().includes(needle)),
  );
}
