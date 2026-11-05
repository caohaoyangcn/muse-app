import { X } from "lucide-react";
import type { MouseEvent as ReactMouseEvent } from "react";
import type { WorkspaceTab } from "../types";
import { cn } from "../lib/utils";

/** One rendered tab in the strip: its identity plus the label shown. */
export interface WorkspaceTabItem {
  tab: WorkspaceTab;
  label: string;
}

export default function WorkspaceTabs({
  tabs,
  active,
  onSelect,
  onClose,
  onContextMenu,
}: {
  tabs: WorkspaceTabItem[];
  active: WorkspaceTab;
  onSelect: (tab: WorkspaceTab) => void;
  onClose: (tab: WorkspaceTab) => void;
  onContextMenu: (e: ReactMouseEvent, tab: WorkspaceTab) => void;
}) {
  return (
    <nav
      className="flex min-h-10 shrink-0 items-end gap-1 overflow-x-auto border-b border-[var(--border)] bg-[var(--panel)] px-3"
      aria-label="工作区标签"
    >
      <button
        type="button"
        className={cn(
          "h-10 shrink-0 border-b-2 px-4 text-sm transition-colors",
          active === "library"
            ? "border-[var(--accent)] text-[var(--text)]"
            : "border-transparent text-[var(--muted)] hover:text-[var(--text)]",
        )}
        aria-current={active === "library" ? "page" : undefined}
        onClick={() => onSelect("library")}
        onContextMenu={(e) => onContextMenu(e, "library")}
      >
        影片库
      </button>
      {tabs.map(({ tab, label }) => {
        const selected = active === tab;
        return (
          <div
            key={String(tab)}
            className={cn(
              "group flex h-10 shrink-0 items-center border-b-2 transition-colors",
              selected ? "border-[var(--accent)]" : "border-transparent",
            )}
            onContextMenu={(e) => onContextMenu(e, tab)}
          >
            <button
              type="button"
              className={cn(
                "h-full max-w-[260px] truncate px-3 text-sm",
                selected ? "text-[var(--text)]" : "text-[var(--muted)] hover:text-[var(--text)]",
              )}
              aria-current={selected ? "page" : undefined}
              onClick={() => onSelect(tab)}
              title={label}
            >
              {label}
            </button>
            <button
              type="button"
              className="mr-1 inline-flex h-6 w-6 items-center justify-center rounded text-[var(--muted)] opacity-60 transition hover:bg-[var(--border)] hover:text-[var(--text)] group-hover:opacity-100 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
              aria-label={`关闭 ${label}`}
              title="关闭标签"
              onClick={(e) => {
                e.stopPropagation();
                onClose(tab);
              }}
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
        );
      })}
    </nav>
  );
}
