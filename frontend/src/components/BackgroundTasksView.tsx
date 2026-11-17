import { useLiveQuery } from "../hooks/useLiveQuery";
import { client } from "../services/desktop";
import type { BackgroundTask } from "../types";
import { Badge, EmptyState, ProgressBar, Spinner, Toolbar } from "../ui/astryx";
import { Button } from "../ui/button";
import { taskEtaText, taskFinishedAtText, taskProgressText, taskRemaining } from "../utils/tasks";

// Stable identity for the empty dashboard, so the memo-free render below does
// not rebuild the array on every invalidation.
const NO_TASKS: BackgroundTask[] = [];

const STATE_LABELS: Record<string, { label: string; variant: "neutral" | "success" | "error" }> = {
  running: { label: "进行中", variant: "neutral" },
  done: { label: "已完成", variant: "success" },
  failed: { label: "失败", variant: "error" },
};

/**
 * 后台任务（任务看板）：one read-only row per task owner — 刮削、片名翻译、内容
 * 扫描、文件检测、演员资料补齐.
 *
 * The backend projects each service's own status into one snapshot; this view
 * deliberately owns no actions. Starting or stopping a task stays with that
 * task's own entry (settings, the board, the scan button), so the dashboard can
 * never disagree with the executor it is describing.
 */
export default function BackgroundTasksView({
  onOpenDetail,
}: {
  onOpenDetail?: (detail: string) => void;
}) {
  const { data, loading, error, reload } = useLiveQuery(
    client.backgroundTasks,
    "background-tasks",
  );
  const tasks = data?.tasks ?? NO_TASKS;
  const running = tasks.filter((task) => task.state === "running").length;

  return (
    <section className="mx-auto flex min-h-full w-full max-w-[920px] flex-col">
      <Toolbar
        className="sticky top-0 z-[5] rounded-none border-b border-[var(--border)] bg-[var(--bg)] px-0 py-3"
        label="后台任务"
        size="sm"
        startContent={<h2 className="mr-2 shrink-0 text-base font-semibold">后台任务</h2>}
        endContent={
          running > 0 ? (
            <span className="flex items-center gap-2 text-xs text-[var(--muted)]">
              <Spinner />
              {running} 个进行中
            </span>
          ) : (
            <span className="text-xs text-[var(--muted)]">空闲</span>
          )
        }
      />

      <p className="mt-3 max-w-[640px] text-[13px] leading-relaxed text-[var(--muted)]">
        刮削、片名翻译、内容扫描等任务的进度。这里只读；开始或停止任务仍在各自的入口。
      </p>

      <div className="mt-4 space-y-3 pb-6">
        {loading ? (
          <div className="flex min-h-[200px] items-center justify-center text-[var(--muted)]">
            加载中…
          </div>
        ) : error ? (
          <div className="flex flex-col items-start gap-3">
            <p className="text-[13px] text-[var(--danger)]">{error}</p>
            <Button onClick={reload}>重试</Button>
          </div>
        ) : tasks.length === 0 ? (
          <EmptyState
            description="开始刮削、翻译或扫描后，进度会显示在这里。"
            title="目前没有后台任务"
          />
        ) : (
          tasks.map((task) => (
            <TaskRow key={task.id} onOpenDetail={onOpenDetail} task={task} />
          ))
        )}
      </div>
    </section>
  );
}

/** One task: what it is, how far it got, and where its detail lives. */
function TaskRow({
  task,
  onOpenDetail,
}: {
  task: BackgroundTask;
  onOpenDetail?: (detail: string) => void;
}) {
  const state = STATE_LABELS[task.state] ?? { label: task.state, variant: "neutral" as const };
  const progress = taskProgressText(task);
  const remaining = taskRemaining(task);
  const eta = taskEtaText(task);
  const detail = task.detail;

  return (
    <article className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)] px-4 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{task.title}</span>
        <Badge label={state.label} variant={state.variant} />
        {detail && onOpenDetail ? (
          <Button className="ml-auto" size="sm" onClick={() => onOpenDetail(detail)}>
            查看明细
          </Button>
        ) : null}
      </div>

      {task.total > 0 ? (
        <ProgressBar
          className="mt-3"
          isLabelHidden
          label={`${task.title}进度`}
          max={task.total}
          value={Math.min(task.done, task.total)}
        />
      ) : null}

      {progress || remaining != null || eta ? (
        <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--muted)]">
          {progress ? <span>{progress}</span> : null}
          {remaining !== null && remaining > 0 ? (
            <span>
              还剩 {remaining} {task.unit}
            </span>
          ) : null}
          {eta ? <span>{eta}</span> : null}
        </div>
      ) : null}

      {task.current ? <p className="mt-1.5 text-[13px]">{task.current}</p> : null}
      {task.message ? (
        <p className="mt-1.5 text-[13px] text-[var(--muted)]">{task.message}</p>
      ) : null}
      {task.failed > 0 ? (
        <p className="mt-1.5 text-[13px] text-[var(--danger)]">
          失败 {task.failed} {task.unit}
        </p>
      ) : null}
      {task.error ? <p className="mt-1.5 text-[13px] text-[var(--danger)]">{task.error}</p> : null}
      {task.state !== "running" && task.updatedAt ? (
        <p className="mt-1.5 text-xs text-[var(--muted)]">
          上次完成于 {taskFinishedAtText(task.updatedAt)}
        </p>
      ) : null}
    </article>
  );
}
