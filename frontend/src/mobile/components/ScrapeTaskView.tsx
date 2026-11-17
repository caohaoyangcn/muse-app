import { useEffect, useRef, useState } from "react";
import { Badge, EmptyState, List, ListItem, SegmentedControl, SegmentedControlItem, Spinner } from "../../ui/astryx";

import type { Video } from "../../types";
import { formatDateTime } from "../../utils/datetime";
import { usePagedList } from "../../hooks/usePagedList";
import { videoName } from "../videoDisplay";
import {
  SCRAPE_CATEGORY_LABELS,
  scrapeStatusLabel,
  useScrapeTasks,
  type ScrapeTaskCategory,
} from "../../views/scrape/useScrapeTasks";
import { Button } from "../../ui/button";
import { Pagination } from "../../ui/pagination";

/** One page of task rows; a finished category can hold a thousand. */
const PAGE_SIZE = 50;

/**
 * The scrape board. Read-only: the mobile client can inspect task status but
 * cannot trigger scraping (that stays on the desktop, where the Cookie and
 * provider configuration live).
 */
export function ScrapeTaskView({ onOpenVideo }: { onOpenVideo: (v: Video) => void }) {
  const [category, setCategory] = useState<ScrapeTaskCategory>("queued");
  // 这一屏没有关键字框，看板只按分组过滤。
  const { tasks, counts, visible: shown, loading, error, reload } = useScrapeTasks({
    status: category,
    keyword: "",
  });
  const listKey = `mobile:tasks:${category}`;
  const paging = usePagedList({ total: shown.length, listKey, pageSize: PAGE_SIZE });
  const pageTasks = shown.slice(paging.startIndex, paging.endIndex);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const firstListKey = useRef(true);
  useEffect(() => {
    if (firstListKey.current) {
      firstListKey.current = false;
      return;
    }
    scrollRef.current?.scrollTo({ top: 0 });
  }, [listKey]);
  const turnPage = (next: number) => {
    paging.goToPage(next);
    scrollRef.current?.scrollTo({ top: 0 });
  };

  if (loading) return <div className="flex items-center justify-center py-20"><Spinner size="lg" /></div>;
  if (error) return <EmptyState actions={<Button onClick={reload} variant="primary">重试</Button>} description={error} title="加载失败" />;

  return (
    <div className="flex min-h-0 flex-1 flex-col px-4">
      <div className="py-2">
        <SegmentedControl
          className="w-full"
          label="刮削状态"
          onChange={(value) => setCategory(value as ScrapeTaskCategory)}
          value={category}
        >
          {(Object.keys(SCRAPE_CATEGORY_LABELS) as ScrapeTaskCategory[]).map((c) => (
            <SegmentedControlItem
              key={c}
              label={`${SCRAPE_CATEGORY_LABELS[c]} ${counts[c]}`}
              value={c}
            />
          ))}
        </SegmentedControl>
      </div>

      <div className="scroll-area min-h-0 flex-1 pb-4" data-scroll-key="main" ref={scrollRef}>
        {tasks.length === 0 ? (
          <EmptyState
            description="在桌面端发起刮削后会出现在这里"
            title="暂无刮削任务"
          />
        ) : shown.length === 0 ? (
          <EmptyState title={`没有${SCRAPE_CATEGORY_LABELS[category]}的任务`} />
        ) : (
          <>
            <List className="flex flex-col gap-2">
              {pageTasks.map((t) => (
                <ListItem
                  className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)]"
                  description={
                    <>
                      <span>
                        {scrapeStatusLabel(t.status)}
                        {t.attempts > 0 ? ` · 尝试 ${t.attempts}/${t.maxAttempts}` : ""}
                        {t.errorCode ? ` · ${t.errorCode}` : ""}
                      </span>
                      {t.lastError ? (
                        <span className="mt-1 line-clamp-2 block text-[var(--danger)]">
                          {t.lastError}
                        </span>
                      ) : null}
                      <span className="mt-1 block">{formatDateTime(t.createdAt)}</span>
                    </>
                  }
                  endContent={
                    t.video.mosaicRestored ? (
                      <Badge label="马赛克破解" variant="purple" />
                    ) : undefined
                  }
                  key={t.id}
                  label={videoName(t.video)}
                  onClick={() => onOpenVideo(t.video)}
                />
              ))}
            </List>
            <Pagination
              page={paging.page}
              pageCount={paging.pageCount}
              total={shown.length}
              unit="条"
              onPageChange={turnPage}
            />
          </>
        )}
      </div>
    </div>
  );
}
