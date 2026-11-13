import { useEffect, useRef, useState } from "react";
import { EventsOn } from "../../wailsjs/runtime";
import { RefreshCw } from "lucide-react";
import { fileScanStatus, scanFileStatuses } from "../services/api";
import type { FileScanStatus } from "../types";
import type { ToastItem } from "../ui/toast";
import { Button } from "../ui/button";
import { cn } from "../lib/utils";

/**
 * 检测文件: the file-existence check, on demand.
 *
 * The app already runs this pass at startup (a file confirmed gone on an
 * attached volume is cleaned up; a file that only looked lost comes back). Until
 * this button existed that was the only trigger, so a video deleted in Explorer
 * while Muse was open stayed a normal card until the next launch — the volume
 * poll only reacts to a disk being plugged in, and playing one card only checks
 * that one file.
 *
 * The pass is asynchronous and can stat thousands of files on slow disks, so the
 * button shows the phase it is in and stays disabled rather than blocking the
 * library. The result arrives as a toast: silence would make a pass that found
 * nothing look like a pass that never ran.
 */
export default function FileScanButton({
  onNotify,
}: {
  /** The shell's toast sink. Passed through so the pass can report its result. */
  onNotify: (toast: Omit<ToastItem, "id">, ttl?: number) => void;
}) {
  const [status, setStatus] = useState<FileScanStatus | null>(null);
  // Kept in a ref so the subscriptions below are installed once: callers pass an
  // inline handler, and resubscribing on every render could drop an event.
  const notifyRef = useRef(onNotify);
  notifyRef.current = onNotify;
  // Only a pass this button asked for is announced. The startup check goes
  // through the same events, and a toast for work the user did not request —
  // possibly minutes after launch — would be unexplained noise.
  const awaitingRef = useRef(false);

  useEffect(() => {
    let alive = true;
    // The last pass may be a finished startup check or one already running.
    void fileScanStatus()
      .then((current) => {
        if (alive) setStatus((prev) => newest(prev, current));
      })
      .catch(() => {
        /* the button still works; this is only the initial label */
      });
    const offProgress = EventsOn("video:filescan:progress", (next?: FileScanStatus) => {
      if (next) setStatus((prev) => newest(prev, next));
    });
    const offDone = EventsOn("video:filescan:done", (next?: FileScanStatus) => {
      if (!next) return;
      setStatus((prev) => newest(prev, next));
      if (!awaitingRef.current) return;
      awaitingRef.current = false;
      notifyRef.current(summarize(next), 6000);
    });
    return () => {
      alive = false;
      offProgress();
      offDone();
    };
  }, []);

  const start = async () => {
    awaitingRef.current = true;
    try {
      const started = await scanFileStatuses();
      setStatus((prev) => newest(prev, started));
    } catch (e) {
      awaitingRef.current = false;
      notifyRef.current({ kind: "error", message: "无法开始文件检测", details: [String(e)] });
    }
  };

  const running = status?.running ?? false;
  const checked = status?.checked ?? 0;
  const total = status?.total ?? 0;
  // The total only exists once the backend has probed the volumes, so the first
  // moment of a pass shows a plain "检测中…" rather than a misleading 0/0.
  const label = running ? (total > 0 ? `检测中 ${checked}/${total}` : "检测中…") : "检测文件";

  return (
    <Button
      size="sm"
      disabled={running}
      onClick={() => void start()}
      title={buttonTitle(status)}
      aria-label="检测每个收藏的文件是否存在"
      aria-busy={running}
    >
      <RefreshCw className={cn("h-3.5 w-3.5", running && "animate-spin")} aria-hidden />
      <span className="tabular-nums">{label}</span>
    </Button>
  );
}

/**
 * Keeps the newest snapshot of the pass. A small library can finish before the
 * start call answers, and events travel on a different channel than the binding
 * reply: without an ordering rule the late "running" answer could overwrite the
 * finished status and leave the button spinning forever. The pass's start time
 * identifies it; within one pass, a finish is newer than its start.
 */
function newest(prev: FileScanStatus | null, next: FileScanStatus): FileScanStatus {
  if (!prev) return next;
  if (next.startedAt !== prev.startedAt) return next.startedAt > prev.startedAt ? next : prev;
  return next.finishedAt >= prev.finishedAt ? next : prev;
}

/**
 * The tooltip. The last pass is summarized here rather than in the toolbar: the
 * startup check already ran, so this is also where "when was this last checked"
 * lives without permanently widening the row.
 */
function buttonTitle(status: FileScanStatus | null): string {
  if (status?.running) return "正在检查每个收藏的文件是否存在";
  if (!status || status.finishedAt === 0) {
    return "立即检查每个收藏的文件是否存在（与重启应用时执行的检查相同）";
  }
  let text = `上次检测：${new Date(status.finishedAt).toLocaleString()}，检查 ${status.checked} 个文件`;
  if (status.removed > 0) text += `，清理 ${status.removed} 个缺失收藏`;
  if (status.skipped > 0) text += `，${status.skipped} 个因整卷保护未清理`;
  if (status.revived > 0) text += `，恢复 ${status.revived} 个`;
  if (status.offline > 0) text += `，${status.offline} 个所在磁盘未接入`;
  if (status.remote > 0) text += `，${status.remote} 个网盘收藏跳过`;
  return text;
}

/**
 * The pass's result as one toast. A clean pass is stated too: pressing the
 * button and seeing nothing happen would look like the button is broken, which
 * is the opposite of what it is for.
 */
function summarize(status: FileScanStatus): Omit<ToastItem, "id"> {
  if (status.error) {
    return { kind: "error", message: "文件检测未能完成", details: [status.error] };
  }

  const details: string[] = [];
  if (status.removed > 0) {
    details.push(`已清理 ${status.removed} 个文件缺失的收藏（只删除记录与缩略图，源文件未被删除）`);
  }
  if (status.skipped > 0) {
    details.push(
      `${status.skipped} 个缺失收藏未清理：该磁盘上的收藏全部找不到文件，已保留记录以防磁盘未正确接入`,
    );
  }
  if (status.revived > 0) {
    details.push(`有 ${status.revived} 个文件重新出现，已恢复为可用`);
  }
  if (status.offline > 0) {
    details.push(`${status.offline} 个收藏所在磁盘未接入，本次未检测`);
  }
  if (status.remote > 0) {
    details.push(`${status.remote} 个网盘收藏不参与本地文件检测，可打开视频时单独校验`);
  }

  if (status.checked === 0) {
    return {
      kind: "info",
      message:
        status.offline > 0 ? "没有可检测的文件：相关磁盘未接入" : "收藏库为空，没有可检测的文件",
      details,
    };
  }
  if (details.length === 0) {
    return { kind: "success", message: `文件检测完成：${status.checked} 个文件都存在` };
  }
  return { kind: "info", message: `文件检测完成：检查 ${status.checked} 个文件`, details };
}
