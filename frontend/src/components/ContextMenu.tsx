import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { MouseEvent as ReactMouseEvent } from "react";
import type { Group, Video } from "../types";
import GroupFlyout from "./GroupFlyout";
import { cn } from "../lib/utils";
import { videoDisplayName } from "../utils/videoName";
import { Button } from "./ui/button";

export type MenuAction = "open" | "details" | "openFolder" | "copyPath" | "edit" | "remove";

interface ContextMenuProps {
  x: number;
  y: number;
  video: Video;
  groups: Group[];
  onAction: (action: MenuAction, video: Video) => void;
  onSaveGroups: (videoId: number, groupIds: number[]) => Promise<void>;
  onClose: () => void;
}

const ITEMS: { action: MenuAction; label: string; danger?: boolean }[] = [
  { action: "open", label: "打开" },
  { action: "details", label: "查看详情…" },
  { action: "openFolder", label: "打开所在文件夹" },
  { action: "copyPath", label: "复制路径" },
  { action: "edit", label: "编辑信息…" },
  { action: "remove", label: "从收藏中移除", danger: true },
];

/**
 * Sub-menus are rendered *inside* the menu surface, not as siblings, and close
 * on hover-away. Hovering a sub-menu item opens its flyout; hovering any other
 * menu item closes the current flyout; leaving the whole surface closes it too
 * (after a short grace period so corner crossings don't flicker).
 *
 * The menu itself is clamped both ways so the flyout staying glued to the
 * sub-menu item's right edge can never be pushed outside the viewport: the
 * flyout uses the sub-menu item's measured rect as its anchor and flips to the
 * left if it would overflow the right edge.
 */
export default function ContextMenu({
  x,
  y,
  video,
  groups,
  onAction,
  onSaveGroups,
  onClose,
}: ContextMenuProps) {
  const ref = useRef<HTMLDivElement>(null);
  const groupsBtnRef = useRef<HTMLButtonElement>(null);
  const [submenu, setSubmenu] = useState<"groups" | null>(null);
  // Position starts unset: the menu must be rendered once to know its own size,
  // so it is laid out invisibly until measured. Clamping against a hardcoded
  // height clipped the last rows ("从收藏中移除" / "添加到分组…") whenever the
  // click landed low in the window, which is routine for cards near the bottom
  // of the grid — measure the real box instead.
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);
  const leaveTimer = useRef<number | null>(null);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const margin = 8;
    const { width, height } = el.getBoundingClientRect();
    setPos({
      left: Math.max(margin, Math.min(x, window.innerWidth - width - margin)),
      top: Math.max(margin, Math.min(y, window.innerHeight - height - margin)),
    });
  }, [x, y]);

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
      if (leaveTimer.current != null) window.clearTimeout(leaveTimer.current);
    };
  }, [onClose]);

  // Delegated hover: only buttons decide. Flyout rows are divs, so hovering
  // the flyout itself never closes it.
  const handleMouseOver = (e: ReactMouseEvent<HTMLDivElement>) => {
    const btn = (e.target as HTMLElement).closest("button");
    if (!btn) return;
    if (btn.dataset.submenu === "groups") setSubmenu("groups");
    else setSubmenu(null);
  };

  const handleMouseLeave = () => {
    if (leaveTimer.current != null) window.clearTimeout(leaveTimer.current);
    leaveTimer.current = window.setTimeout(() => setSubmenu(null), 100);
  };
  const handleMouseEnter = () => {
    if (leaveTimer.current != null) {
      window.clearTimeout(leaveTimer.current);
      leaveTimer.current = null;
    }
  };


  const openGroupFlyout = submenu === "groups" && groupsBtnRef.current != null;
  return (
    <div
      ref={ref}
      className="fixed z-[200] min-w-[210px] rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel-2)] p-1.5 shadow-[0_8px_24px_rgba(0,0,0,0.45)]"
      style={pos ? { left: pos.left, top: pos.top } : { left: x, top: y, visibility: "hidden" }}
      onMouseOver={handleMouseOver}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
    >
      {ITEMS.map((item) => (
        <Button
          variant="ghost"
          key={item.action}
          className={cn(
            "h-auto w-full justify-start rounded-md px-3 py-2 text-left text-[var(--text)]",
            item.danger && "text-[var(--danger)]",
          )}
          onClick={() => {
            onAction(item.action, video);
            onClose();
          }}
        >
          {item.action === "openFolder" && video.sourceId > 0
            ? "在 OpenList 中打开"
            : item.action === "copyPath" && video.sourceId > 0
              ? "复制网盘路径"
              : item.label}
        </Button>
      ))}
      <Button
        variant="ghost"
        ref={groupsBtnRef}
        data-submenu="groups"
        className={cn(
          "h-auto w-full justify-start rounded-md px-3 py-2 text-left text-[var(--text)]",
          submenu === "groups" && "bg-[var(--border)]",
        )}
        onClick={() => setSubmenu("groups")}
      >
        添加到分组…
      </Button>
      {openGroupFlyout && (
        <GroupFlyout
          videoId={video.id}
          videoName={videoDisplayName(video)}
          groups={groups}
          anchor={groupsBtnRef.current!.getBoundingClientRect()}
          onSave={onSaveGroups}
          onClose={() => setSubmenu(null)}
        />
      )}
    </div>
  );
}
