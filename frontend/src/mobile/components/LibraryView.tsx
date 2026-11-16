import { useCallback, useEffect, useMemo, useState } from "react";

import type { Group, SortDir, SortKey, Video, ViewMode } from "../../types";
import { collectDiskOptions } from "../../utils/diskFilter";
import { compareLastOpened } from "../../utils/sortVideos";
import { client, videosInGroup } from "../../services/web";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import { cn } from "../../lib/utils";
import {
  SegmentedControl,
  SegmentedControlItem,
  Toolbar,
} from "../../ui/astryx";
import { Button } from "../../ui/button";
import { Icon } from "../../ui/icon";
import {
  buildFilterChips,
  chipKindLabel,
  collectActorOptions,
  filterCount,
  filterVideos,
  NO_FILTERS,
  removeFilterChip,
  withSelectedActors,
  withSelectedDisks,
  type FilterChip,
  type LibraryFilters,
} from "../libraryFilters";
import {
  readSortDir,
  readSortKey,
  readViewMode,
  writeSortDir,
  writeSortKey,
  writeViewMode,
} from "../prefs";
import { FilterSheet } from "./FilterSheet";
import { VideoGrid, VideoList } from "./VideoGrid";
import { EmptyState, Spinner } from "../../ui/astryx";

const collator = new Intl.Collator("zh-CN", { numeric: true, sensitivity: "base" });

// Stable identities for the empty answers.
const NO_VIDEOS: Video[] = [];
const NO_GROUPS: Group[] = [];

const SORT_LABELS: Record<SortKey, string> = {
  createdAt: "添加时间",
  lastOpenedAt: "最近播放",
  fileSize: "文件大小",
  fileName: "文件名",
  likeCount: "点赞数",
};

/**
 * The library tab: a cinematic poster grid, one compact control line, the
 * applied filter chips and the filter sheet.
 *
 * Filter state lives in the shell (so it survives opening a video and coming
 * back); this view is responsible for turning it into rows: selected groups are
 * fetched as a union, selected actors and disks narrow that list locally, and
 * the keyword remains a whole-library search intersected with the group
 * selection.
 */
export function LibraryView({
  keyword,
  filters,
  onFiltersChange,
  filterOpen,
  onFilterOpenChange,
  onOpenVideo,
  onOpenMenu,
}: {
  keyword: string;
  filters: LibraryFilters;
  onFiltersChange: (next: LibraryFilters) => void;
  filterOpen: boolean;
  onFilterOpenChange: (open: boolean) => void;
  onOpenVideo: (v: Video) => void;
  onOpenMenu: (v: Video) => void;
}) {
  const [sortKey, setSortKey] = useState<SortKey>(readSortKey);
  const [sortDir, setSortDir] = useState<SortDir>(readSortDir);
  const [viewMode, setViewMode] = useState<ViewMode>(readViewMode);

  const groupIds = filters.groupIds;
  const groupKey = groupIds.join(",");

  // Search always spans the whole library (matching the desktop app); the group
  // selection then intersects it by id. With no keyword the selected groups are
  // the source list themselves, so no redundant full-library read is needed.
  const fetchLibrary = useCallback(async () => {
    const basePromise: Promise<Video[] | null> = keyword
      ? client.searchVideos(keyword)
      : groupIds.length === 0
        ? client.listVideos()
        : Promise.resolve(null);
    const [groupList, base, groupLists] = await Promise.all([
      client.listGroups(),
      basePromise,
      Promise.all(groupIds.map((id) => videosInGroup(id))),
    ]);

    let videos: Video[];
    if (groupIds.length > 0) {
      const union = new Map<number, Video>();
      for (const list of groupLists) {
        for (const video of list ?? []) union.set(video.id, video);
      }
      videos = keyword ? (base ?? []).filter((video) => union.has(video.id)) : [...union.values()];
    } else {
      videos = base ?? NO_VIDEOS;
    }
    return { videos, groups: groupList ?? NO_GROUPS };
  }, [keyword, groupIds]);

  const { data, loading, error, reload } = useLiveQuery(
    fetchLibrary,
    `library:${groupKey}:${keyword}`,
    { keepPreviousData: true },
  );
  const baseVideos = data?.videos ?? NO_VIDEOS;
  const groups = data?.groups ?? NO_GROUPS;

  // A group deleted on the desktop must not strand the phone behind a 404: drop
  // ids that no longer exist once the group list has loaded.
  useEffect(() => {
    if (!data) return;
    const known = new Set(groups.map((group) => group.id));
    const kept = filters.groupIds.filter((id) => known.has(id));
    if (kept.length !== filters.groupIds.length) {
      onFiltersChange({ ...filters, groupIds: kept });
    }
  }, [data, groups, filters, onFiltersChange]);

  const sorted = useMemo(() => {
    const copy = [...baseVideos];
    const dir = sortDir === "asc" ? 1 : -1;
    copy.sort((a, b) => {
      if (sortKey === "lastOpenedAt") return compareLastOpened(a, b, sortDir);
      let cmp: number;
      switch (sortKey) {
        case "fileSize":
          cmp = a.fileSize - b.fileSize;
          break;
        case "fileName":
          cmp = collator.compare(a.fileName, b.fileName);
          break;
        case "likeCount":
          // Zero-like items belong together at the bottom in either direction.
          cmp = a.likeCount - b.likeCount;
          break;
        default:
          cmp = a.createdAt - b.createdAt;
      }
      if (cmp === 0) cmp = a.id - b.id;
      return cmp * dir;
    });
    return copy;
  }, [baseVideos, sortKey, sortDir]);

  const actorOptions = useMemo(
    () => withSelectedActors(collectActorOptions(baseVideos), filters.actors),
    [baseVideos, filters.actors],
  );
  const diskOptions = useMemo(
    () => withSelectedDisks(collectDiskOptions(baseVideos), filters.disks),
    [baseVideos, filters.disks],
  );
  const chips = useMemo(
    () => buildFilterChips(filters, groups, diskOptions),
    [filters, groups, diskOptions],
  );
  const shown = useMemo(() => filterVideos(sorted, filters), [sorted, filters]);
  const count = filterCount(filters);

  const setSort = useCallback((key: SortKey) => {
    setSortKey(key);
    writeSortKey(key);
    // Changing the field resets to the field's natural "most relevant" first.
    setSortDir("desc");
    writeSortDir("desc");
  }, []);
  const setDir = useCallback((dir: SortDir) => {
    setSortDir(dir);
    writeSortDir(dir);
  }, []);
  const setView = useCallback((next: ViewMode) => {
    setViewMode(next);
    writeViewMode(next);
  }, []);

  const resetFilters = useCallback(() => onFiltersChange(NO_FILTERS), [onFiltersChange]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* One line at rest. The sheet holds the conditions; this bar only names
          the current ones and offers the two actions used mid-scroll. */}
      <div className="shrink-0 px-3 pb-2 pt-1.5">
        <Toolbar
          className="px-0 py-0"
          endContent={
            <div className="flex items-center gap-2">
              <SegmentedControl
                label="视图切换"
                layout="hug"
                onChange={(value) => setView(value as ViewMode)}
                size="sm"
                value={viewMode}
              >
                <SegmentedControlItem
                  icon={<Icon className="size-3.5" name="grid" />}
                  isLabelHidden
                  label="网格视图"
                  value="grid"
                />
                <SegmentedControlItem
                  icon={<Icon className="size-3.5" name="list" />}
                  isLabelHidden
                  label="列表视图"
                  value="list"
                />
              </SegmentedControl>
              <span className="shrink-0 text-[11px] leading-none text-[var(--muted)] tabular-nums">
                {shown.length}
                {shown.length !== sorted.length ? ` / ${sorted.length}` : ""} 部
              </span>
            </div>
          }
          label="片库工具栏"
          startContent={
            <div className="flex items-center gap-1.5">
              <Button
                aria-label={count === 0 ? "筛选" : `筛选，已选 ${count} 个条件`}
                className={cn(
                  "h-9 shrink-0 gap-1.5 rounded-full px-3 text-xs",
                  count > 0
                    ? "border-[var(--accent)] bg-[rgba(124,92,255,0.16)] text-[var(--text)]"
                    : "border-[var(--border)] bg-[var(--panel-2)] text-[var(--muted)]",
                )}
                onClick={() => onFilterOpenChange(true)}
                size="sm"
                variant="ghost"
              >
                <Icon className="size-3.5" name="filter" />
                筛选
                {count > 0 ? (
                  <span className="inline-flex h-4 min-w-4 items-center justify-center rounded-full bg-[var(--accent)] px-1 text-[10px] font-semibold leading-none text-white tabular-nums">
                    {count}
                  </span>
                ) : null}
              </Button>

              <Button
                aria-label={`排序：${SORT_LABELS[sortKey]} ${sortDir === "asc" ? "升序" : "降序"}，点击调整`}
                className="h-9 min-w-0 shrink gap-1 rounded-full border-[var(--border)] bg-[var(--panel-2)] px-3 text-xs text-[var(--text)]"
                onClick={() => onFilterOpenChange(true)}
                size="sm"
                variant="ghost"
              >
                <span className="truncate">{SORT_LABELS[sortKey]}</span>
                <Icon
                  className={cn("size-3 shrink-0", sortDir === "asc" && "rotate-180")}
                  name="chevron-down"
                />
              </Button>
            </div>
          }
          variant="transparent"
        />

        {chips.length > 0 ? (
          <div className="mt-2 flex items-center gap-2">
            <div className="filter-scroll flex min-w-0 flex-1 gap-1.5 overflow-x-auto">
              {chips.map((chip) => (
                <AppliedChip
                  chip={chip}
                  key={chip.id}
                  onRemove={() => onFiltersChange(removeFilterChip(filters, chip))}
                />
              ))}
            </div>
            <Button
              className="h-7 shrink-0 rounded-full px-2 text-[11px] text-[var(--muted)]"
              onClick={resetFilters}
              size="sm"
              variant="ghost"
            >
              清除
            </Button>
          </div>
        ) : null}
      </div>

      <div className="scroll-area min-h-0 flex-1 px-3 pb-4 pt-2" data-scroll-key="main">
        {loading ? (
          <div className="flex items-center justify-center py-20"><Spinner size="lg" /></div>
        ) : error ? (
          <EmptyState actions={<Button onClick={reload} variant="primary">重试</Button>} description={error} title="加载失败" />
        ) : sorted.length === 0 ? (
          <EmptyState
            description={
              keyword
                ? count > 0
                  ? "当前筛选内没有匹配的收藏，换个关键词或减少筛选"
                  : "换个关键词试试"
                : count > 0
                  ? "所选条件暂无收藏，试试减少筛选"
                  : "在桌面端添加收藏后会出现在这里"
            }
            title={
              keyword
                ? "没有匹配的视频"
                : count > 0
                  ? "当前筛选没有视频"
                  : "收藏库为空"
            }
          />
        ) : shown.length === 0 ? (
          <EmptyState description="试试减少演员或硬盘筛选" title="没有符合条件的视频" />
        ) : viewMode === "grid" ? (
          <VideoGrid onMenu={onOpenMenu} onOpen={onOpenVideo} videos={shown} />
        ) : (
          <VideoList onMenu={onOpenMenu} onOpen={onOpenVideo} videos={shown} />
        )}
      </div>

      <FilterSheet
        actors={actorOptions}
        disks={diskOptions}
        filters={filters}
        groups={groups}
        onChange={onFiltersChange}
        onClose={() => onFilterOpenChange(false)}
        onReset={resetFilters}
        onSortDir={setDir}
        onSortKey={setSort}
        open={filterOpen}
        sortDir={sortDir}
        sortKey={sortKey}
      />
    </div>
  );
}

function AppliedChip({ chip, onRemove }: { chip: FilterChip; onRemove: () => void }) {
  return (
    <span className="inline-flex h-7 shrink-0 items-center gap-1 rounded-full border border-[rgba(124,92,255,0.45)] bg-[rgba(124,92,255,0.14)] pl-2.5 pr-1">
      <span className="text-[10px] leading-none text-[var(--muted)]">{chipKindLabel(chip.kind)}</span>
      <span className="max-w-[9rem] truncate text-[11px] leading-none text-[var(--text)]">
        {chip.label}
      </span>
      <Button
        aria-label={`移除筛选：${chipKindLabel(chip.kind)} ${chip.label}`}
        className="size-5 min-w-0 shrink-0 rounded-full p-0 text-[var(--muted)]"
        onClick={onRemove}
        size="sm"
        variant="ghost"
      >
        <Icon className="size-3" name="x" />
      </Button>
    </span>
  );
}
