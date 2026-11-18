import type { ViewFilter } from "../types";
import { cn } from "../lib/utils";
import { Button } from "../ui/button";
import { Sidebar as SidebarRoot, SidebarNav, SidebarSection } from "../ui/sidebar";
import { Icon, type MuseIconName } from "../ui/icon";
import { IconButton } from "../ui/astryx";
import type { LibraryFilters, ValueOption } from "../utils/libraryFilter";
import { withSelectedValues } from "../utils/libraryFilter";
import { withoutHiddenTags } from "../utils/hiddenTags";

interface SidebarProps {
  filter: ViewFilter;
  /** Tag choices derived from the rows the library currently holds. */
  tagOptions: ValueOption[];
  /** Tags removed from this shortcut; the full filter dialog still offers them. */
  hiddenTags: string[];
  /** The tag selection, shown as active rows and pinned past the top list. */
  filters: LibraryFilters;
  onSelect: (filter: ViewFilter) => void;
  onToggleTag: (tag: string) => void;
  onHideTag: (tag: string) => void;
  onOpenFilters: () => void;
}

const ROW =
  "relative flex h-8 w-full shrink-0 items-center justify-start gap-2 rounded-[var(--radius)] px-2 text-[13px] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]";

// The sidebar is a shortcut, not the full filter surface: a library can carry
// hundreds of scraped tags, and rendering them all would bury the tools. The
// dialog behind "更多标签…" owns search and paging.
const TAG_LIMIT = 12;

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
}: {
  icon: MuseIconName;
  label: string;
  active: boolean;
  onClick: () => void;
  title: string;
  dataTour?: string;
}) {
  return (
    <Button
      aria-current={active ? "page" : undefined}
      className={rowClass(active)}
      data-tour={dataTour}
      icon={<Icon name={icon} className={active ? "text-[var(--accent)]" : undefined} />}
      onClick={onClick}
      title={title}
      variant="ghost"
    >
      {active && <ActiveBar />}
      <span className="min-w-0 flex-1 truncate text-left">{label}</span>
    </Button>
  );
}

/**
 * Left navigation in three zones: 浏览 (smart views), 标签 (the top scraped tags
 * as quick filters) and 工具 (maintenance entries). Tags replaced the old
 * user-defined group tree: they cannot be created, renamed, nested or
 * reordered, and the sidebar only offers the most-used ones — the filter
 * dialog owns the complete list.
 */
export default function Sidebar({
  filter,
  tagOptions,
  hiddenTags,
  filters,
  onSelect,
  onToggleTag,
  onHideTag,
  onOpenFilters,
}: SidebarProps) {
  const visibleOptions = withoutHiddenTags(tagOptions, hiddenTags);
  // A selected tag stays pinned in the shortcut even when it sits outside the
  // top 12 — but a tag the user explicitly hid is the one exception: it must
  // not come back just because it is selected. Its condition is still visible
  // in the chips above the list and in the filter dialog.
  const pinnedSelection = filters.tags.filter((tag) => !hiddenTags.includes(tag));
  const topTags = withSelectedValues(visibleOptions.slice(0, TAG_LIMIT), pinnedSelection);
  const hasMoreTags = visibleOptions.length > TAG_LIMIT;
  const selectedTags = new Set(filters.tags);

  return (
    <SidebarRoot data-tour="sidebar">
      <SidebarNav className="flex min-h-0 flex-1 flex-col">
        <SidebarSection className="mt-0 shrink-0">浏览</SidebarSection>
        <NavRow
          icon="star"
          label="全部收藏"
          active={filter.kind === "all"}
          onClick={() => onSelect({ kind: "all" })}
          title="全部收藏"
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
          <span>标签</span>
          <IconButton
            className="h-5 w-5 rounded p-0"
            icon={<Icon name="filter" className="h-3.5 w-3.5" />}
            label="筛选标签、演员与硬盘"
            onClick={onOpenFilters}
            size="sm"
            tooltip="全部标签与筛选"
            variant="ghost"
          />
        </SidebarSection>

        {/* The tag list is the only shrinkable child: short lists sit right
            above the tools, long lists scroll instead of pushing them away. */}
        <div className="min-h-0 overflow-y-auto pr-0.5">
          {topTags.length === 0 ? (
            <div className="px-2 py-2 text-xs text-[var(--muted)]">
              {hiddenTags.length > 0 ? "快捷标签已全部隐藏" : "暂无标签——刮削后自动出现"}
            </div>
          ) : (
            topTags.map((tag) => {
              const active = selectedTags.has(tag.name);
              return (
                // The row and its hide button are siblings, not nested: a
                // button inside a button is invalid HTML and breaks keyboard
                // activation. The wrapper only positions the close control.
                <div className="group relative" key={tag.name}>
                  <Button
                    aria-pressed={active}
                    className={rowClass(active, "pr-8")}
                    icon={<Icon name="tag" className={active ? "text-[var(--accent)]" : undefined} />}
                    onClick={() => onToggleTag(tag.name)}
                    title={`${tag.name} · ${tag.count} 个视频`}
                    variant="ghost"
                  >
                    {active && <ActiveBar />}
                    <span className="min-w-0 flex-1 truncate text-left">{tag.name}</span>
                    <span className="ml-auto shrink-0 text-[11px] tabular-nums text-[var(--muted)]">
                      {tag.count}
                    </span>
                  </Button>
                  <IconButton
                    className="absolute right-1 top-1/2 -translate-y-1/2 opacity-40 transition-opacity hover:opacity-100 group-hover:opacity-100 focus-visible:opacity-100"
                    icon={<Icon className="size-3" name="x" />}
                    label={`隐藏快捷标签：${tag.name}`}
                    onClick={() => onHideTag(tag.name)}
                    size="sm"
                    tooltip="从快捷入口隐藏；仍可在筛选面板中选择"
                    variant="ghost"
                  />
                </div>
              );
            })
          )}
          {hasMoreTags ? (
            <Button
              className={rowClass(false)}
              icon={<Icon name="filter" />}
              onClick={onOpenFilters}
              title="搜索并筛选全部标签、演员与硬盘"
              variant="ghost"
            >
              <span className="min-w-0 flex-1 truncate text-left">更多标签…</span>
            </Button>
          ) : null}
          {hiddenTags.length > 0 ? (
            <Button
              className={rowClass(false)}
              icon={<Icon name="filter" />}
              onClick={onOpenFilters}
              title="已隐藏的标签仍可在筛选面板中找到并恢复"
              variant="ghost"
            >
              <span className="min-w-0 flex-1 truncate text-left">
                已隐藏 {hiddenTags.length} 个标签
              </span>
            </Button>
          ) : null}
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
            icon="history"
            label="后台任务"
            active={filter.kind === "tasks"}
            onClick={() => onSelect({ kind: "tasks" })}
            title="查看正在运行的后台任务与进度"
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
