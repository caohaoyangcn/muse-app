import type { Group } from "../../types";
import { cn } from "../../lib/utils";
import { BottomSheet } from "./Sheet";

/**
 * The group filter. Renders the desktop sidebar's tree as a flat, readable list
 * (indented children) instead of the "胶囊" pills — a vertical list scrolls far
 * better on a phone than a wrapping pill cloud. Filtering only: group editing
 * stays on the desktop.
 */
export function GroupSheet({
  open,
  groups,
  activeId,
  onSelect,
  onClose,
}: {
  open: boolean;
  groups: Group[];
  activeId: number | "all";
  onSelect: (id: number | "all") => void;
  onClose: () => void;
}) {
  const roots = groups.filter((g) => g.parentId == null);

  return (
    <BottomSheet onClose={onClose} open={open} title="按分组筛选">
      <div className="flex flex-col gap-0.5">
        <GroupRow
          active={activeId === "all"}
          depth={0}
          label="全部"
          onClick={() => {
            onSelect("all");
            onClose();
          }}
        />
        {roots.map((g) => (
          <div key={g.id}>
            <GroupRow
              active={activeId === g.id}
              count={g.videoCount}
              depth={0}
              label={g.name}
              onClick={() => {
                onSelect(g.id);
                onClose();
              }}
            />
            {groups
              .filter((c) => c.parentId === g.id)
              .map((c) => (
                <GroupRow
                  active={activeId === c.id}
                  count={c.videoCount}
                  depth={1}
                  key={c.id}
                  label={c.name}
                  onClick={() => {
                    onSelect(c.id);
                    onClose();
                  }}
                />
              ))}
          </div>
        ))}
      </div>
    </BottomSheet>
  );
}

function GroupRow({
  label,
  count,
  depth,
  active,
  onClick,
}: {
  label: string;
  count?: number;
  depth: number;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      className={cn(
        "flex items-center gap-2 rounded-[var(--radius)] px-3 py-3 text-left text-[15px] active:bg-[var(--panel-2)]",
        active ? "text-[var(--accent)]" : "text-[var(--text)]",
      )}
      onClick={onClick}
      style={{ paddingLeft: 12 + depth * 16 }}
      type="button"
    >
      <span className="flex-1 truncate">{label}</span>
      {typeof count === "number" ? (
        <span className="text-xs text-[var(--muted)]">{count}</span>
      ) : null}
      {active ? <span className="text-[var(--accent)]">✓</span> : null}
    </button>
  );
}
