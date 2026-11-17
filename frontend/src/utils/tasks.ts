import type { BackgroundTask } from "../types";

/**
 * How a background task's progress reads on the dashboard.
 *
 * The rules exist because the numbers are not always known: a scrape queue has
 * no batch boundary and a scan's second phase is not sized until the first one
 * ends, so those rows must show counts without inventing a denominator. These
 * helpers are shared so both clients phrase (and estimate) the same way.
 */

/** 进度文字：有总数时是 done/total，没有总数时只报已完成数。 */
export function taskProgressText(task: BackgroundTask): string {
  const unit = task.unit ? ` ${task.unit}` : "";
  if (task.total > 0) {
    return `${task.done}/${task.total}${unit}`;
  }
  if (task.done > 0) {
    return `已完成 ${task.done}${unit}`;
  }
  return "";
}

/**
 * 还剩多少条：总数已知时是 total - done - failed（失败也算处理过），
 * 否则 null —— 不猜。
 */
export function taskRemaining(task: BackgroundTask): number | null {
  if (task.total <= 0) return null;
  return Math.max(0, task.total - task.done - task.failed);
}

/**
 * 预计剩余时间。样本太少（刚开跑、只有一两条结果）时不显示，避免开屏第一秒
 * 报出一个几十小时的荒谬数字；返回 null 表示界面上不要画这一段。
 */
export function taskEtaText(task: BackgroundTask, now: number = Date.now()): string | null {
  if (task.state !== "running" || task.total <= 0) return null;
  const processed = task.done + task.failed;
  if (processed < 3) return null;
  if (!task.startedAt) return null;
  const elapsed = now - task.startedAt;
  if (elapsed < 5_000) return null;
  const remaining = taskRemaining(task);
  if (remaining === null || remaining === 0) return null;

  const perMs = processed / elapsed;
  const remainingMs = remaining / perMs;
  const minutes = Math.ceil(remainingMs / 60_000);
  if (minutes < 1) return "不到 1 分钟";
  if (minutes < 60) return `约 ${minutes} 分钟`;
  const hours = Math.round(minutes / 6) / 10;
  return `约 ${hours} 小时`;
}

/** “19:48”（今天）或 “01-15 09:20”（更早），用于已结束任务的上次完成时间。 */
export function taskFinishedAtText(ms: number, now: Date = new Date()): string {
  const at = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, "0");
  const time = `${pad(at.getHours())}:${pad(at.getMinutes())}`;
  const sameDay =
    at.getFullYear() === now.getFullYear() &&
    at.getMonth() === now.getMonth() &&
    at.getDate() === now.getDate();
  return sameDay ? time : `${pad(at.getMonth() + 1)}-${pad(at.getDate())} ${time}`;
}
