import type { MouseEvent as ReactMouseEvent } from "react";
import type { ActorTab, WorkspaceTab } from "../types";
import { Tab, TabList } from "../ui/astryx";

/** One rendered tab in the strip: its identity, label and optional actor group. */
export interface WorkspaceTabItem {
  tab: WorkspaceTab;
  label: string;
  /** Actor group this tab belongs to; only used to keep the group's colour. */
  groupLabel?: string;
}

/** Chrome's tab-group palette, tuned for the app's dark surface. */
interface GroupColor {
  solid: string;
  border: string;
}

const GROUP_COLORS: GroupColor[] = [
  { solid: "#1a73e8", border: "rgba(90,155,255,0.55)" },
  { solid: "#d93025", border: "rgba(255,110,100,0.55)" },
  { solid: "#f9ab00", border: "rgba(249,171,0,0.55)" },
  { solid: "#1e8e3e", border: "rgba(80,200,120,0.55)" },
  { solid: "#d01884", border: "rgba(255,100,190,0.55)" },
  { solid: "#9334e6", border: "rgba(190,130,255,0.55)" },
  { solid: "#0e9fb5", border: "rgba(60,220,240,0.55)" },
  { solid: "#d9650a", border: "rgba(255,170,90,0.55)" },
  { solid: "#5f6368", border: "rgba(160,165,175,0.5)" },
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

/** `TabList` values are strings; actor tabs already carry a unique prefix. */
function tabFromValue(value: string): WorkspaceTab {
  if (value === "library") return "library";
  if (value.startsWith("actor:")) return value as ActorTab;
  return Number(value);
}

/**
 * Astryx's Tab renders its label unclipped (the label doubles as the width
 * sizer), so a long video name would widen the whole strip. Truncate the
 * visible text here and keep the full name on `aria-label`.
 */
function visibleLabel(label: string): string {
  const MAX = 18;
  return label.length > MAX ? `${label.slice(0, MAX - 1)}…` : label;
}

/**
 * The workspace tab strip, on Astryx `TabList`.
 *
 * One flat strip: actor groups keep their colour as a small leading dot, but
 * no longer have a collapsible container. Tabs close through the right-click
 * menu (close / close others / close right) and a middle click; there is no
 * per-tab close button because `Tab` is one button and its `endContent` may
 * not contain interactive elements.
 */
export default function WorkspaceTabs({
  tabs,
  active,
  onSelect,
  onClose,
  onContextMenu,
}: {
  tabs: WorkspaceTabItem[];
  active: WorkspaceTab;
  onSelect: (tab: WorkspaceTab) => void;
  onClose: (tab: WorkspaceTab) => void;
  onContextMenu: (e: ReactMouseEvent, tab: WorkspaceTab) => void;
}) {
  return (
    <TabList
      aria-label="工作区标签"
      className="min-h-10 shrink-0 bg-[var(--panel)] px-3"
      hasDivider
      onChange={(value) => onSelect(tabFromValue(value))}
      size="md"
      value={String(active)}
    >
      {tabs.map(({ tab, label, groupLabel }) => {
        const color = groupLabel == null ? null : groupColor(groupLabel);
        return (
          <Tab
            aria-label={label}
            icon={
              color ? (
                <span
                  aria-hidden="true"
                  className="size-2 shrink-0 rounded-full border"
                  style={{ borderColor: color.border }}
                />
              ) : undefined
            }
            key={String(tab)}
            label={visibleLabel(label)}
            onAuxClick={(event) => {
              if (event.button !== 1) return;
              event.preventDefault();
              onClose(tab);
            }}
            onContextMenu={(event) => onContextMenu(event, tab)}
            selectedIcon={
              color ? (
                <span
                  aria-hidden="true"
                  className="size-2 shrink-0 rounded-full"
                  style={{ backgroundColor: color.solid }}
                />
              ) : undefined
            }
            value={String(tab)}
          />
        );
      })}
    </TabList>
  );
}
