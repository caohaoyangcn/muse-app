import { useState } from "react";

import type { Video } from "../../types";
import { formatDateTime } from "../../utils/task";
import { cn } from "../../lib/utils";
import { EmptyState, ErrorState, Loading } from "./States";
import RestoredBadge from "../../ui/restored-badge";
import { videoName } from "../videoDisplay";
import {
  SCRAPE_CATEGORY_LABELS,
  scrapeStatusLabel,
  useScrapeTasks,
  type ScrapeTaskCategory,
} from "../../views/scrape/useScrapeTasks";

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

  if (loading) return <Loading />;
  if (error) return <ErrorState message={error} onRetry={reload} />;

  return (
    <div className="flex min-h-0 flex-1 flex-col px-4">
      <div className="flex gap-2 py-2">
        {(Object.keys(SCRAPE_CATEGORY_LABELS) as ScrapeTaskCategory[]).map((c) => (
          <button
            className={cn(
              "flex-1 rounded-full border px-3 py-2 text-sm",
              c === category
                ? "border-transparent bg-[var(--accent)] text-white"
                : "border-[var(--border)] bg-[var(--panel-2)] text-[var(--muted)]",
            )}
            key={c}
            onClick={() => setCategory(c)}
            type="button"
          >
            {SCRAPE_CATEGORY_LABELS[c]} {counts[c]}
          </button>
        ))}
      </div>

      <div className="scroll-area min-h-0 flex-1 pb-4" data-scroll-key="main">
        {tasks.length === 0 ? (
          <EmptyState
            hint="在桌面端发起刮削后会出现在这里"
            title="暂无刮削任务"
          />
        ) : shown.length === 0 ? (
          <EmptyState title={`没有${SCRAPE_CATEGORY_LABELS[category]}的任务`} />
        ) : (
          <div className="flex flex-col gap-2">
            {shown.map((t) => (
              <button
                className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)] p-3 text-left active:scale-[0.99]"
                key={t.id}
                onClick={() => onOpenVideo(t.video)}
                type="button"
              >
                <div className="flex min-w-0 items-center gap-1.5">
                  <div className="truncate text-sm font-medium">
                    {videoName(t.video)}
                  </div>
                  {t.video.mosaicRestored && <RestoredBadge />}
                </div>
                <div className="mt-1 text-xs text-[var(--muted)]">
                  {scrapeStatusLabel(t.status)}
                  {t.attempts > 0 ? ` · 尝试 ${t.attempts}/${t.maxAttempts}` : ""}
                  {t.errorCode ? ` · ${t.errorCode}` : ""}
                </div>
                {t.lastError ? (
                  <div className="mt-1 line-clamp-2 text-xs text-[var(--danger)]">
                    {t.lastError}
                  </div>
                ) : null}
                <div className="mt-1 text-xs text-[var(--muted)]">
                  {formatDateTime(t.createdAt)}
                </div>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
