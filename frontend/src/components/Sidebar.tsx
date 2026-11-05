import { useEffect, useRef, useState } from "react";
import type { Group, ViewFilter } from "../types";
import { MIME_VIDEO_DRAG, MIME_VIDEO_DRAG_MULTI } from "../types";
import { cn } from "../lib/utils";
import { Button } from "./ui/button";
import { Sidebar as SidebarRoot, SidebarNav, SidebarSection } from "./ui/sidebar";
import { ChevronDown, ChevronRight, Plus } from "lucide-react";

interface SidebarProps {
  groups: Group[];
  filter: ViewFilter;
  onCreate: () => void;
  onSelect: (filter: ViewFilter) => void;
  onRename: (group: Group) => void;
  onDelete: (group: Group) => void;
  onReorder: (order: Group[]) => void;
  onSetParent: (id: number, parentId: number | null) => Promise<void>;
  onDropVideo: (group: Group, videoIds: number[]) => void;
  onGroupContextMenu: (e: React.MouseEvent, group: Group) => void;
}

/**
 * Left navigation: "All Favorites" and the reorderable group list (tree).
 * Ordering/nesting use native HTML5 drag & drop:
 *  - dragging over a group's top/bottom edge inserts above/below (reorder);
 *  - dragging over a group's middle nests the dragged group as its child;
 *  - dragging onto "All Favorites" detaches a group back to the top level.
 * Each group is also a drop target for video cards (add to group). A drag may
 * carry a single video (MIME_VIDEO_DRAG) or a multi-selected batch
 * (MIME_VIDEO_DRAG_MULTI); both are decoded into a video-id list.
 */
export default function Sidebar({
  groups,
  filter,
  onCreate,
  onSelect,
  onRename,
  onDelete,
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

  const renderGroup = (g: Group, depth: number, index: number) => {
    const children = groups.filter((c) => c.parentId === g.id);
    const hasChildren = children.length > 0;
    // Children are hidden unless explicitly expanded. While a group drag hovers
    // with the "nest" intent, force it open so the nesting being created is
    // visible.
    const expanded = expandedIds.has(g.id) || (overState?.zone === "nest" && overState.index === index);
    const active = filter.kind === "group" && filter.id === g.id;
    const over = overState?.index === index ? overState.zone : null;
    const pad = depth > 0 ? { paddingLeft: `${12 + depth * 14}px` } : undefined;
    return (
      <div key={g.id}>
        <div
          className={cn(
            "group flex cursor-grab items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-[var(--muted)] hover:bg-[var(--panel-2)] hover:text-[var(--text)] active:cursor-grabbing",
            active && "bg-[var(--panel-2)] text-[var(--text)]",
            over && "bg-[rgba(124,92,255,0.08)] shadow-[inset_0_0_0_2px_var(--accent)]",
            over === "above" && "shadow-[inset_0_3px_0_var(--accent)]",
            over === "below" && "shadow-[inset_0_-3px_0_var(--accent)]",
            over === "nest" && "border border-dashed border-[var(--accent)] bg-[rgba(124,92,255,0.18)]",
            videoOverId === g.id && "border border-dashed border-[var(--accent)] bg-[rgba(124,92,255,0.16)] shadow-[inset_0_0_0_2px_var(--accent)]",
          )}
          style={pad}
          draggable
          onClick={() => onSelect({ kind: "group", id: g.id })}
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
              {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
            </button>
          ) : (
            <span className="w-4 shrink-0" aria-hidden="true" />
          )}
          <span className="text-sm">{depth > 0 ? "└" : "🗂"}</span>
          <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{g.name}</span>
          <span className="ml-auto rounded-[10px] bg-[var(--border)] px-2 py-px text-xs text-[var(--muted)]">{g.videoCount}</span>
          <span
            className="hidden shrink-0 gap-0.5 group-hover:flex"
            onClick={(e) => e.stopPropagation()}
            role="presentation"
          >
            <Button variant="ghost" size="sm" className="h-6 rounded-md px-1.5 py-0.5 text-xs" title="重命名分组" onClick={() => onRename(g)}>
              ✎
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 rounded-md px-1.5 py-0.5 text-xs hover:bg-[rgba(229,83,75,0.2)] hover:text-[var(--danger)]"
              title="删除分组"
              onClick={() => onDelete(g)}
            >
              🗑
            </Button>
          </span>
        </div>
        {expanded && children.map((c) => renderGroup(c, depth + 1, groups.indexOf(c)))}
      </div>
    );
  };

  return (
    <SidebarRoot data-tour="sidebar">
      <SidebarNav>
        <div
          className={cn(
            "flex cursor-pointer items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-[var(--muted)] hover:bg-[var(--panel-2)] hover:text-[var(--text)]",
            filter.kind === "all" && "bg-[var(--panel-2)] text-[var(--text)]",
            dragIndex != null && "bg-[rgba(124,92,255,0.08)] shadow-[inset_0_0_0_2px_var(--accent)]",
          )}
          onClick={() => onSelect({ kind: "all" })}
          onDragOver={(e) => {
            if (dragIndex == null) return;
            e.preventDefault();
            setOverState(null);
          }}
          onDrop={handleDropDetach}
          title="全部收藏（把分组拖到这里可设为顶层）"
        >
          <span>⭐</span>
          <span>全部收藏</span>
        </div>

        <SidebarSection>
          <span>分组</span>
          <Button variant="ghost" size="sm" className="h-6 w-6 rounded-md p-0" title="新建分组" onClick={onCreate}>
            <Plus className="h-4 w-4" />
          </Button>
        </SidebarSection>

        {groups.length === 0 ? (
          <div className="px-3 py-1.5 text-xs text-[var(--muted)]">暂无分组</div>
        ) : (
          topLevel.map((g) => renderGroup(g, 0, groups.indexOf(g)))
        )}

        <SidebarSection>
          <span>刮削</span>
        </SidebarSection>
        <div
          className={cn(
            "flex cursor-pointer items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-[var(--muted)] hover:bg-[var(--panel-2)] hover:text-[var(--text)]",
            (filter.kind === "actors") && "bg-[var(--panel-2)] text-[var(--text)]",
          )}
          onClick={() => onSelect({ kind: "actors" })}
          title="按演员浏览出演过的影片"
        >
          <span>🎭</span>
          <span>演员</span>
        </div>
        <div
          data-tour="scrape-tasks"
          className={cn(
            "flex cursor-pointer items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-[var(--muted)] hover:bg-[var(--panel-2)] hover:text-[var(--text)]",
            filter.kind === "scrapeTasks" && "bg-[var(--panel-2)] text-[var(--text)]",
          )}
          onClick={() => onSelect({ kind: "scrapeTasks", status: "queued" })}
          title="查看刮削任务"
        >
          <span>↻</span>
          <span>刮削任务</span>
        </div>
        <div
          className={cn(
            "flex cursor-pointer items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-[var(--muted)] hover:bg-[var(--panel-2)] hover:text-[var(--text)]",
            filter.kind === "duplicates" && "bg-[var(--panel-2)] text-[var(--text)]",
          )}
          onClick={() => onSelect({ kind: "duplicates" })}
          title="同一番号的多个版本：保留一个，或全部保留"
        >
          <span>⧉</span>
          <span>查重</span>
        </div>
      </SidebarNav>
    </SidebarRoot>
  );
}
