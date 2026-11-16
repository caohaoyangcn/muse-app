import type { Group } from "../types";
import { DropdownMenuDivider, DropdownMenuItem } from "../ui/astryx";
import { MenuSurface } from "../ui/menu";
import GroupRelationFlyout from "./GroupRelationFlyout";

export type GroupMenuAction =
  | { kind: "rename" }
  | { kind: "delete" }
  | { kind: "addChild" }
  | { kind: "addParent" };

interface GroupContextMenuProps {
  x: number;
  y: number;
  group: Group;
  groups: Group[];
  onAction: (action: GroupMenuAction) => void;
  onSetParent: (id: number, parentId: number | null) => Promise<void>;
  onClose: () => void;
}

/**
 * Right-click menu for a group (in the sidebar). Rename/delete are plain
 * Astryx rows; parent/child editing are Astryx submenus whose checkbox/radio
 * rows save immediately through setGroupParent.
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
  return (
    <MenuSurface label="分组操作" onClose={onClose} x={x} y={y}>
      <DropdownMenuItem label="重命名" onClick={() => onAction({ kind: "rename" })} />
      <GroupRelationFlyout
        group={group}
        groups={groups}
        mode="parent"
        onSetParent={onSetParent}
      />
      <GroupRelationFlyout
        group={group}
        groups={groups}
        mode="child"
        onSetParent={onSetParent}
      />
      <DropdownMenuDivider />
      <DropdownMenuItem
        label="删除分组"
        onClick={() => onAction({ kind: "delete" })}
        variant="destructive"
      />
    </MenuSurface>
  );
}
