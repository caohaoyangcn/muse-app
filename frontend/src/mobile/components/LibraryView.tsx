import { useCallback, useEffect, useMemo, useState } from "react";

import type { CardSize, Group, SortDir, SortKey, Video, ViewMode } from "../../types";
import { filterVideosByKeyword } from "../../utils/videoFilter";
import { listGroups, listVideos, videosInGroup } from "../api";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import {
  readCardSize,
  readSortDir,
  readSortKey,
  readViewMode,
  writeCardSize,
  writeSortDir,
  writeSortKey,
  writeViewMode,
} from "../prefs";
import { EmptyState, ErrorState, Loading } from "./States";
import { GroupSheet } from "./GroupSheet";
import { SortViewBar } from "./SortViewBar";
import { VideoGrid, VideoList } from "./VideoGrid";

const collator = new Intl.Collator("zh-CN", { numeric: true, sensitivity: "base" });

// Stable identities for the empty answers.
const NO_VIDEOS: Video[] = [];
const NO_GROUPS: Group[] = [];

/** The library tab: search, group filter, sort, grid/list and the card list. */
export function LibraryView({
  keyword,
  filterGroupId,
  onOpenVideo,
  onOpenMenu,
  onFilterGroupChange,
}: {
  keyword: string;
  filterGroupId: number | "all";
  onOpenVideo: (v: Video) => void;
  onOpenMenu: (v: Video) => void;
  onFilterGroupChange: (id: number | "all") => void;
}) {
  const [groupSheet, setGroupSheet] = useState(false);
  // Keyword for the group view's own box. The header search stays a
  // whole-library query; this one only narrows the group already on screen.
  const [groupFilter, setGroupFilter] = useState("");

  const [sortKey, setSortKey] = useState<SortKey>(readSortKey);
  const [sortDir, setSortDir] = useState<SortDir>(readSortDir);
  const [viewMode, setViewMode] = useState<ViewMode>(readViewMode);
  const [cardSize, setCardSize] = useState<CardSize>(readCardSize);

  // Search always spans the whole library (matching the desktop app);
  // otherwise the selected group narrows the list.
  const fetchLibrary = useCallback(async () => {
    const [list, groupList] = await Promise.all([
      keyword
        ? listVideos(keyword)
        : filterGroupId === "all"
          ? listVideos()
          : videosInGroup(filterGroupId),
      listGroups(),
    ]);
    return { videos: list ?? NO_VIDEOS, groups: groupList ?? NO_GROUPS };
  }, [keyword, filterGroupId]);

  const { data, loading, error, reload } = useLiveQuery(
    fetchLibrary,
    `library:${filterGroupId}:${keyword}`,
  );
  const videos = data?.videos ?? NO_VIDEOS;
  const groups = data?.groups ?? NO_GROUPS;

  const sorted = useMemo(() => {
    const copy = [...videos];
    const dir = sortDir === "asc" ? 1 : -1;
    copy.sort((a, b) => {
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
  }, [videos, sortKey, sortDir]);

  const activeGroup = groups.find((g) => g.id === filterGroupId);
  const groupView = filterGroupId !== "all";
  // Switching groups drops the previous group's filter (re-picking the same
  // group keeps it).
  useEffect(() => {
    setGroupFilter("");
  }, [filterGroupId]);
  const shown = useMemo(
    () => (groupView ? filterVideosByKeyword(sorted, groupFilter) : sorted),
    [sorted, groupFilter, groupView],
  );
  const groupFiltering = groupView && groupFilter.trim() !== "";
  const heading = keyword
    ? `“${keyword}” — ${sorted.length} 个结果`
    : groupFiltering
      ? `匹配 ${shown.length} / 共 ${sorted.length} 个视频`
      : filterGroupId === "all"
        ? `${sorted.length} 个视频`
        : `${activeGroup?.name ?? ""} — ${sorted.length} 个视频`;

  return (
    <div className="flex min-h-0 flex-1 flex-col px-4">
      <div className="flex items-center gap-2 py-2">
        <button
          className="flex shrink-0 items-center gap-1 rounded-full border border-[var(--border)] bg-[var(--panel-2)] px-3 py-1.5 text-xs text-[var(--text)]"
          onClick={() => setGroupSheet(true)}
          type="button"
        >
          {filterGroupId === "all" ? "分组" : activeGroup?.name ?? "分组"} ▾
        </button>
        <div className="min-w-0 flex-1 truncate text-xs text-[var(--muted)]">{heading}</div>
        {groupView ? (
          <input
            className="w-[8.5rem] shrink-0 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)] px-2.5 py-1.5 text-xs outline-none focus:border-[var(--accent)]"
            onChange={(e) => setGroupFilter(e.target.value)}
            placeholder="筛选本组"
            type="search"
            value={groupFilter}
          />
        ) : null}
      </div>

      <SortViewBar
        onSortKey={(k) => {
          setSortKey(k);
          writeSortKey(k);
          setSortDir("desc");
          writeSortDir("desc");
        }}
        onToggleDir={() => {
          const next = sortDir === "asc" ? "desc" : "asc";
          setSortDir(next);
          writeSortDir(next);
        }}
        onViewMode={(m) => {
          setViewMode(m);
          writeViewMode(m);
        }}
        onCardSize={(s) => {
          setCardSize(s);
          writeCardSize(s);
        }}
        sortDir={sortDir}
        sortKey={sortKey}
        viewMode={viewMode}
        cardSize={cardSize}
      />

      <div className="scroll-area min-h-0 flex-1 py-3" data-scroll-key="main">
        {loading ? (
          <Loading />
        ) : error ? (
          <ErrorState message={error} onRetry={reload} />
        ) : sorted.length === 0 ? (
          <EmptyState
            hint={keyword ? "换个关键词试试" : "在桌面端添加收藏后会出现在这里"}
            title={keyword ? "没有匹配的视频" : "收藏库为空"}
          />
        ) : shown.length === 0 ? (
          <EmptyState hint="换个关键词试试" title="本组没有匹配的视频" />
        ) : viewMode === "grid" ? (
          <VideoGrid cardSize={cardSize} onMenu={onOpenMenu} onOpen={onOpenVideo} videos={shown} />
        ) : (
          <VideoList cardSize={cardSize} onMenu={onOpenMenu} onOpen={onOpenVideo} videos={shown} />
        )}
      </div>

      <GroupSheet
        activeId={filterGroupId}
        groups={groups}
        onClose={() => setGroupSheet(false)}
        onSelect={onFilterGroupChange}
        open={groupSheet}
      />
    </div>
  );
}
