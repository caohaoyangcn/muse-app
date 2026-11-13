import type { Group, Video } from "../types";
import GroupFlyout from "./GroupFlyout";
import { videoDisplayName } from "../utils/videoName";
import { MenuSurface, type MenuItemSpec } from "../ui/menu";

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
  { action: "remove", label: "移入回收站", danger: true },
];

/**
 * Right-click menu for a library card.
 *
 * 添加到分组 is a submenu rather than a dialog: the flyout is rendered inside
 * the menu surface (see MenuSurface / FlyoutShell), so hovering into it keeps
 * the menu open, and the flyout flips to the left instead of leaving the
 * viewport when the menu sits near the right edge.
 *
 * Network-drive favorites have no local folder to reveal, so the two path rows
 * say what they actually do (open in OpenList / copy the remote path).
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
  const items: MenuItemSpec[] = [
    ...ITEMS.map((item) => ({
      key: item.action,
      danger: item.danger,
      label:
        item.action === "openFolder" && video.sourceId > 0
          ? "在 OpenList 中打开"
          : item.action === "copyPath" && video.sourceId > 0
            ? "复制网盘路径"
            : item.label,
      onSelect: () => onAction(item.action, video),
    })),
    {
      key: "groups",
      label: "添加到分组…",
      renderFlyout: (anchor) => (
        <GroupFlyout
          videoId={video.id}
          videoName={videoDisplayName(video)}
          groups={groups}
          anchor={anchor}
          onSave={onSaveGroups}
          onClose={onClose}
        />
      ),
    },
  ];

  return <MenuSurface items={items} onClose={onClose} x={x} y={y} />;
}
