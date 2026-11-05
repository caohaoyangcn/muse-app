import { useMemo, useState } from "react";
import { listJavTaskOverviews } from "../services/api";
import { useLiveQuery } from "../hooks/useLiveQuery";
import type { JavTaskOverview, SortDir, Video } from "../types";
import { formatDateTime } from "../utils/task";
import { videoCoverURL } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import RestoredBadge from "./RestoredBadge";
import { Button } from "./ui/button";
import { cn } from "../lib/utils";

export type ScrapeTaskStatus = "queued" | "succeeded" | "failed";

const STATUS_LABELS: Record<ScrapeTaskStatus, string> = {
  queued: "排队中",
  succeeded: "已成功",
  failed: "已失败",
};

const DETAIL_STATUS_LABELS: Record<string, string> = {
  queued: "排队中",
  running: "刮削中",
  retryable: "等待重试",
  succeeded: "已成功",
  no_code: "未识别到番号",
  not_found: "未找到作品",
  blocked: "请求被阻止",
  disabled: "已跳过",
};

function statusLabel(status: string): string {
  return DETAIL_STATUS_LABELS[status] ?? status;
}

function taskSearchText(task: JavTaskOverview): string {
	return [task.video.fileName, task.video.javCode, task.video.title]
		.join(" ")
		.toLocaleLowerCase();
}

function TaskRow({ task, onOpen }: { task: JavTaskOverview; onOpen: (video: Video) => void }) {
  const { video } = task;
  const error = task.lastError.trim();
  return (
    <button
      type="button"
      className="group flex w-full items-center gap-3 border border-[var(--border)] bg-[var(--card)] px-3 py-2 text-left transition hover:border-[var(--accent)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
      onClick={() => onOpen(video)}
      title={video.path}
    >
      <div className="relative aspect-video w-28 shrink-0 overflow-hidden rounded-md bg-[#111114]">
        {video.thumbnailPath || (video.coverSource === "scraped" && video.scrapedCoverPath) ? (
          <img className="block h-full w-full object-contain" src={videoCoverURL(video)} alt="" draggable={false} />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-xl opacity-50">▣</div>
        )}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-1.5">
          <div className="truncate font-medium">{videoDisplayName(video)}</div>
          {video.mosaicRestored && <RestoredBadge />}
        </div>
        {error && <div className="mt-1 truncate text-xs text-[var(--danger)]">{error}</div>}
      </div>
      <div className="flex shrink-0 flex-col items-end gap-1 text-xs text-[var(--muted)]">
        <span
          className={cn(
            "border px-2 py-1",
            task.category === "queued" && "border-[var(--accent)]/40 text-[var(--accent)]",
            task.category === "succeeded" && "border-[#3fb26f]/40 text-[#3fb26f]",
            task.category === "failed" && "border-[var(--danger)]/40 text-[var(--danger)]",
          )}
        >
          {statusLabel(task.status)}
        </span>
        <span>创建于 {formatDateTime(task.createdAt)}</span>
        <span>尝试 {task.attempts}/{task.maxAttempts}</span>
      </div>
    </button>
  );
}

// Stable identity for the empty board, so the memos below do not recompute.
const NO_TASKS: JavTaskOverview[] = [];

export default function ScrapeTaskView({
  status,
  keyword,
  onOpen,
  onStatusChange,
}: {
  status: ScrapeTaskStatus;
  keyword: string;
  onOpen: (video: Video) => void;
  onStatusChange: (status: ScrapeTaskStatus) => void;
}) {
  // A scrape writes one task per video; the live query follows the backend's
  // jav:scrape:updated events, so this board stays current by itself.
  const { data, loading, error, reload } = useLiveQuery(listJavTaskOverviews, "scrape-tasks");
  const tasks = data ?? NO_TASKS;
  const [sortDir, setSortDir] = useState<SortDir>("desc");

  const counts = useMemo(() => ({
    queued: tasks.filter((task) => task.category === "queued").length,
    succeeded: tasks.filter((task) => task.category === "succeeded").length,
    failed: tasks.filter((task) => task.category === "failed").length,
  }), [tasks]);

  const visible = useMemo(() => {
    const needle = keyword.trim().toLocaleLowerCase();
    return tasks
      .filter((task) => task.category === status)
      .filter((task) => !needle || taskSearchText(task).includes(needle))
      .sort((a, b) => (a.createdAt - b.createdAt || a.id - b.id) * (sortDir === "asc" ? 1 : -1));
  }, [keyword, sortDir, status, tasks]);

  return (
    <section className="mx-auto flex min-h-full w-full max-w-[1080px] flex-col">
      <div className="sticky top-0 z-[5] flex flex-wrap items-center gap-2 bg-[var(--bg)] py-3.5">
        <h2 className="mr-2 text-base font-semibold">刮削任务</h2>
        {(Object.keys(STATUS_LABELS) as ScrapeTaskStatus[]).map((value) => (
          <Button
            key={value}
            size="sm"
            variant={status === value ? "default" : "ghost"}
            className={cn(status === value && "border-[var(--accent)] bg-[rgba(124,92,255,0.14)]")}
            onClick={() => onStatusChange(value)}
          >
            {STATUS_LABELS[value]} <span className="tabular-nums">{counts[value]}</span>
          </Button>
        ))}
        <Button
          size="sm"
          variant="ghost"
          className="ml-auto"
          onClick={() => setSortDir((dir) => dir === "asc" ? "desc" : "asc")}
          title={sortDir === "desc" ? "按创建时间升序" : "按创建时间降序"}
        >
          创建时间 {sortDir === "desc" ? "↓" : "↑"}
        </Button>
      </div>

      {error ? (
        <div className="flex min-h-[320px] flex-col items-center justify-center gap-3 text-center text-[var(--danger)]">
          <div>加载刮削任务失败</div>
          <div className="max-w-[680px] break-words text-xs">{error}</div>
          <Button size="sm" onClick={reload}>重试</Button>
        </div>
      ) : loading ? (
        <div className="flex min-h-[320px] items-center justify-center text-[var(--muted)]">加载中…</div>
      ) : visible.length === 0 ? (
        <div className="flex min-h-[320px] items-center justify-center text-center text-[var(--muted)]">
          <div>
            <div className="mb-1.5 text-xl text-[var(--text)]">{keyword.trim() ? "无匹配任务" : `暂无${STATUS_LABELS[status]}任务`}</div>
            <div>{keyword.trim() ? `没有匹配“${keyword.trim()}”的任务` : "任务状态变化后会自动显示在这里"}</div>
          </div>
        </div>
      ) : (
        <div className="flex flex-col gap-1.5 pb-8">
          {visible.map((task) => <TaskRow key={task.id} task={task} onOpen={onOpen} />)}
        </div>
      )}
    </section>
  );
}
