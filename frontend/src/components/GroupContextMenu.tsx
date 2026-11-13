import type { Group } from "../types";
import GroupRelationFlyout from "./GroupRelationFlyout";
import { MenuSurface, type MenuItemSpec } from "../ui/menu";

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
 * Positioning, outside-click/Escape closing and the hover-intent for the two
 * flyouts all come from MenuSurface; this file only describes the rows.
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
  const items: MenuItemSpec[] = [
    { key: "rename", label: "重命名", onSelect: () => onAction({ kind: "rename" }) },
    {
      key: "addParent",
      label: "添加父级…",
      renderFlyout: (anchor) => (
        <GroupRelationFlyout
          mode="parent"
          group={group}
          groups={groups}
          anchor={anchor}
          onSetParent={onSetParent}
          onClose={onClose}
        />
      ),
    },
    {
      key: "addChild",
      label: "添加子级…",
      renderFlyout: (anchor) => (
        <GroupRelationFlyout
          mode="child"
          group={group}
          groups={groups}
          anchor={anchor}
          onSetParent={onSetParent}
          onClose={onClose}
        />
      ),
    },
    {
      key: "delete",
      label: "删除分组",
      danger: true,
      onSelect: () => onAction({ kind: "delete" }),
    },
  ];

  return <MenuSurface items={items} label="分组操作" onClose={onClose} x={x} y={y} />;
}
