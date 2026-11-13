import { useEffect, useRef, useState } from "react";
import type { MouseEvent as ReactMouseEvent } from "react";
import type { Group } from "../types";
import GroupRelationFlyout from "./GroupRelationFlyout";
import { cn } from "../lib/utils";
import { Button } from "../ui/button";

export type GroupMenuAction =
  | { kind: "rename" }
  | { kind: "delete" }
  | { kind: "addChild" }
  | { kind: "addParent" };

interface GroupContextMenuProps {
  x: number;
  y: number;
  group: Group;
  groups: Group[]; // all groups, used to build candidate trees for the flyouts
  onAction: (action: GroupMenuAction) => void;
  onSetParent: (id: number, parentId: number | null) => Promise<void>;
  onClose: () => void;
}

/**
 * Right-click menu for a group (in the sidebar). Besides rename/delete it
 * exposes parent/child editing:
 *  - 添加子级 opens a flyout that lists every other group as a toggleable child
 *    of the current group (editing child groups from a parent group).
 *  - 添加父级 opens a flyout to choose which existing group is the parent of the
 *    current group (editing the parent from a child group).
 * Both saves live through the same setGroupParent backend call.
 *
 * The flyout is rendered *inside* the menu surface and closes on hover-away
 * (hovering a normal item or leaving the whole surface), so it never lingers
 * after the cursor moves off. The flyout anchors to the opened sub-menu item
 * and flips left when the right side is full.
 */
export default function GroupContextMenu({
  x,
  y,
  group,
  groups,
  onAction,
  onSetParent,
  onClose,
}: GroupContextMenuProps) {
  const ref = useRef<HTMLDivElement>(null);
  const parentBtnRef = useRef<HTMLButtonElement>(null);
  const childBtnRef = useRef<HTMLButtonElement>(null);
  const [flyout, setFlyout] = useState<"child" | "parent" | null>(null);
  const leaveTimer = useRef<number | null>(null);

  // Clamp both horizontally and vertically so the menu (and its right-attached
  // flyout) always stays fully in the viewport. Menu visual min-width is 210px.
  const left = Math.min(x, window.innerWidth - 222);
  const top = Math.min(y, window.innerHeight - 200);

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

  const handleMouseOver = (e: ReactMouseEvent<HTMLDivElement>) => {
    const btn = (e.target as HTMLElement).closest("button");
    if (!btn) return;
    if (btn.dataset.flyout === "parent") setFlyout("parent");
    else if (btn.dataset.flyout === "child") setFlyout("child");
    else setFlyout(null);
  };

  const handleMouseLeave = () => {
    if (leaveTimer.current != null) window.clearTimeout(leaveTimer.current);
    leaveTimer.current = window.setTimeout(() => setFlyout(null), 100);
  };
  const handleMouseEnter = () => {
    if (leaveTimer.current != null) {
      window.clearTimeout(leaveTimer.current);
      leaveTimer.current = null;
    }
  };


  const items: { key: "rename" | "addParent" | "addChild" | "delete"; label: string; danger?: boolean; onClick: () => void }[] = [
    { key: "rename", label: "重命名", onClick: () => onAction({ kind: "rename" }) },
    {
      key: "addParent",
      label: "添加父级…",
      onClick: () => setFlyout("parent"),
    },
    {
      key: "addChild",
      label: "添加子级…",
      onClick: () => setFlyout("child"),
    },
    { key: "delete", label: "删除分组", danger: true, onClick: () => onAction({ kind: "delete" }) },
  ];

  const flyoutOpener: "addParent" | "addChild" | null =
    flyout === "parent" ? "addParent" : flyout === "child" ? "addChild" : null;

  const openParentFlyout = flyout === "parent" && parentBtnRef.current != null;
  const openChildFlyout = flyout === "child" && childBtnRef.current != null;

  return (
    <div
      ref={ref}
      className="fixed z-[200] min-w-[210px] rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel-2)] p-1.5 shadow-[0_8px_24px_rgba(0,0,0,0.45)]"
      style={{ left, top }}
      onMouseOver={handleMouseOver}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
    >
      {items.map((item) => (
        <Button
          variant="ghost"
          key={item.key}
          ref={item.key === "addParent" ? parentBtnRef : item.key === "addChild" ? childBtnRef : undefined}
          data-flyout={item.key === "addParent" ? "parent" : item.key === "addChild" ? "child" : undefined}
          className={cn(
            "h-auto w-full justify-start rounded-md px-3 py-2 text-left text-[var(--text)]",
            item.danger && "text-[var(--danger)]",
            flyoutOpener === item.key && "bg-[var(--border)]",
          )}
          onClick={() => {
            item.onClick();
            if (item.key === "rename" || item.key === "delete") onClose();
          }}
        >
          {item.label}
        </Button>
      ))}
      {openParentFlyout && (
        <GroupRelationFlyout
          mode="parent"
          group={group}
          groups={groups}
          anchor={parentBtnRef.current!.getBoundingClientRect()}
          onSetParent={onSetParent}
          onClose={() => setFlyout(null)}
        />
      )}
      {openChildFlyout && (
        <GroupRelationFlyout
          mode="child"
          group={group}
          groups={groups}
          anchor={childBtnRef.current!.getBoundingClientRect()}
                    onSetParent={onSetParent}
          onClose={() => setFlyout(null)}
        />
      )}
    </div>
  );
}
