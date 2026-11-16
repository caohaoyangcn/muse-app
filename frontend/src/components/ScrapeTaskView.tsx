import type { JavTaskOverview, Video } from "../types";
import { formatDateTime } from "../utils/task";
import { videoCoverURL } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import {
  Badge,
  EmptyState,
  List,
  ListItem,
  Spinner,
  ToggleButton,
  ToggleButtonGroup,
  Toolbar,
  toggleChipClass,
} from "../ui/astryx";
import { Button } from "../ui/button";
import { Icon } from "../ui/icon";
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
  const statusVariant =
    task.category === "failed" ? "error" : task.category === "succeeded" ? "success" : "info";
  return (
    <ListItem
      className="border border-[var(--border)] bg-[var(--card)]"
      description={
        <>
          <span>创建于 {formatDateTime(task.createdAt)} · 尝试 {task.attempts}/{task.maxAttempts}</span>
          {error ? <span className="mt-0.5 block truncate text-[var(--danger)]">{error}</span> : null}
        </>
      }
      endContent={
        <Badge
          label={scrapeStatusLabel(task.status)}
          variant={statusVariant}
        />
      }
      label={videoDisplayName(video)}
      onClick={() => onOpen(video)}
      startContent={
        <span className="block aspect-video w-28 shrink-0 overflow-hidden rounded-md bg-[#111114]">
          {video.thumbnailPath || (video.coverSource === "scraped" && video.scrapedCoverPath) ? (
            <img alt="" className="block h-full w-full object-contain" draggable={false} src={videoCoverURL(video)} />
          ) : (
            <span className="flex h-full w-full items-center justify-center text-[var(--muted)]">
              <Icon className="size-5" name="grid" />
            </span>
          )}
        </span>
      }
    />
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
      <Toolbar
        className="sticky top-0 z-[5] rounded-none border-b border-[var(--border)] bg-[var(--bg)] px-0 py-3"
        endContent={
          <Button
            className="ml-auto"
            onClick={toggleSort}
            size="sm"
            title={sortDir === "desc" ? "按创建时间升序" : "按创建时间降序"}
            variant="ghost"
          >
            <Icon className="size-3.5" name={sortDir === "desc" ? "arrow-down" : "arrow-up"} />
            创建时间
          </Button>
        }
        label="刮削任务"
        startContent={
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="mr-2 shrink-0 text-base font-semibold">刮削任务</h2>
            <ToggleButtonGroup
          className="flex flex-wrap items-center gap-1.5"
          label="刮削状态"
          onChange={(value) => value && onStatusChange(value as ScrapeTaskStatus)}
          size="sm"
          type="single"
          value={status}
        >
          {(Object.keys(SCRAPE_CATEGORY_LABELS) as ScrapeTaskStatus[]).map((value) => (
            <ToggleButton
              className={toggleChipClass}
              key={value}
              label={SCRAPE_CATEGORY_LABELS[value]}
              value={value}
            >
              {SCRAPE_CATEGORY_LABELS[value]}{" "}
              <span className="tabular-nums opacity-70">{counts[value]}</span>
            </ToggleButton>
          ))}
            </ToggleButtonGroup>
          </div>
        }
      />


      {error ? (
        <EmptyState
          actions={<Button onClick={reload} size="sm" variant="primary">重试</Button>}
          description={error}
          title="加载刮削任务失败"
        />
      ) : loading ? (
        <div className="flex min-h-[320px] items-center justify-center">
          <Spinner label="加载中…" size="lg" />
        </div>
      ) : visible.length === 0 ? (
        <EmptyState
          description={keyword.trim() ? `没有匹配“${keyword.trim()}”的任务` : "任务状态变化后会自动显示在这里"}
          title={keyword.trim() ? "无匹配任务" : `暂无${SCRAPE_CATEGORY_LABELS[status]}任务`}
        />
      ) : (
        <List className="flex flex-col gap-1.5 pb-8">
          {visible.map((task) => <TaskRow key={task.id} onOpen={onOpen} task={task} />)}
        </List>
      )}
    </section>
  );
}
