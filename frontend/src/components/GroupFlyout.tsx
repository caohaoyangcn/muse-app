import { useEffect, useState } from "react";
import type { Group } from "../types";
import { videoGroupIDs } from "../services/desktop";
import {
  DropdownMenuCheckboxItem,
  DropdownMenuItem,
  DropdownMenuSubMenu,
} from "../ui/astryx";

interface GroupFlyoutProps {
  videoId: number;
  videoName: string;
  groups: Group[];
  onSave: (videoId: number, groupIds: number[]) => Promise<void>;
}

/**
 * Inline Astryx submenu for "Add to Group". Lists the groups as checkbox menu
 * rows; every toggle saves instantly (live, no Save button). The menu root owns
 * hover-intent, focus and Escape; this component only owns membership state.
 */
export default function GroupFlyout({ videoId, videoName, groups, onSave }: GroupFlyoutProps) {
  const [checked, setChecked] = useState<Set<number>>(new Set());
  const [ready, setReady] = useState(false);

  useEffect(() => {
    videoGroupIDs(videoId)
      .then((ids) => setChecked(new Set(ids)))
      .finally(() => setReady(true));
  }, [videoId]);

  const toggle = (id: number) => {
    const next = new Set(checked);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setChecked(next);
    onSave(videoId, Array.from(next)).catch((e) => {
      console.warn("group save failed", e);
    });
  };

  return (
    <DropdownMenuSubMenu
      footer={
        <div className="px-3 py-1.5 text-[11px] text-[var(--muted)]">点击分组立即生效</div>
      }
      hasSpinner={!ready}
      header={
        <div
          className="overflow-hidden text-ellipsis whitespace-nowrap px-3 pb-1.5 pt-2 text-xs text-[var(--muted)]"
          title={videoName}
        >
          添加“{videoName}”到：
        </div>
      }
      label="添加到分组…"
      menuWidth={240}
    >
      {!ready ? (
        <DropdownMenuItem isDisabled label="加载…" />
      ) : groups.length === 0 ? (
        <DropdownMenuItem isDisabled label="暂无分组——请在侧边栏创建" />
      ) : (
        groups.map((group) => (
          <DropdownMenuCheckboxItem
            endContent={<span className="text-xs text-[var(--muted)]">{group.videoCount}</span>}
            key={group.id}
            label={group.name}
            onChange={() => toggle(group.id)}
            value={checked.has(group.id)}
          />
        ))
      )}
    </DropdownMenuSubMenu>
  );
}
