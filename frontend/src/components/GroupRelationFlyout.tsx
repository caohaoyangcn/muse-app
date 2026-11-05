import { useEffect, useRef, useState } from "react";
import type { Group } from "../types";
import FlyoutShell from "./FlyoutShell";
import { cn } from "../lib/utils";

interface GroupRelationFlyoutProps {
  mode: "child" | "parent";
  group: Group;
  groups: Group[]; // all groups, to build candidate / child lists
  anchor: DOMRect;
  onSetParent: (id: number, parentId: number | null) => Promise<void>;
  onClose: () => void;
}

/**
 * Fly-out beside the group context menu that edits the parent/child relation:
 *  - mode "child": every other group listed as a checkbox; checked groups are
 *    direct children of `group`. Toggling adds/removes that child relationship.
 *  - mode "parent": single-choice picker for which other group is the parent of
 *    `group`, plus an option to clear it back to top-level.
 * Every change saves instantly through setGroupParent (live, no Save button).
 */
export default function GroupRelationFlyout({
  mode,
  group,
  groups,
  anchor,
  onSetParent,
  onClose,
}: GroupRelationFlyoutProps) {
  // Direct children of this group (for "child" mode).
  const [childIds, setChildIds] = useState<Set<number>>(
    () => new Set(groups.filter((g) => g.parentId === group.id).map((g) => g.id)),
  );
  const [pending, setPending] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    window.addEventListener("keydown", onKey);
    window.addEventListener("mousedown", onDown);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("mousedown", onDown);
    };
  }, [onClose]);

  // Valid candidates differ by mode:
  //  - child:  any group that isn't this group or one of its ancestors
  //            (a child can be a descendant/unrelated, never an ancestor).
  //  - parent: any group that isn't this group or one of its descendants
  //            (a parent can be an ancestor/unrelated, never a descendant).
  const related =
    mode === "child" ? collectAncestors(group, groups) : collectDescendants(group, groups);
  const blocked = new Set(related);
  blocked.add(group.id);
  const candidates = groups.filter((g) => !blocked.has(g.id));

  const save = (id: number, parentId: number | null) => {
    setPending(true);
    onSetParent(id, parentId)
      .then(() => {
        setPending(false);
        // Update local child set so the flyout reflects the change without a reload.
        if (mode === "child" && parentId === group.id) {
          setChildIds((prev) => new Set(prev).add(id));
        } else if (mode === "child" && parentId === null) {
          setChildIds((prev) => {
            const next = new Set(prev);
            next.delete(id);
            return next;
          });
        }
      })
      .catch(() => setPending(false));
  };

  const toggleChild = (id: number) => {
    save(id, childIds.has(id) ? null : group.id);
  };

  return (
    <FlyoutShell anchor={anchor} side="right">
      <div ref={ref} className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel-2)] shadow-[0_8px_24px_rgba(0,0,0,0.45)]">
        <div className="shrink-0 border-b border-[var(--border)] px-3 pb-1.5 pt-2 text-xs text-[var(--muted)]">
          {mode === "child" ? (
            <>将“{group.name}”设为父级的子分组：</>
          ) : (
            <>选择“{group.name}”的父级：</>
          )}
        </div>

        <div className="min-h-10 flex-1 overflow-y-auto p-1.5">
          {candidates.length === 0 ? (
            <div className="p-3 text-xs text-[var(--muted)]">
              {mode === "child"
                ? "没有可设为子分组的其他分组"
                : "没有可设为父级的其他分组"}
            </div>
          ) : mode === "child" ? (
            candidates.map((g) => (
              <div
                key={g.id}
                className={cn(
                  "flex cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-2 hover:bg-[var(--panel)]",
                  childIds.has(g.id) && "bg-[var(--panel)]",
                )}
                onClick={() => !pending && toggleChild(g.id)}
                title={`${g.name} (${g.videoCount})`}
              >
                <input type="checkbox" checked={childIds.has(g.id)} onChange={() => toggleChild(g.id)} />
                <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{g.name}</span>
                <span className="text-xs text-[var(--muted)]">{g.videoCount}</span>
              </div>
            ))
          ) : (
            <>
              <div
                className="flex cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-2 hover:bg-[var(--panel)]"
                onClick={() => !pending && save(group.id, null)}
                title="取消父级，作为顶层分组"
              >
                <input type="checkbox" checked={group.parentId == null} readOnly />
                <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">（作为顶层分组）</span>
              </div>
              {candidates.map((g) => (
                <div
                  key={g.id}
                  className={cn(
                    "flex cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-2 hover:bg-[var(--panel)]",
                    group.parentId === g.id && "bg-[var(--panel)]",
                  )}
                  onClick={() => !pending && save(group.id, g.id)}
                  title={`${g.name} (${g.videoCount})`}
                >
                  <input
                    type="checkbox"
                    checked={group.parentId === g.id}
                    readOnly
                  />
                  <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{g.name}</span>
                  <span className="text-xs text-[var(--muted)]">{g.videoCount}</span>
                </div>
              ))}
            </>
          )}
        </div>

        <div className="shrink-0 border-t border-[var(--border)] px-3 pb-2 pt-1.5 text-[11px] text-[var(--muted)]">{pending ? "保存中…" : "点击立即生效"}</div>
      </div>
    </FlyoutShell>
  );
}

// collectAncestors returns the group's ancestor ids (walking up the parent
// chain). An ancestor can never be made a child of the group (would cycle).
function collectAncestors(root: Group, groups: Group[]): Set<number> {
  const parentOf = new Map<number, number>();
  for (const g of groups) {
    if (g.parentId != null) parentOf.set(g.id, g.parentId);
  }
  const out = new Set<number>();
  let cur = parentOf.get(root.id);
  while (cur != null) {
    out.add(cur);
    cur = parentOf.get(cur);
  }
  return out;
}

// collectDescendants returns the group's descendant ids. A descendant can never
// be made the parent of the group (would cycle).
function collectDescendants(root: Group, groups: Group[]): Set<number> {
  const children = new Map<number, number[]>();
  for (const g of groups) {
    if (g.parentId != null) {
      const list = children.get(g.parentId) ?? [];
      list.push(g.id);
      children.set(g.parentId, list);
    }
  }
  const out = new Set<number>();
  const stack = children.get(root.id) ?? [];
  while (stack.length) {
    const id = stack.pop()!;
    out.add(id);
    for (const c of children.get(id) ?? []) stack.push(c);
  }
  return out;
}
