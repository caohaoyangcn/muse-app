import type { CloudDownloadProgress } from "../types";
import { formatFileSize } from "../utils/format";
import { cn } from "../lib/utils";
import { Button } from "./ui/button";
import { CloudDownload, X } from "lucide-react";

/** A small bottom-right progress card for the one remote file being cached
 *  before the system player opens it. It is not a toast: it can be cancelled. */
export default function CloudDownloadBar({
  progress,
  onCancel,
  onDismiss,
}: {
  progress: CloudDownloadProgress;
  onCancel: () => void;
  onDismiss: () => void;
}) {
  const active = progress.status === "downloading";
  const percent =
    progress.total > 0 ? Math.min(100, Math.round((progress.downloaded / progress.total) * 100)) : 0;
  return (
    <div className="fixed bottom-4 right-4 z-[390] w-[340px] rounded-lg border border-[var(--border)] bg-[var(--panel-2)] p-3.5 shadow-[0_8px_24px_rgba(0,0,0,0.45)]">
      <div className="flex items-start gap-2">
        <CloudDownload className="mt-0.5 h-4 w-4 shrink-0 text-[var(--accent)]" aria-hidden />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium" title={progress.fileName}>
            {progress.fileName || "远程视频"}
          </div>
          <div className={cn("mt-0.5 text-[11px]", progress.status === "error" ? "text-[var(--danger)]" : "text-[var(--muted)]")}>
            {active
              ? progress.total > 0
                ? `正在缓存 ${percent}% · ${formatFileSize(progress.downloaded)} / ${formatFileSize(progress.total)}`
                : `正在缓存 · ${formatFileSize(progress.downloaded)}`
              : progress.status === "done"
                ? "缓存完成，正在打开…"
                : progress.status === "cancelled"
                  ? "已取消下载"
                  : progress.error || "下载失败"}
          </div>
        </div>
        {active ? (
          <Button size="sm" variant="ghost" onClick={onCancel} aria-label="取消下载">
            取消
          </Button>
        ) : (
          <button
            type="button"
            className="rounded p-1 text-[var(--muted)] hover:text-[var(--text)]"
            onClick={onDismiss}
            aria-label="关闭下载提示"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>
      <div className="mt-2 h-1 overflow-hidden rounded-full bg-[var(--border)]" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}>
        <div
          className={cn(
            "h-full rounded-full transition-[width] duration-200",
            progress.status === "error" ? "bg-[var(--danger)]" : "bg-[var(--accent)]",
            active && progress.total <= 0 && "w-1/3 animate-pulse",
          )}
          style={active && progress.total <= 0 ? undefined : { width: `${percent}%` }}
        />
      </div>
    </div>
  );
}
