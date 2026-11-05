import { useEffect, useState } from "react";
import type { Group } from "../types";
import { videoGroupIDs } from "../services/api";
import FlyoutShell from "./FlyoutShell";
import { cn } from "../lib/utils";

interface GroupFlyoutProps {
  videoId: number;
  videoName: string;
  groups: Group[];
  anchor: DOMRect;
  onSave: (videoId: number, groupIds: number[]) => Promise<void>;
  onClose: () => void;
}

/**
 * Inline fly-out shown next to the context menu for "Add to Group". Lists the
 * groups with checkboxes; a group is selected only when the user clicks it,
 * and every toggle saves instantly to the backend (live). No Save button.
 */
export default function GroupFlyout({
  videoId,
  videoName,
  groups,
  anchor,
  onSave,
  onClose,
}: GroupFlyoutProps) {
  const [checked, setChecked] = useState<Set<number>>(new Set());
  const [ready, setReady] = useState(false);

  useEffect(() => {
    videoGroupIDs(videoId)
      .then((ids) => setChecked(new Set(ids)))
      .finally(() => setReady(true));
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
    };
  }, [videoId, onClose]);

  const toggle = (id: number) => {
    const next = new Set(checked);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setChecked(next);
    const ids = Array.from(next);
    onSave(videoId, ids).catch((e) => {
      console.warn("group save failed", e);
    });
  };

  return (
    <FlyoutShell anchor={anchor} side="right">
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel-2)] shadow-[0_8px_24px_rgba(0,0,0,0.45)]">
        <div className="shrink-0 overflow-hidden text-ellipsis whitespace-nowrap border-b border-[var(--border)] px-3 pb-1.5 pt-2 text-xs text-[var(--muted)]" title={videoName}>
          添加“{videoName}”到：
        </div>
        {!ready ? (
          <div className="p-3 text-xs text-[var(--muted)]">加载中…</div>
        ) : groups.length === 0 ? (
          <div className="p-3 text-xs text-[var(--muted)]">暂无分组——请在侧边栏创建</div>
        ) : (
          <div className="min-h-10 flex-1 overflow-y-auto p-1.5">
            {groups.map((g) => (
              <div
                key={g.id}
                className={cn(
                  "flex cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-2 hover:bg-[var(--panel)]",
                  checked.has(g.id) && "bg-[var(--panel)]",
                )}
                onClick={() => toggle(g.id)}
                title={`${g.name} (${g.videoCount})`}
              >
                <input type="checkbox" checked={checked.has(g.id)} onChange={() => toggle(g.id)} />
                <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{g.name}</span>
                <span className="text-xs text-[var(--muted)]">{g.videoCount}</span>
              </div>
            ))}
          </div>
        )}
        <div className="shrink-0 border-t border-[var(--border)] px-3 pb-2 pt-1.5 text-[11px] text-[var(--muted)]">点击分组立即生效</div>
      </div>
    </FlyoutShell>
  );
}
