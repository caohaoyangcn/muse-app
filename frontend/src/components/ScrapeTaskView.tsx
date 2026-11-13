import type { JavTaskOverview, Video } from "../types";
import { formatDateTime } from "../utils/task";
import { videoCoverURL } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import RestoredBadge from "./RestoredBadge";
import { Button } from "../ui/button";
import { cn } from "../lib/utils";
import {
  SCRAPE_CATEGORY_LABELS,
  scrapeStatusLabel,
  useScrapeTasks,
  type ScrapeTaskCategory,
} from "../views/scrape/useScrapeTasks";

export type ScrapeTaskStatus = ScrapeTaskCategory;

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
          {scrapeStatusLabel(task.status)}
        </span>
        <span>创建于 {formatDateTime(task.createdAt)}</span>
        <span>尝试 {task.attempts}/{task.maxAttempts}</span>
      </div>
    </button>
  );
}

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
  const { counts, visible, sortDir, toggleSort, loading, error, reload } = useScrapeTasks({
    status,
    keyword,
  });

  return (
    <section className="mx-auto flex min-h-full w-full max-w-[1080px] flex-col">
      <div className="sticky top-0 z-[5] flex flex-wrap items-center gap-2 bg-[var(--bg)] py-3.5">
        <h2 className="mr-2 text-base font-semibold">刮削任务</h2>
        {(Object.keys(SCRAPE_CATEGORY_LABELS) as ScrapeTaskStatus[]).map((value) => (
          <Button
            key={value}
            size="sm"
            variant={status === value ? "default" : "ghost"}
            className={cn(status === value && "border-[var(--accent)] bg-[rgba(124,92,255,0.14)]")}
            onClick={() => onStatusChange(value)}
          >
            {SCRAPE_CATEGORY_LABELS[value]} <span className="tabular-nums">{counts[value]}</span>
          </Button>
        ))}
        <Button
          size="sm"
          variant="ghost"
          className="ml-auto"
          onClick={toggleSort}
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
            <div className="mb-1.5 text-xl text-[var(--text)]">{keyword.trim() ? "无匹配任务" : `暂无${SCRAPE_CATEGORY_LABELS[status]}任务`}</div>
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
