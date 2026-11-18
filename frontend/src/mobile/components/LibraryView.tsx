import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { SortDir, SortKey, Video, ViewMode } from "../../types";
import { collectDiskOptions } from "../../utils/diskFilter";
import { compareLastOpened } from "../../utils/sortVideos";
import { client } from "../../services/web";
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
  collectActorOptions,
  collectTagOptions,
  filterCount,
  filterVideos,
  NO_FILTERS,
  withSelectedActors,
  withSelectedDisks,
  withSelectedTags,
  type LibraryFilters,
} from "../../utils/libraryFilter";
import {
  readSortDir,
  readSortKey,
  readViewMode,
  writeSortDir,
  writeSortKey,
  writeViewMode,
} from "../prefs";
import { FilterSheet } from "./FilterSheet";
import { SortSheet } from "./SortSheet";
import { SORT_LABELS, defaultSortDir } from "../../utils/sortOptions";
import { VideoGrid, VideoList } from "./VideoGrid";
import type { VideoMenuActions } from "../videoMenu";
import { EmptyState, Spinner } from "../../ui/astryx";
import { FilterChips } from "../../views/library/FilterChips";

const collator = new Intl.Collator("zh-CN", { numeric: true, sensitivity: "base" });

// Stable identities for the empty answers.
const NO_VIDEOS: Video[] = [];

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
  actions,
  openMenuId,
  onMenuOpenChange,
}: {
  keyword: string;
  filters: LibraryFilters;
  onFiltersChange: (next: LibraryFilters) => void;
  filterOpen: boolean;
  onFilterOpenChange: (open: boolean) => void;
  onOpenVideo: (v: Video) => void;
  actions: VideoMenuActions;
  openMenuId: number | null;
  onMenuOpenChange: (video: Video, open: boolean) => void;
}) {
  const [sortKey, setSortKey] = useState<SortKey>(readSortKey);
  const [sortDir, setSortDir] = useState<SortDir>(readSortDir);
  const [sortOpen, setSortOpen] = useState(false);
  const [viewMode, setViewMode] = useState<ViewMode>(readViewMode);

  // Search always spans the whole library; tags, actors and disks narrow that
  // list locally. There is no longer a server-side group selection: the tag
  // dimensions live on the Video read model itself.
  const fetchLibrary = useCallback(async () => {
    return keyword ? client.searchVideos(keyword) : client.listVideos();
  }, [keyword]);

  const { data, loading, error, reload } = useLiveQuery(fetchLibrary, `library:${keyword}`, {
    keepPreviousData: true,
  });
  const baseVideos = data ?? NO_VIDEOS;

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

  // Identity of the list the paging depth belongs to. A different filter, sort
  // or keyword is a different list, and each one remembers how far it was
  // paged; the view mode is not part of it, so switching between the wall and
  // the compact list keeps the place the user had reached.
  const listKey = useMemo(
    () =>
      [
        sortKey,
        sortDir,
        keyword,
        filters.tags.join(","),
        filters.actors.join(","),
        filters.disks.join(","),
      ].join("|"),
    [sortKey, sortDir, keyword, filters.tags, filters.actors, filters.disks],
  );

  const tagOptions = useMemo(
    () => withSelectedTags(collectTagOptions(baseVideos), filters.tags),
    [baseVideos, filters.tags],
  );
  const actorOptions = useMemo(
    () => withSelectedActors(collectActorOptions(baseVideos), filters.actors),
    [baseVideos, filters.actors],
  );
  const diskOptions = useMemo(
    () => withSelectedDisks(collectDiskOptions(baseVideos), filters.disks),
    [baseVideos, filters.disks],
  );
  const shown = useMemo(() => filterVideos(sorted, filters), [sorted, filters]);
  const count = filterCount(filters);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  // A new filter, sort or keyword is a different list and starts at its top.
  // The mount pass is skipped so the shell's history scroll restore wins.
  const firstListKey = useRef(true);
  useEffect(() => {
    if (firstListKey.current) {
      firstListKey.current = false;
      return;
    }
    scrollRef.current?.scrollTo({ top: 0 });
  }, [listKey]);

  const setSort = useCallback((key: SortKey) => {
    setSortKey(key);
    writeSortKey(key);
    // Changing the field returns to that field's natural first direction
    // (newest/largest first, A→Z for 文件名).
    const dir = defaultSortDir(key);
    setSortDir(dir);
    writeSortDir(dir);
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
          size="sm"
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
                icon={<Icon className="size-3.5" name="filter" />}
                onClick={() => onFilterOpenChange(true)}
                variant="ghost"
              >
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
                onClick={() => setSortOpen(true)}
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

        <FilterChips
          className="mt-2"
          disks={diskOptions}
          filters={filters}
          onChange={onFiltersChange}
          onClear={resetFilters}
        />
      </div>

      <div className="scroll-area min-h-0 flex-1 px-3 pb-4 pt-2" data-scroll-key="main" ref={scrollRef}>
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
          <EmptyState description="试试减少标签、演员或硬盘筛选" title="没有符合条件的视频" />
        ) : viewMode === "grid" ? (
          <VideoGrid
            actions={actions}
            memoryKey={listKey}
            onMenuOpenChange={onMenuOpenChange}
            onOpen={onOpenVideo}
            openMenuId={openMenuId}
            videos={shown}
          />
        ) : (
          <VideoList
            actions={actions}
            memoryKey={listKey}
            onMenuOpenChange={onMenuOpenChange}
            onOpen={onOpenVideo}
            openMenuId={openMenuId}
            videos={shown}
          />
        )}
      </div>

      <FilterSheet
        actors={actorOptions}
        disks={diskOptions}
        filters={filters}
        onChange={onFiltersChange}
        onClose={() => onFilterOpenChange(false)}
        onReset={resetFilters}
        open={filterOpen}
        tags={tagOptions}
      />

      <SortSheet
        onClose={() => setSortOpen(false)}
        onSortDir={setDir}
        onSortKey={setSort}
        open={sortOpen}
        sortDir={sortDir}
        sortKey={sortKey}
      />
    </div>
  );
}
