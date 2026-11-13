import { useMemo, useState } from "react";

import type { JavTaskOverview, SortDir } from "../../types";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import { client } from "../../services/client";

// Stable identity for the empty board, so the memos below do not recompute.
const NO_TASKS: JavTaskOverview[] = [];

export type ScrapeTaskCategory = "queued" | "succeeded" | "failed";

/** 看板分组的名字。短，因为要挤在标签按钮里。 */
export const SCRAPE_CATEGORY_LABELS: Record<ScrapeTaskCategory, string> = {
  queued: "进行中",
  succeeded: "已完成",
  failed: "失败",
};

/** 单条任务的真实状态。比分组细：同一"进行中"里还有排队、刮削、等重试之分。 */
const DETAIL_LABELS: Record<string, string> = {
  queued: "排队中",
  running: "刮削中",
  retryable: "等待重试",
  succeeded: "已成功",
  no_code: "未识别到番号",
  not_found: "未找到作品",
  blocked: "请求被阻止",
  disabled: "已跳过",
};

/** 任务状态的展示名；未知状态退回原始值，方便发现后端新增了状态。 */
export function scrapeStatusLabel(status: string): string {
  return DETAIL_LABELS[status] ?? status;
}

function searchText(task: JavTaskOverview): string {
  return [task.video.fileName, task.video.javCode, task.video.title]
    .join(" ")
    .toLocaleLowerCase();
}

/**
 * 刮削任务看板。只读：两端都只看得见状态，发起刮削仍留在桌面端（Cookie 与数据源
 * 配置在那里）。
 *
 * 分组由壳决定（桌面把它放在工具栏、移动端放在底部标签里），筛选与排序在这里，
 * 因为两个平台对"当前分组 + 关键字 + 创建时间"的理解必须相同。
 */
export function useScrapeTasks({
  status,
  keyword,
}: {
  status: ScrapeTaskCategory;
  keyword: string;
}) {
  // A scrape writes one task per video; the live query follows the backend's
  // jav:scrape:updated events, so this board stays current by itself.
  const { data, loading, error, reload } = useLiveQuery(
    client.listJavTaskOverviews,
    "scrape-tasks",
  );
  const tasks = data ?? NO_TASKS;
  const [sortDir, setSortDir] = useState<SortDir>("desc");

  const counts = useMemo(() => {
    const acc: Record<ScrapeTaskCategory, number> = { queued: 0, succeeded: 0, failed: 0 };
    for (const task of tasks) {
      // 不认识的分组归到"失败"，宁可让用户看见也不要静默藏起来。
      const category = (task.category as ScrapeTaskCategory) in acc
        ? (task.category as ScrapeTaskCategory)
        : "failed";
      acc[category] += 1;
    }
    return acc;
  }, [tasks]);

  const visible = useMemo(() => {
    const needle = keyword.trim().toLocaleLowerCase();
    return tasks
      .filter((task) => task.category === status)
      .filter((task) => !needle || searchText(task).includes(needle))
      .sort((a, b) => (a.createdAt - b.createdAt || a.id - b.id) * (sortDir === "asc" ? 1 : -1));
  }, [keyword, sortDir, status, tasks]);

  return {
    tasks,
    counts,
    visible,
    sortDir,
    toggleSort: () => setSortDir((dir) => (dir === "asc" ? "desc" : "asc")),
    loading,
    error,
    reload,
  };
}
