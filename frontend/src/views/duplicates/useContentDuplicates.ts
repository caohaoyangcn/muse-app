import { useEffect, useState } from "react";

import type { ContentDuplicateGroup, ContentScanStatus, Video } from "../../types";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import { client } from "../../services/client";
import { errorMessage } from "../../services/errors";
import { invalidate } from "../../services/live";
import type { ViewNotify } from "../notify";

// Stable identity for the empty worklist.
const NO_GROUPS: ContentDuplicateGroup[] = [];

/** 完整校验的逐字节进度，按组（内容指纹）归属。 */
export type ConfirmProgress = { hash: string; done: number; total: number };

export type ContentDuplicatesOptions = {
  notify: ViewNotify;
  /**
   * 桌面端：订阅完整校验的进度事件（Wails）。浏览器没有事件通道，不传即可 ——
   * 那一边靠 `group.confirming` 与定时重读表示"还在校验"。
   */
  subscribeConfirmProgress?: (onProgress: (progress: ConfirmProgress | null) => void) => () => void;
  /** 有拷贝被移入回收站后通知壳：桌面据此关掉详情页并清出选中集。 */
  onResolved?: (removedIds: number[]) => void;
};

/**
 * 内容重复的工作清单：同一份字节被收藏了两次。
 *
 * 与番号查重是两回事：那边是同一部作品的不同压制，这边是完全相同的文件；一组可能
 * 同时出现在两个清单里。两个结局与番号查重一致：保留一份（其余记录移入回收站，文件
 * 一律不动）或保留全部（记下决定，直到成员变化）。
 *
 * 扫描进度两平台采用同一套轮询：桌面原本用 Wails 事件推，但事件只在桌面存在，轮询
 * 让"扫描条"在两个壳上行为一致，代价是 1.5 秒的刷新延迟。
 */
export function useContentDuplicates({
  notify,
  subscribeConfirmProgress,
  onResolved,
}: ContentDuplicatesOptions) {
  const [includeIgnored, setIncludeIgnored] = useState(false);
  const [scan, setScan] = useState<ContentScanStatus | null>(null);
  const [confirmProgress, setConfirmProgress] = useState<ConfirmProgress | null>(null);
  const [busy, setBusy] = useState(false);

  const { data, loading, error, reload } = useLiveQuery(
    async () => (await client.listContentDuplicates(includeIgnored)) ?? NO_GROUPS,
    `content-duplicates:${includeIgnored}`,
  );
  const groups = data ?? NO_GROUPS;

  // 扫描状态：读一次，运行中按 1.5 秒跟进；结束时刷新一次全量数据 —— 新指纹会
  // 改写哪些收藏能互相匹配，不只影响这个面板。
  useEffect(() => {
    void client.contentScanStatus().then(setScan).catch(() => undefined);
  }, []);

  useEffect(() => {
    if (!scan?.running) return;
    let stopped = false;
    const timer = window.setInterval(() => {
      void client
        .contentScanStatus()
        .then((status) => {
          if (stopped) return;
          setScan(status);
          if (!status.running) invalidate();
        })
        .catch(() => undefined);
    }, 1500);
    return () => {
      stopped = true;
      window.clearInterval(timer);
    };
  }, [scan?.running]);

  // 桌面端把逐字节进度推过来；结束时清空并刷新。
  useEffect(() => {
    if (!subscribeConfirmProgress) return;
    return subscribeConfirmProgress((progress) => {
      setConfirmProgress(progress);
      if (progress === null) invalidate();
    });
  }, [subscribeConfirmProgress]);

  // 没有事件的一端（浏览器）靠这个跟上"还在校验"的组：结果落库后成员上的
  // confirming 会消失，轮询随之停止。
  const anyConfirming = groups.some((g) => g.confirming);
  useEffect(() => {
    if (!anyConfirming) return;
    const timer = window.setInterval(() => reload(), 3000);
    return () => window.clearInterval(timer);
  }, [anyConfirming, reload]);

  const startScan = async () => {
    try {
      setScan(await client.scanContentDuplicates());
      notify({ kind: "success", text: "已开始扫描内容指纹，可继续使用其它功能" });
    } catch (e) {
      notify({ kind: "error", text: `无法开始扫描：${errorMessage(e, "未知错误")}` });
    }
  };

  /** 保留一份拷贝：其余记录移入回收站，底层文件不动。 */
  const keepCopy = async (group: ContentDuplicateGroup, keep: Video) => {
    const removed = group.members.map((m) => m.video).filter((v) => v.id !== keep.id);
    setBusy(true);
    try {
      for (const v of removed) await client.removeVideo(v.id);
      notify({
        kind: "success",
        text:
          removed.length === 1
            ? `已保留「${keep.fileName}」，移入回收站 1 个重复拷贝`
            : `已保留「${keep.fileName}」，移入回收站 ${removed.length} 个重复拷贝`,
      });
      invalidate();
      onResolved?.(removed.map((v) => v.id));
    } catch (e) {
      notify({ kind: "error", text: `移入回收站失败：${errorMessage(e, "未知错误")}` });
      reload();
    } finally {
      setBusy(false);
    }
  };

  /** 保留全部拷贝，并记下决定。 */
  const keepAll = async (group: ContentDuplicateGroup) => {
    try {
      await client.ignoreContentDuplicate(group.hash);
      notify({
        kind: "success",
        text: group.members.length > 2 ? "已保留全部拷贝" : "已保留两份拷贝",
      });
      reload();
    } catch (e) {
      notify({ kind: "error", text: `忽略重复提示失败：${errorMessage(e, "未知错误")}` });
    }
  };

  /** 重新读取指纹：成员在应用之外被改动后，用它让结论重新可信。 */
  const reverify = async (group: ContentDuplicateGroup) => {
    try {
      await client.reverifyContentDuplicate(group.hash);
      notify({ kind: "success", text: "已重新读取内容指纹，请稍候刷新" });
      // 后台正在重算，给一点时间再重建列表，否则看到的还是旧结论。
      window.setTimeout(reload, 800);
    } catch (e) {
      notify({ kind: "error", text: `重新校验失败：${errorMessage(e, "未知错误")}` });
    }
  };

  /** 完整校验：逐字节读取整份文件（5 GB 在 USB 机械盘上约 40 秒）。 */
  const confirmGroup = async (group: ContentDuplicateGroup) => {
    try {
      await client.confirmContentDuplicate(group.hash);
      setConfirmProgress({ hash: group.hash, done: 0, total: group.members.length });
      notify({ kind: "success", text: "已开始完整校验（逐字节读取，大文件较慢）" });
      reload();
    } catch (e) {
      notify({ kind: "error", text: `完整校验失败：${errorMessage(e, "未知错误")}` });
    }
  };

  return {
    groups,
    activeCount: groups.filter((g) => !g.ignored).length,
    loading,
    error,
    reload,
    includeIgnored,
    setIncludeIgnored,
    scan,
    confirmProgress,
    busy,
    startScan,
    keepCopy,
    keepAll,
    reverify,
    confirmGroup,
  };
}

/** 按文件名或指纹筛选，纯展示逻辑，留给各壳自己决定用不用。 */
export function filterContentGroups(
  groups: ContentDuplicateGroup[],
  keyword: string,
): ContentDuplicateGroup[] {
  const needle = keyword.trim().toLowerCase();
  if (!needle) return groups;
  return groups.filter(
    (group) =>
      group.hash.toLowerCase().includes(needle) ||
      group.members.some((m) => m.video.fileName.toLowerCase().includes(needle)),
  );
}

/**
 * 扫描在做什么，带当前阶段的计数。两个阶段数的东西不同（先读大小，再比对内容），
 * 所以某个阶段的总数在它开始之前并不存在。
 */
export function scanStatusText(status: ContentScanStatus | null): string {
  if (!status) return "尚未扫描过内容指纹";
  if (status.running) {
    if (status.phase === "examining") {
      return `正在检查文件大小… ${status.examined}/${status.examineTotal}`;
    }
    if (status.phase === "comparing") {
      return `正在比对内容指纹… ${status.done}/${status.total}`;
    }
    return "正在准备扫描…";
  }
  if (status.finishedAt > 0) {
    let text = `上次扫描：检查 ${status.examineTotal} 个文件`;
    text += status.total > 0 ? ` · 比对 ${status.total} 个` : " · 无需比对内容";
    if (status.failed > 0) text += ` · ${status.failed} 个无法读取`;
    return text;
  }
  return "尚未扫描过内容指纹";
}

/** 当前阶段的进度，进度条按它绘制。 */
export function scanProgress(
  status: ContentScanStatus | null,
): { done: number; total: number } | null {
  if (!status?.running) return null;
  if (status.phase === "examining") return { done: status.examined, total: status.examineTotal };
  if (status.phase === "comparing") return { done: status.done, total: status.total };
  return null;
}
