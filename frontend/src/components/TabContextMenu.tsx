import type { WorkspaceTab } from "../types";
import { MenuSurface, type MenuItemSpec } from "../ui/menu";

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
  const isLibrary = tab === "library";
  const items: MenuItemSpec[] = [
    ...(isLibrary
      ? []
      : [
          {
            key: "close",
            label: `关闭 ${label}`,
            onSelect: () => onAction("close", tab),
          } satisfies MenuItemSpec,
        ]),
    {
      key: "closeOthers",
      // On a non-library tab "other" means every other non-library tab; on the
      // library tab it means every non-library tab.
      label: "关闭其他标签页",
      disabled: isLibrary ? tabCount === 0 : tabCount <= 1,
      onSelect: () => onAction("closeOthers", tab),
    },
    {
      key: "closeAll",
      label: "关闭所有标签页（回到主页）",
      danger: true,
      disabled: tabCount === 0,
      onSelect: () => onAction("closeAll", tab),
    },
  ];

  return <MenuSurface items={items} onClose={onClose} x={x} y={y} />;
}
