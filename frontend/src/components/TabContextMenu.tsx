import { useEffect, useRef } from "react";
import type { WorkspaceTab } from "../types";
import { cn } from "../lib/utils";
import { Button } from "../ui/button";

export type TabMenuAction = "close" | "closeOthers" | "closeAll";

interface TabContextMenuProps {
  x: number;
  y: number;
  /** The tab that was right-clicked. */
  tab: WorkspaceTab;
  /** Human-readable title of that tab, used in the "关闭 xxx" item. */
  label: string;
  /** Number of open non-library tabs (video and actor); decides which items
   *  can do anything. */
  tabCount: number;
  onAction: (action: TabMenuAction, tab: WorkspaceTab) => void;
  onClose: () => void;
}

/**
 * Right-click menu for a workspace tab. Closing the library tab itself is not
 * a thing (it is the home view), so on the library tab the menu only offers
 * "关闭其他标签页" and "关闭所有标签页", both of which keep the library tab and
 * drop every other tab.
 *
 * Positioned like ContextMenu: clamped both ways so it always stays inside the
 * viewport. Menu visual min-width is 210px.
 */
export default function TabContextMenu({
  x,
  y,
  tab,
  label,
  tabCount,
  onAction,
  onClose,
}: TabContextMenuProps) {
  const ref = useRef<HTMLDivElement>(null);

  const left = Math.min(x, window.innerWidth - 222);
  const top = Math.min(y, window.innerHeight - 150);

  useEffect(() => {
    const close = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", onKey);
    window.addEventListener("blur", onClose);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("blur", onClose);
    };
  }, [onClose]);

  const isLibrary = tab === "library";
  const items: { action: TabMenuAction; label: string; danger?: boolean; disabled?: boolean }[] = [
    ...(isLibrary ? [] : [{ action: "close" as const, label: `关闭 ${label}` }]),
    {
      action: "closeOthers",
      // On a non-library tab "other" means every other non-library tab; on the
      // library tab it means every non-library tab.
      label: "关闭其他标签页",
      disabled: isLibrary ? tabCount === 0 : tabCount <= 1,
    },
    {
      action: "closeAll",
      label: "关闭所有标签页（回到主页）",
      danger: true,
      disabled: tabCount === 0,
    },
  ];

  return (
    <div
      ref={ref}
      className="fixed z-[200] min-w-[210px] rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel-2)] p-1.5 shadow-[0_8px_24px_rgba(0,0,0,0.45)]"
      style={{ left, top }}
    >
      {items.map((item) => (
        <Button
          variant="ghost"
          key={item.action}
          disabled={item.disabled}
          className={cn(
            "h-auto w-full justify-start rounded-md px-3 py-2 text-left text-[var(--text)]",
            item.danger && "text-[var(--danger)]",
          )}
          onClick={() => {
            onAction(item.action, tab);
            onClose();
          }}
        >
          {item.label}
        </Button>
      ))}
    </div>
  );
}
