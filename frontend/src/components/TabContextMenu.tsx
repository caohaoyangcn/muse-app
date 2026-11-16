import type { WorkspaceTab } from "../types";
import { DropdownMenuItem } from "../ui/astryx";
import { MenuSurface } from "../ui/menu";

export type TabMenuAction = "close" | "closeOthers" | "closeAll";

interface TabContextMenuProps {
  x: number;
  y: number;
  tab: WorkspaceTab;
  label: string;
  tabCount: number;
  onAction: (action: TabMenuAction, tab: WorkspaceTab) => void;
  onClose: () => void;
}

/** Right-click menu for a workspace tab, using Astryx menu rows. */
export default function TabContextMenu({
  x,
  y,
  tab,
  label,
  tabCount,
  onAction,
  onClose,
}: TabContextMenuProps) {
  const isLibrary = tab === "library";
  return (
    <MenuSurface label="标签页操作" onClose={onClose} x={x} y={y}>
      {isLibrary ? null : (
        <DropdownMenuItem label={`关闭 ${label}`} onClick={() => onAction("close", tab)} />
      )}
      <DropdownMenuItem
        isDisabled={isLibrary ? tabCount === 0 : tabCount <= 1}
        label="关闭其他标签页"
        onClick={() => onAction("closeOthers", tab)}
      />
      <DropdownMenuItem
        isDisabled={tabCount === 0}
        label="关闭所有标签页（回到主页）"
        onClick={() => onAction("closeAll", tab)}
        variant="destructive"
      />
    </MenuSurface>
  );
}
