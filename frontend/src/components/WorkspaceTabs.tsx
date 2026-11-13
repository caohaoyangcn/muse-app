import { X } from "lucide-react";
import type { MouseEvent as ReactMouseEvent } from "react";
import type { WorkspaceTab } from "../types";
import { cn } from "../lib/utils";
import { NavIcon } from "../ui/NavIcon";

/** One rendered tab in the strip: its identity plus the label shown. */
export interface WorkspaceTabItem {
  tab: WorkspaceTab;
  label: string;
}

/** A collapsible automatic group of video tabs that share an actor. */
export interface WorkspaceTabGroup {
  /** Canonical actor name, also the collapse-state key. */
  id: string;
  label: string;
  tabs: WorkspaceTabItem[];
}

/** Chrome's tab-group palette, tuned for the app's dark surface. */
interface GroupColor {
  solid: string;
  tint: string;
  border: string;
  text: string;
  badge: string;
}

const GROUP_COLORS: GroupColor[] = [
  {
    solid: "#1a73e8",
    tint: "rgba(26,115,232,0.16)",
    border: "rgba(90,155,255,0.55)",
    text: "#ffffff",
    badge: "rgba(255,255,255,0.22)",
  },
  {
    solid: "#d93025",
    tint: "rgba(217,48,37,0.16)",
    border: "rgba(255,110,100,0.55)",
    text: "#ffffff",
    badge: "rgba(255,255,255,0.22)",
  },
  {
    solid: "#f9ab00",
    tint: "rgba(249,171,0,0.15)",
    border: "rgba(249,171,0,0.55)",
    text: "#1b1b1f",
    badge: "rgba(0,0,0,0.16)",
  },
  {
    solid: "#1e8e3e",
    tint: "rgba(30,142,62,0.16)",
    border: "rgba(80,200,120,0.55)",
    text: "#ffffff",
    badge: "rgba(255,255,255,0.22)",
  },
  {
    solid: "#d01884",
    tint: "rgba(208,24,132,0.16)",
    border: "rgba(255,100,190,0.55)",
    text: "#ffffff",
    badge: "rgba(255,255,255,0.22)",
  },
  {
    solid: "#9334e6",
    tint: "rgba(147,52,230,0.16)",
    border: "rgba(190,130,255,0.55)",
    text: "#ffffff",
    badge: "rgba(255,255,255,0.22)",
  },
  {
    solid: "#0e9fb5",
    tint: "rgba(14,159,181,0.16)",
    border: "rgba(60,220,240,0.55)",
    text: "#1b1b1f",
    badge: "rgba(0,0,0,0.16)",
  },
  {
    solid: "#d9650a",
    tint: "rgba(217,101,10,0.16)",
    border: "rgba(255,170,90,0.55)",
    text: "#1b1b1f",
    badge: "rgba(0,0,0,0.16)",
  },
  {
    solid: "#5f6368",
    tint: "rgba(95,99,104,0.18)",
    border: "rgba(160,165,175,0.5)",
    text: "#ffffff",
    badge: "rgba(255,255,255,0.22)",
  },
];

/**
 * Chrome assigns a colour when a group is created. These groups are automatic,
 * so the colour is derived from the canonical actor name instead: the same
 * actor always gets the same colour, and adding another group does not reshuffle
 * the existing ones.
 */
function groupColor(name: string): GroupColor {
  let hash = 0;
  for (let i = 0; i < name.length; i += 1) {
    hash = (hash * 31 + name.charCodeAt(i)) >>> 0;
  }
  return GROUP_COLORS[hash % GROUP_COLORS.length];
}

export default function WorkspaceTabs({
  groups,
  tabs,
  active,
  collapsedGroups,
  onToggleGroup,
  onSelect,
  onClose,
  onContextMenu,
}: {
  groups: WorkspaceTabGroup[];
  /** Loose tabs: video tabs without a group, then actor film-list tabs. */
  tabs: WorkspaceTabItem[];
  active: WorkspaceTab;
  collapsedGroups: ReadonlySet<string>;
  onToggleGroup: (id: string) => void;
  onSelect: (tab: WorkspaceTab) => void;
  onClose: (tab: WorkspaceTab) => void;
  onContextMenu: (e: ReactMouseEvent, tab: WorkspaceTab) => void;
}) {
  const renderTab = ({ tab, label }: WorkspaceTabItem, color?: GroupColor) => {
    const selected = active === tab;
    return (
      <div
        key={String(tab)}
        className={cn(
          "group flex h-10 shrink-0 items-center border-b-2 transition-colors",
          selected && color == null
            ? "border-[var(--accent)]"
            : color == null
              ? "border-transparent"
              : undefined,
        )}
        // Chrome draws the group's colour along the bottom of every member
        // tab, including the active one; ungrouped tabs keep the app accent.
        style={
          color
            ? { borderBottomColor: selected ? color.solid : color.border }
            : undefined
        }
        onContextMenu={(e) => onContextMenu(e, tab)}
      >
        <button
          type="button"
          className={cn(
            "h-full max-w-[260px] truncate px-3 text-sm",
            selected ? "text-[var(--text)]" : "text-[var(--muted)] hover:text-[var(--text)]",
          )}
          aria-current={selected ? "page" : undefined}
          onClick={() => onSelect(tab)}
          title={label}
        >
          {label}
        </button>
        <button
          type="button"
          className="mr-1 inline-flex h-6 w-6 items-center justify-center rounded text-[var(--muted)] opacity-60 transition hover:bg-[var(--border)] hover:text-[var(--text)] group-hover:opacity-100 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
          aria-label={`关闭 ${label}`}
          title="关闭标签"
          onClick={(e) => {
            e.stopPropagation();
            onClose(tab);
          }}
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>
    );
  };

  return (
    <nav
      className="flex min-h-10 shrink-0 items-end gap-1 overflow-x-auto border-b border-[var(--border)] bg-[var(--panel)] px-3"
      aria-label="工作区标签"
    >
      <button
        type="button"
        className={cn(
          "h-10 shrink-0 border-b-2 px-4 text-sm transition-colors",
          active === "library"
            ? "border-[var(--accent)] text-[var(--text)]"
            : "border-transparent text-[var(--muted)] hover:text-[var(--text)]",
        )}
        aria-current={active === "library" ? "page" : undefined}
        onClick={() => onSelect("library")}
        onContextMenu={(e) => onContextMenu(e, "library")}
      >
        影片库
      </button>
      {groups.map((group) => {
        const collapsed = collapsedGroups.has(group.id);
        const containsActive = group.tabs.some((item) => item.tab === active);
        const color = groupColor(group.label);
        return (
          <div
            key={`group:${group.id}`}
            className={cn(
              "flex h-10 shrink-0 items-end gap-0 rounded-t-lg border border-b-0 transition-colors",
              !collapsed && "pl-1 pr-0.5",
            )}
            style={{
              backgroundColor: collapsed ? "transparent" : color.tint,
              borderColor: collapsed
                ? "transparent"
                : containsActive
                  ? color.solid
                  : color.border,
            }}
            onContextMenu={(e) => e.preventDefault()}
          >
            <button
              type="button"
              className="mb-1 flex h-8 max-w-[220px] items-center gap-1.5 rounded-md px-2 text-xs font-medium shadow-[0_1px_2px_rgba(0,0,0,0.28)] transition-[filter] hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--text)]"
              style={{ backgroundColor: color.solid, color: color.text }}
              aria-expanded={!collapsed}
              aria-label={`${group.label} 标签组，${group.tabs.length} 个标签页，${
                collapsed ? "已折叠" : "已展开"
              }`}
              title={`${group.label} · 演员标签组（${group.tabs.length}）· 点击${
                collapsed ? "展开" : "折叠"
              }`}
              onClick={() => onToggleGroup(group.id)}
            >
              <span className="max-w-[140px] truncate">{group.label}</span>
              <span
                className="shrink-0 rounded-full px-1.5 text-[10px] leading-[16px] tabular-nums"
                style={{ backgroundColor: color.badge }}
              >
                {group.tabs.length}
              </span>
              <NavIcon
                name={collapsed ? "chevron-right" : "chevron-down"}
                className="h-3.5 w-3.5 shrink-0 opacity-80"
              />
            </button>
            {!collapsed && group.tabs.map((item) => renderTab(item, color))}
          </div>
        );
      })}
      {tabs.map((item) => renderTab(item))}
    </nav>
  );
}
