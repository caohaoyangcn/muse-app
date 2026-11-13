import { useEffect, useRef, useState } from "react";
import type { DragEvent as ReactDragEvent } from "react";
import type { Group, ViewFilter } from "../types";
import { MIME_VIDEO_DRAG, MIME_VIDEO_DRAG_MULTI } from "../types";
import { cn } from "../lib/utils";
import { Button } from "../ui/button";
import { Sidebar as SidebarRoot, SidebarNav, SidebarSection } from "../ui/sidebar";
import { NavIcon, type NavIconName } from "./NavIcon";

interface SidebarProps {
  groups: Group[];
  filter: ViewFilter;
  onCreate: () => void;
  onSelect: (filter: ViewFilter) => void;
  onReorder: (order: Group[]) => void;
  onSetParent: (id: number, parentId: number | null) => Promise<void>;
  onDropVideo: (group: Group, videoIds: number[]) => void;
  onGroupContextMenu: (e: React.MouseEvent, group: Group) => void;
}

const ROW =
  "relative flex h-8 w-full shrink-0 items-center gap-2 rounded-[var(--radius)] px-2 text-[13px] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]";

function rowClass(active: boolean, extra?: string) {
  return cn(
    ROW,
    active
      ? "bg-[rgba(124,92,255,0.12)] text-[var(--text)]"
      : "text-[var(--muted)] hover:bg-[var(--panel-2)] hover:text-[var(--text)]",
    extra,
  );
}

/** The 2px accent marker shared by every selected row. */
function ActiveBar() {
  return (
    <span
      aria-hidden="true"
      className="absolute left-0 top-1/2 h-4 w-0.5 -translate-y-1/2 rounded-full bg-[var(--accent)]"
    />
  );
}

/** One static entry in the browse or tool zone. */
function NavRow({
  icon,
  label,
  active,
  onClick,
  title,
  dataTour,
  onDragOver,
  onDrop,
}: {
  icon: NavIconName;
  label: string;
  active: boolean;
  onClick: () => void;
  title: string;
  dataTour?: string;
  onDragOver?: (e: ReactDragEvent<HTMLButtonElement>) => void;
  onDrop?: (e: ReactDragEvent<HTMLButtonElement>) => void;
}) {
  return (
    <button
      type="button"
      className={rowClass(active)}
      aria-current={active ? "page" : undefined}
      data-tour={dataTour}
      onClick={onClick}
      onDragOver={onDragOver}
      onDrop={onDrop}
      title={title}
    >
      {active && <ActiveBar />}
      <NavIcon name={icon} className={active ? "text-[var(--accent)]" : undefined} />
      <span className="min-w-0 flex-1 truncate text-left">{label}</span>
    </button>
  );
}

/**
 * Left navigation in three zones: 浏览 (smart views), 分组 (the reorderable
 * group tree), and 工具 (maintenance entries kept below the tree). The tree is
 * the only flexible child: it scrolls when long, so the browse entries and
 * tools stay visible without leaving an empty gap when the tree is short.
 *
 * Ordering/nesting use native HTML5 drag & drop:
 *  - dragging over a group's top/bottom edge inserts above/below (reorder);
 *  - dragging over a group's middle nests the dragged group as its child;
 *  - dragging onto "All Favorites" detaches a group back to the top level.
 * Each group is also a drop target for video cards (add to group). A drag may
 * carry a single video (MIME_VIDEO_DRAG) or a multi-selected batch
 * (MIME_VIDEO_DRAG_MULTI); both are decoded into a video-id list.
 *
 * Renaming/deleting/parenting all live in the group context menu, reachable by
 * right-click or by the `⋯` button that replaces the count on hover.
 */
export default function Sidebar({
  groups,
  filter,
  onCreate,
  onSelect,
  onReorder,
  onSetParent,
  onDropVideo,
  onGroupContextMenu,
}: SidebarProps) {
  const [dragIndex, setDragIndex] = useState<number | null>(null);
  const [overState, setOverState] = useState<
    { index: number; zone: "above" | "below" | "nest" } | null
  >(null);
  const dragId = useRef<number | null>(null);
  const [videoOverId, setVideoOverId] = useState<number | null>(null);
  // Child groups start collapsed so a deep tree does not flood the sidebar;
  // the chevron next to a group toggles it. Session-only (not persisted), so
  // every launch starts from the collapsed default.
  const [expandedIds, setExpandedIds] = useState<Set<number>>(new Set());

  const toggleExpanded = (id: number) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  // When the selected group sits under a collapsed parent, expand its ancestor
  // chain once so the active row is never hidden — on selection change or when
  // the group is re-parented. It is an expand, not a forced-open: the user can
  // still collapse the parent afterwards.
  const activeGroupParentID = filter.kind === "group"
    ? groups.find((g) => g.id === filter.id)?.parentId ?? null
    : null;
  useEffect(() => {
    if (activeGroupParentID == null) return;
    setExpandedIds((prev) => {
      const next = new Set(prev);
      const byId = new Map(groups.map((x) => [x.id, x]));
      const seen = new Set<number>();
      // The seen-set bounds a corrupt cyclic parent chain.
      let cursor: number | null = activeGroupParentID;
      while (cursor != null && !seen.has(cursor)) {
        seen.add(cursor);
        next.add(cursor);
        cursor = byId.get(cursor)?.parentId ?? null;
      }
      return next.size === prev.size ? prev : next;
    });
  }, [activeGroupParentID, groups]);

  const handleDragStart = (e: React.DragEvent, index: number, id: number) => {
    dragId.current = id;
    setDragIndex(index);
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", String(id));
  };

  const isVideoDrag = (e: React.DragEvent) => e.dataTransfer.types.includes(MIME_VIDEO_DRAG);
  const isMultiVideoDrag = (e: React.DragEvent) =>
    e.dataTransfer.types.includes(MIME_VIDEO_DRAG_MULTI);

  // Decode the dragged video ids: a multi-selection drag carries the ids in a
  // dedicated MIME type; a single-card drag carries just its own id.
  const draggedVideoIds = (e: React.DragEvent): number[] => {
    const types = Array.from(e.dataTransfer.types);
    if (types.includes(MIME_VIDEO_DRAG_MULTI)) {
      const raw = e.dataTransfer.getData(MIME_VIDEO_DRAG_MULTI);
      return raw
        .split(",")
        .map((s) => Number(s))
        .filter((n) => Number.isFinite(n) && n > 0);
    }
    if (types.includes(MIME_VIDEO_DRAG)) {
      const id = Number(e.dataTransfer.getData(MIME_VIDEO_DRAG));
      return Number.isFinite(id) && id > 0 ? [id] : [];
    }
    return [];
  };

  const handleDragOverGroup = (e: React.DragEvent, index: number, id: number) => {
    if (isVideoDrag(e) || isMultiVideoDrag(e)) {
      e.preventDefault();
      setVideoOverId(id);
      setOverState(null);
      return;
    }
    e.preventDefault();
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const frac = (e.clientY - rect.top) / rect.height;
    let zone: "above" | "below" | "nest";
    if (frac < 0.3) zone = "above";
    else if (frac > 0.7) zone = "below";
    else zone = "nest";
    setOverState({ index, zone });
  };

  const handleGroupDrop = (e: React.DragEvent, targetIndex: number) => {
    e.preventDefault();
    const draggedId = dragId.current;
    const zone = overState?.zone ?? "below";
    dragId.current = null;
    setDragIndex(null);
    setOverState(null);
    if (draggedId == null) return;

    if (zone === "nest" && draggedId !== groups[targetIndex].id) {
      void onSetParent(draggedId, groups[targetIndex].id);
      return;
    }

    const from = dragIndex;
    if (from == null || from === targetIndex) return;
    const next = [...groups];
    const [moved] = next.splice(from, 1);
    let insertAt = targetIndex;
    if (zone === "below" && targetIndex > from) insertAt = targetIndex - 1;
    next.splice(insertAt, 0, moved);
    onReorder(next);
  };

  const handleDropDetach = (e: React.DragEvent) => {
    e.preventDefault();
    const draggedId = dragId.current;
    dragId.current = null;
    setDragIndex(null);
    setOverState(null);
    if (draggedId != null) void onSetParent(draggedId, null);
  };

  const handleDropVideo = (e: React.DragEvent, group: Group) => {
    e.preventDefault();
    setVideoOverId(null);
    const ids = draggedVideoIds(e);
    if (ids.length > 0) onDropVideo(group, ids);
  };

  const topLevel = groups.filter((g) => g.parentId == null);

  const renderGroup = (g: Group, index: number) => {
    const children = groups.filter((c) => c.parentId === g.id);
    const hasChildren = children.length > 0;
    // Children are hidden unless explicitly expanded. While a group drag hovers
    // with the "nest" intent, force it open so the nesting being created is
    // visible.
    const expanded = expandedIds.has(g.id) || (overState?.zone === "nest" && overState.index === index);
    const active = filter.kind === "group" && filter.id === g.id;
    const over = overState?.index === index ? overState.zone : null;
    return (
      <div key={g.id}>
        <div
          className={cn(
            rowClass(active),
            "group/row cursor-grab active:cursor-grabbing",
            over && "bg-[rgba(124,92,255,0.08)] shadow-[inset_0_0_0_2px_var(--accent)]",
            over === "above" && "shadow-[inset_0_3px_0_var(--accent)]",
            over === "below" && "shadow-[inset_0_-3px_0_var(--accent)]",
            over === "nest" && "border border-dashed border-[var(--accent)] bg-[rgba(124,92,255,0.18)]",
            videoOverId === g.id && "border border-dashed border-[var(--accent)] bg-[rgba(124,92,255,0.16)] shadow-[inset_0_0_0_2px_var(--accent)]",
          )}
          role="button"
          tabIndex={0}
          aria-current={active ? "page" : undefined}
          onClick={() => onSelect({ kind: "group", id: g.id })}
          onKeyDown={(e) => {
            if (e.target !== e.currentTarget) return;
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              e.stopPropagation();
              onSelect({ kind: "group", id: g.id });
            }
          }}
          onContextMenu={(e) => onGroupContextMenu(e, g)}
          onDragStart={(e) => handleDragStart(e, index, g.id)}
          onDragOver={(e) => handleDragOverGroup(e, index, g.id)}
          onDragLeave={() => {
            setOverState((cur) => (cur && cur.index === index ? null : cur));
            setVideoOverId((cur) => (cur === g.id ? null : cur));
          }}
          onDrop={(e) => {
            if (isVideoDrag(e) || isMultiVideoDrag(e)) handleDropVideo(e, g);
            else handleGroupDrop(e, index);
          }}
          onDragEnd={() => {
            dragId.current = null;
            setDragIndex(null);
            setOverState(null);
          }}
          title={g.name}
        >
          {active && <ActiveBar />}
          {hasChildren ? (
            <button
              type="button"
              className="flex h-4 w-4 shrink-0 items-center justify-center rounded text-[var(--muted)] hover:text-[var(--text)]"
              aria-expanded={expanded}
              aria-label={expanded ? `折叠 ${g.name} 的子分组` : `展开 ${g.name} 的子分组`}
              title={expanded ? "折叠子分组" : "展开子分组"}
              onClick={(e) => {
                e.stopPropagation();
                toggleExpanded(g.id);
              }}
            >
              <NavIcon name={expanded ? "chevron-down" : "chevron-right"} />
            </button>
          ) : (
            <span className="w-4 shrink-0" aria-hidden="true" />
          )}
          <NavIcon
            name={expanded || active ? "folder-open" : "folder"}
            className={active ? "text-[var(--accent)]" : undefined}
          />
          <span className="min-w-0 flex-1 truncate">{g.name}</span>
          <span className="relative ml-auto flex h-5 w-9 shrink-0 items-center justify-end">
            <span className="text-[11px] tabular-nums text-[var(--muted)] transition-opacity group-hover/row:opacity-0 group-focus-within/row:opacity-0">
              {g.videoCount}
            </span>
            <button
              type="button"
              className="pointer-events-none absolute inset-y-0 right-0 flex w-5 items-center justify-center rounded text-[var(--muted)] opacity-0 transition hover:bg-[var(--border)] hover:text-[var(--text)] group-hover/row:pointer-events-auto group-hover/row:opacity-100 group-focus-within/row:pointer-events-auto group-focus-within/row:opacity-100 focus-visible:pointer-events-auto focus-visible:opacity-100"
              aria-label={`分组操作：${g.name}`}
              title="分组操作"
              onClick={(e) => {
                e.stopPropagation();
                onGroupContextMenu(e, g);
              }}
            >
              <NavIcon name="more-horizontal" className="h-3.5 w-3.5" />
            </button>
          </span>
        </div>
        {expanded && children.length > 0 && (
          <div className="ml-3 border-l border-[var(--border)] pl-1">
            {children.map((c) => renderGroup(c, groups.indexOf(c)))}
          </div>
        )}
      </div>
    );
  };

  return (
    <SidebarRoot data-tour="sidebar">
      <SidebarNav className="flex min-h-0 flex-1 flex-col">
        <SidebarSection className="mt-0 shrink-0">浏览</SidebarSection>
        <NavRow
          icon="star"
          label="全部收藏"
          active={filter.kind === "all"}
          onClick={() => onSelect({ kind: "all" })}
          onDragOver={(e) => {
            if (dragIndex == null) return;
            e.preventDefault();
            setOverState(null);
          }}
          onDrop={handleDropDetach}
          title="全部收藏（把分组拖到这里可设为顶层）"
        />
        <NavRow
          icon="history"
          label="最近播放"
          active={filter.kind === "recent"}
          onClick={() => onSelect({ kind: "recent" })}
          title="按播放时间分组：1 天内 / 3 天内 / 7 天内 / 14 天内 / 更早"
        />
        <NavRow
          icon="users"
          label="演员"
          active={filter.kind === "actors"}
          onClick={() => onSelect({ kind: "actors" })}
          title="按演员浏览出演过的影片"
        />

        <SidebarSection className="mt-4 shrink-0">
          <span>分组</span>
          <Button variant="ghost" size="sm" className="h-5 w-5 rounded p-0" title="新建分组" onClick={onCreate}>
            <NavIcon name="plus" className="h-3.5 w-3.5" />
          </Button>
        </SidebarSection>

        {/* The tree is the only shrinkable child: short lists sit right above
            the tools, long lists scroll instead of pushing them away. */}
        <div className="min-h-0 overflow-y-auto pr-0.5">
          {groups.length === 0 ? (
            <div className="px-2 py-2 text-xs text-[var(--muted)]">暂无分组</div>
          ) : (
            topLevel.map((g) => renderGroup(g, groups.indexOf(g)))
          )}
        </div>

        <div className="mt-3 shrink-0 border-t border-[var(--border)] pt-2">
          <SidebarSection className="mt-0">工具</SidebarSection>
          <NavRow
            icon="list-checks"
            label="刮削任务"
            active={filter.kind === "scrapeTasks"}
            dataTour="scrape-tasks"
            onClick={() => onSelect({ kind: "scrapeTasks", status: "queued" })}
            title="查看刮削任务"
          />
          <NavRow
            icon="duplicate"
            label="查重"
            active={filter.kind === "duplicates"}
            onClick={() => onSelect({ kind: "duplicates" })}
            title="同一番号的多个版本：保留一个，或全部保留"
          />
          <NavRow
            icon="archive"
            label="归档与回收站"
            active={filter.kind === "trash"}
            onClick={() => onSelect({ kind: "trash" })}
            title="归档的网盘收藏与已移出收藏的影片：可恢复或彻底删除"
          />
        </div>
      </SidebarNav>
    </SidebarRoot>
  );
}
