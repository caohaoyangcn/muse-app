import { useState } from "react";
import type { Group } from "../types";
import {
  DropdownMenuCheckboxItem,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSubMenu,
} from "../ui/astryx";

interface GroupRelationFlyoutProps {
  mode: "child" | "parent";
  group: Group;
  groups: Group[];
  onSetParent: (id: number, parentId: number | null) => Promise<void>;
}

/**
 * Astryx submenu that edits the parent/child relation:
 *  - mode "child": every valid group listed as a checkbox; checked groups are
 *    direct children of `group`.
 *  - mode "parent": a single-choice radio group for which group is the parent,
 *    plus an option to clear it back to top-level.
 * Every change saves instantly through setGroupParent (live, no Save button).
 */
export default function GroupRelationFlyout({
  mode,
  group,
  groups,
  onSetParent,
}: GroupRelationFlyoutProps) {
  const [childIds, setChildIds] = useState<Set<number>>(
    () => new Set(groups.filter((candidate) => candidate.parentId === group.id).map((candidate) => candidate.id)),
  );
  const [parentId, setParentId] = useState<number | null>(group.parentId ?? null);
  const [pending, setPending] = useState(false);

  const related =
    mode === "child" ? collectAncestors(group, groups) : collectDescendants(group, groups);
  const blocked = new Set(related);
  blocked.add(group.id);
  const candidates = groups.filter((candidate) => !blocked.has(candidate.id));

  const save = (id: number, nextParentId: number | null) => {
    setPending(true);
    onSetParent(id, nextParentId)
      .then(() => {
        setPending(false);
        if (mode === "child") {
          setChildIds((prev) => {
            const next = new Set(prev);
            if (nextParentId === group.id) next.add(id);
            else next.delete(id);
            return next;
          });
        } else {
          setParentId(nextParentId);
        }
      })
      .catch(() => setPending(false));
  };

  return (
    <DropdownMenuSubMenu
      footer={
        <div className="px-3 py-1.5 text-[11px] text-[var(--muted)]">
          {pending ? "保存中…" : "点击立即生效"}
        </div>
      }
      hasSpinner={pending}
      header={
        <div className="px-3 pb-1.5 pt-2 text-xs text-[var(--muted)]">
          {mode === "child" ? (
            <>将“{group.name}”设为父级的子分组：</>
          ) : (
            <>选择“{group.name}”的父级：</>
          )}
        </div>
      }
      label={mode === "child" ? "添加子级…" : "添加父级…"}
      menuWidth={260}
    >
      {candidates.length === 0 ? (
        <DropdownMenuItem
          isDisabled
          label={mode === "child" ? "没有可设为子分组的其他分组" : "没有可设为父级的其他分组"}
        />
      ) : mode === "child" ? (
        candidates.map((candidate) => (
          <DropdownMenuCheckboxItem
            endContent={<span className="text-xs text-[var(--muted)]">{candidate.videoCount}</span>}
            key={candidate.id}
            label={candidate.name}
            onChange={() => !pending && save(candidate.id, childIds.has(candidate.id) ? null : group.id)}
            value={childIds.has(candidate.id)}
          />
        ))
      ) : (
        <DropdownMenuRadioGroup
          hasCloseOnSelect={false}
          label="父级分组"
          onChange={(value) => {
            if (pending) return;
            const next = value === "root" ? null : Number(value);
            if (next !== parentId) save(group.id, next);
          }}
          value={parentId == null ? "root" : String(parentId)}
        >
          <DropdownMenuRadioItem label="（作为顶层分组）" value="root" />
          {candidates.map((candidate) => (
            <DropdownMenuRadioItem
              endContent={<span className="text-xs text-[var(--muted)]">{candidate.videoCount}</span>}
              key={candidate.id}
              label={candidate.name}
              value={String(candidate.id)}
            />
          ))}
        </DropdownMenuRadioGroup>
      )}
    </DropdownMenuSubMenu>
  );
}

function collectAncestors(root: Group, groups: Group[]): Set<number> {
  const parentOf = new Map<number, number>();
  for (const group of groups) {
    if (group.parentId != null) parentOf.set(group.id, group.parentId);
  }
  const out = new Set<number>();
  let current = parentOf.get(root.id);
  while (current != null) {
    out.add(current);
    current = parentOf.get(current);
  }
  return out;
}

function collectDescendants(root: Group, groups: Group[]): Set<number> {
  const children = new Map<number, number[]>();
  for (const group of groups) {
    if (group.parentId != null) {
      const list = children.get(group.parentId) ?? [];
      list.push(group.id);
      children.set(group.parentId, list);
    }
  }
  const out = new Set<number>();
  const stack = children.get(root.id) ?? [];
  while (stack.length) {
    const id = stack.pop()!;
    out.add(id);
    for (const child of children.get(id) ?? []) stack.push(child);
  }
  return out;
}
