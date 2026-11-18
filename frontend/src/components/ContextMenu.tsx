import type { Video } from "../types";
import { DropdownMenuItem } from "../ui/astryx";
import { MenuSurface } from "../ui/menu";

export type MenuAction = "open" | "details" | "openFolder" | "copyPath" | "edit" | "remove";

interface ContextMenuProps {
  x: number;
  y: number;
  video: Video;
  onAction: (action: MenuAction, video: Video) => void;
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
 * Right-click menu for a library card, built from Astryx DropdownMenu rows.
 * Network-drive favorites have no local folder to reveal, so the two path rows
 * say what they actually do (open in OpenList / copy the remote path).
 */
export default function ContextMenu({
  x,
  y,
  video,
  onAction,
  onClose,
}: ContextMenuProps) {
  return (
    <MenuSurface label="收藏操作" onClose={onClose} x={x} y={y}>
      {ITEMS.map((item) => (
        <DropdownMenuItem
          key={item.action}
          label={
            item.action === "openFolder" && video.sourceId > 0
              ? "在 OpenList 中打开"
              : item.action === "copyPath" && video.sourceId > 0
                ? "复制网盘路径"
                : item.label
          }
          onClick={() => onAction(item.action, video)}
          variant={item.danger ? "destructive" : "default"}
        />
      ))}
    </MenuSurface>
  );
}
