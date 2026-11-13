import { useEffect, useRef } from "react";
import type { LucideIcon } from "lucide-react";
import { ChevronRight, Cloud, FileVideo, FolderSearch } from "lucide-react";
import { cn } from "../lib/utils";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";

export interface CloudSummary {
  count: number;
  onlineCount: number;
  loading: boolean;
  error: string | null;
}

interface AddSourceDialogProps {
  open: boolean;
  onPickLocal: () => void;
  onPickFolder: () => void;
  /**
   * Omit until the WebDAV source is wired. The cloud entry is then not
   * rendered, so the dialog never offers a dead end.
   */
  onPickCloud?: () => void;
  cloud?: CloudSummary | null;
  onClose: () => void;
}

/**
 * The source router behind "添加视频". It is intentionally not a file browser:
 * each entry closes this dialog and hands off to the picker or the cloud
 * browser, keeping the add flow identical regardless of where the file lives.
 *
 * Keyboard: 1/2/3 select, Up/Down moves between entries, Esc closes. The first
 * entry is focused on open, so a keyboard user reaches the local picker with
 * one additional Enter/Space.
 */
export default function AddSourceDialog({
  open,
  onPickLocal,
  onPickFolder,
  onPickCloud,
  cloud,
  onClose,
}: AddSourceDialogProps) {
  const localRef = useRef<HTMLButtonElement>(null);
  const folderRef = useRef<HTMLButtonElement>(null);
  const cloudRef = useRef<HTMLButtonElement>(null);

  const cloudAvailable = onPickCloud != null;

  // Radix focuses the content by default. Focus the first source instead, so
  // the keyboard path does not start on a container or the close button.
  useEffect(() => {
    if (!open) return;
    const frame = requestAnimationFrame(() => localRef.current?.focus());
    return () => cancelAnimationFrame(frame);
  }, [open]);

  const moveFocus = (delta: 1 | -1) => {
    const items = [localRef.current, folderRef.current, cloudRef.current].filter(
      (item): item is HTMLButtonElement => item != null && !item.disabled,
    );
    if (items.length === 0) return;
    const current = document.activeElement as HTMLButtonElement | null;
    const index = current ? items.indexOf(current) : -1;
    const next = index < 0 ? 0 : (index + delta + items.length) % items.length;
    items[next]?.focus();
  };

  const onKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "1") {
      event.preventDefault();
      onPickLocal();
      return;
    }
    if (event.key === "2") {
      event.preventDefault();
      onPickFolder();
      return;
    }
    if (event.key === "3" && cloudAvailable) {
      event.preventDefault();
      onPickCloud();
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      moveFocus(1);
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      moveFocus(-1);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="w-[520px] gap-0 p-0" onKeyDown={onKeyDown}>
        <DialogHeader className="px-5 pb-3 pt-5 pr-10">
          <DialogTitle>添加视频</DialogTitle>
          <DialogDescription>选择来源</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-2 px-5 pb-2">
          <SourceCard
            ref={localRef}
            icon={FileVideo}
            title="本地文件"
            description="选择一个或多个视频文件"
            shortcut="1"
            onClick={onPickLocal}
          />
          <SourceCard
            ref={folderRef}
            icon={FolderSearch}
            title="扫描文件夹"
            description="加入目录监控并立即扫描；之后新增的视频会自动收录"
            shortcut="2"
            onClick={onPickFolder}
          />
          {cloudAvailable && (
            <SourceCard
              ref={cloudRef}
              icon={Cloud}
              title="从网盘添加"
              description={cloudDescription(cloud)}
              shortcut="3"
              badge="可选"
              onClick={() => onPickCloud?.()}
            />
          )}
        </div>

        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-[var(--border)] px-5 py-3 text-[11px] text-[var(--muted)]">
          <span className="flex items-center gap-1.5">
            <Kbd>1</Kbd> 本地
            <Kbd>2</Kbd> 文件夹
            {cloudAvailable && (
              <>
                <Kbd>3</Kbd> 网盘
              </>
            )}
          </span>
          <span className="ml-auto flex items-center gap-1.5">
            <Kbd>Esc</Kbd> 关闭
          </span>
          <span className="w-full">也可以直接把文件拖进窗口</span>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function cloudDescription(cloud: CloudSummary | null | undefined): string {
  if (!cloud || cloud.loading) return "正在检查网盘连接…";
  if (cloud.error) return "无法读取网盘连接；仍可进入设置";
  if (cloud.count === 0) return "未配置；进入后可去设置";
  if (cloud.onlineCount === 0) return `${cloud.count} 个连接 · 离线`;
  return `${cloud.onlineCount} 个连接可用`;
}

function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="rounded-[5px] border border-[var(--border)] bg-[var(--panel)] px-1.5 py-0.5 font-sans text-[10px] text-[var(--text)]">
      {children}
    </kbd>
  );
}

const SourceCard = ({
  ref,
  icon: Icon,
  title,
  description,
  shortcut,
  badge,
  onClick,
}: {
  ref?: React.Ref<HTMLButtonElement>;
  icon: LucideIcon;
  title: string;
  description: string;
  shortcut: string;
  badge?: string;
  onClick: () => void;
}) => (
  <button
    ref={ref}
    type="button"
    onClick={onClick}
    className={cn(
      "group flex w-full items-center gap-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)] px-3.5 py-3 text-left transition-colors",
      "hover:border-[var(--accent)] hover:bg-[var(--panel-2)]",
      "focus-visible:border-[var(--accent)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]",
    )}
  >
    <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-[10px] bg-[var(--panel-2)] text-[var(--accent)]">
      <Icon className="h-[18px] w-[18px]" aria-hidden />
    </span>
    <span className="min-w-0 flex-1">
      <span className="flex items-center gap-2">
        <span className="text-sm font-medium text-[var(--text)]">{title}</span>
        {badge && (
          <span className="rounded-full border border-[var(--border)] px-1.5 py-0.5 text-[10px] leading-none text-[var(--muted)]">
            {badge}
          </span>
        )}
      </span>
      <span className="mt-0.5 block text-xs leading-relaxed text-[var(--muted)]">{description}</span>
    </span>
    <span className="flex shrink-0 items-center gap-1 text-[var(--muted)]">
      <kbd className="rounded-[5px] border border-[var(--border)] bg-[var(--panel-2)] px-1.5 py-0.5 font-sans text-[10px]">
        {shortcut}
      </kbd>
      <ChevronRight className="h-4 w-4 transition-colors group-hover:text-[var(--accent)]" aria-hidden />
    </span>
  </button>
);
