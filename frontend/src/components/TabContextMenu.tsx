import type { WorkspaceTab } from "../types";
import { DropdownMenuItem } from "../ui/astryx";
import { MenuSurface } from "../ui/menu";

export type TabMenuAction = "close" | "closeOthers" | "closeRight" | "closeAll";

interface TabContextMenuProps {
  x: number;
  y: number;
  tab: WorkspaceTab;
  label: string;
  /** Open tabs other than the library home. */
  tabCount: number;
  /** Tabs that sit to the right of this one in the strip. */
  rightCount: number;
  onAction: (action: TabMenuAction, tab: WorkspaceTab) => void;
  onClose: () => void;
}

/**
 * Right-click menu for a workspace tab, using Astryx menu rows. This is the
 * primary close path now that tabs carry no per-tab close button.
 */
export default function TabContextMenu({
  x,
  y,
  tab,
  label,
  tabCount,
  rightCount,
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
        isDisabled={rightCount === 0}
        label="关闭右侧标签页"
        onClick={() => onAction("closeRight", tab)}
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
