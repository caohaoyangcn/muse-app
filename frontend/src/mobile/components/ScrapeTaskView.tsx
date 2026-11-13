import { useMemo, useState } from "react";

import type { JavTaskOverview, Video } from "../../types";
import { listJavTasks } from "../../services/http";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import { formatDateTime } from "../../utils/task";
import { cn } from "../../lib/utils";
import { EmptyState, ErrorState, Loading } from "./States";
import RestoredBadge from "../../components/RestoredBadge";
import { videoName } from "../videoDisplay";

type Category = "queued" | "succeeded" | "failed";

const CATEGORY_LABELS: Record<Category, string> = {
  queued: "进行中",
  succeeded: "已完成",
  failed: "失败",
};

// Stable identity for the empty board.
const NO_TASKS: JavTaskOverview[] = [];

/**
 * The scrape board. Read-only: the mobile client can inspect task status but
 * cannot trigger scraping (that stays on the desktop, where the Cookie and
 * provider configuration live).
 */
export function ScrapeTaskView({ onOpenVideo }: { onOpenVideo: (v: Video) => void }) {
  const { data, loading, error, reload } = useLiveQuery(listJavTasks, "scrape-tasks");
  const tasks = data ?? NO_TASKS;
  const [category, setCategory] = useState<Category>("queued");

  const counts = useMemo(() => {
    const acc: Record<Category, number> = { queued: 0, succeeded: 0, failed: 0 };
    for (const t of tasks) {
      const c = (t.category as Category) in acc ? (t.category as Category) : "failed";
      acc[c]++;
    }
    return acc;
  }, [tasks]);

  const shown = tasks.filter((t) => t.category === category);

  if (loading) return <Loading />;
  if (error) return <ErrorState message={error} onRetry={reload} />;

  return (
    <div className="flex min-h-0 flex-1 flex-col px-4">
      <div className="flex gap-2 py-2">
        {(Object.keys(CATEGORY_LABELS) as Category[]).map((c) => (
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
            {CATEGORY_LABELS[c]} {counts[c]}
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
          <EmptyState title={`没有${CATEGORY_LABELS[category]}的任务`} />
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
                  {t.status}
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
