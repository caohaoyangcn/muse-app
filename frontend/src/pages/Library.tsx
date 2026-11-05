import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { EventsOn, OnFileDrop, OnFileDropOff } from "../../wailsjs/runtime";
import ActorFilmsView from "../components/ActorFilmsView";
import AddSourceDialog from "../components/AddSourceDialog";
import CloudBrowserDialog from "../components/CloudBrowserDialog";
import CloudDownloadBar from "../components/CloudDownloadBar";
import ActorIndexView from "../components/ActorIndexView";
import ConfirmDialog from "../components/ConfirmDialog";
import ContextMenu, { type MenuAction } from "../components/ContextMenu";
import DiskFilterBar from "../components/DiskFilterBar";
import DropOverlay from "../components/DropOverlay";
import DuplicateView from "../components/DuplicateView";
import EditVideoDialog from "../components/EditVideoDialog";
import EmptyState from "../components/EmptyState";
import ErrorBoundary from "../components/ErrorBoundary";
import FileScanButton from "../components/FileScanButton";
import GroupContextMenu, { type GroupMenuAction } from "../components/GroupContextMenu";
import GroupDialog from "../components/GroupDialog";
import GroupSuggestDialog, { type SuggestFile } from "../components/GroupSuggestDialog";
import SearchBar from "../components/SearchBar";
import ServerBar from "../components/ServerBar";
import Sidebar from "../components/Sidebar";
import ScrapeTaskView, { type ScrapeTaskStatus } from "../components/ScrapeTaskView";
import SortBar from "../components/SortBar";
import ViewControls from "../components/ViewControls";
import TabContextMenu, { type TabMenuAction } from "../components/TabContextMenu";
import TrashView from "../components/TrashView";
import Toasts, { type ToastItem } from "../components/Toasts";
import VideoGrid from "../components/VideoGrid";
import VideoList from "../components/VideoList";
import VideoDetailPanel from "../components/VideoDetailPanel";
import WorkspaceTabs, { type WorkspaceTabItem } from "../components/WorkspaceTabs";
import {
  addVideoToGroup,
  addVideos,
  addWatchDir,
  cancelCloudDownload,
  copyPath,
  createGroup,
  deleteGroup,
  likeVideo,
  listCloudConnections,
  listGroups,
  listVideos,
  openContainingFolder,
  openCloudPage,
  openVideo,
  pickVideoFiles,
  pickWatchDir,
  refreshStatus,
  refreshVolumes,
  removeVideo,
  renameGroup,
  reorderGroups,
  searchVideos,
  setGroupParent,
  setVideoGroups,
  suggestGroupsForVideos,
  videosInGroup,
} from "../services/api";
import { invalidate } from "../services/live";
import { useLiveQuery } from "../hooks/useLiveQuery";
import type {
  AddVideoResult,
  CardSize,
  Group,
  SortDir,
  SortKey,
  CloudDownloadProgress,
  Video,
  ViewFilter,
  ViewMode,
  WorkspaceTab,
} from "../types";
import { actorTabID, actorTabName } from "../types";
import { baseName } from "../utils/format";
import { beginVideoDrag } from "../utils/dragGhost";
import {
  collectDiskOptions,
  filterVideosByDisk,
  withSelectedDisks,
  type DiskOption,
} from "../utils/diskFilter";
import { filterVideosByKeyword } from "../utils/videoFilter";
import { videoDisplayName } from "../utils/videoName";
import {
  readCardSize,
  readSortDir,
  readSortKey,
  readViewMode,
  writeCardSize,
  writeSortDir,
  writeSortKey,
  writeViewMode,
} from "../utils/viewPrefs";
import museLogo from "../assets/images/muse-logo.png";
import { Button } from "../components/ui/button";
import { cn } from "../lib/utils";
import { ChevronLeft, ChevronRight } from "lucide-react";

// Upper bound on the browser-style visit history, so a long session cannot grow
// the stacks without limit.
const HISTORY_LIMIT = 50;

// How long a returning tab may wait for its asynchronously loaded rows before
// the remembered scroll offset clamps to the new content height.
const SCROLL_RESTORE_TIMEOUT_MS = 4000;

// Stable identities for the empty answers, so the memos below do not recompute
// on every render before the first read lands.
const NO_VIDEOS: Video[] = [];
const NO_GROUPS: Group[] = [];

export default function Library({
  onOpenSettings,
}: {
  /** `section` opens Settings directly on that submenu (the tour replay lives
   *  under 新手引导). */
  onOpenSettings: (section?: string) => void;
}) {
  const [keyword, setKeyword] = useState("");
  // What the list actually queries. Debounced so typing does not issue a read
  // per keystroke; cleared instantly.
  const [debouncedKeyword, setDebouncedKeyword] = useState("");
  const [filter, setFilter] = useState<ViewFilter>({ kind: "all" });
  // Keyword typed into the group view's own filter box. Separate from the
  // header `keyword`, which stays a whole-library query: this one only narrows
  // the group already on screen.
  const [groupKeyword, setGroupKeyword] = useState("");
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const [menu, setMenu] = useState<{ x: number; y: number; video: Video } | null>(null);
  const [groupMenu, setGroupMenu] = useState<{ x: number; y: number; group: Group } | null>(null);
  const [removeTarget, setRemoveTarget] = useState<Video | null>(null);
  const [missingOpen, setMissingOpen] = useState<Video | null>(null);
  const [editVideo, setEditVideo] = useState<Video | null>(null);
  const [detailTabs, setDetailTabs] = useState<Video[]>([]);
  // Open actor tabs, in strip order. The name is the tab's identity and its
  // display label; films and counts are resolved by the tab's own view.
  const [actorTabs, setActorTabs] = useState<string[]>([]);
  const [activeTab, setActiveTab] = useState<WorkspaceTab>("library");
  // Browser-style visit history over the tab strip: `backStack` holds the tabs
  // we came from, `forwardStack` the ones we stepped back out of. Kept as two
  // stacks rather than an index because pruning closed tabs then needs no index
  // fix-up.
  const [backStack, setBackStack] = useState<WorkspaceTab[]>([]);
  const [forwardStack, setForwardStack] = useState<WorkspaceTab[]>([]);
  const [tabMenu, setTabMenu] = useState<{ x: number; y: number; tab: WorkspaceTab } | null>(null);
  const mainRef = useRef<HTMLElement | null>(null);
  const scrollPositionsRef = useRef(new Map<WorkspaceTab, number>());
  // The tab whose scroll restore is still waiting for its content to load. Its
  // remembered position must not be replaced by the 0 it currently shows.
  const pendingRestoreRef = useRef<WorkspaceTab | null>(null);
  // The header search box. Ctrl/Cmd+F focuses it from anywhere in the library.
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  // Bumped by that shortcut; the focus effect below runs after the view that
  // must show the box has rendered (the box is unmounted on some panels).
  const [searchFocusToken, setSearchFocusToken] = useState(0);
  const [dragActive, setDragActive] = useState(false);
  const [groupDialog, setGroupDialog] = useState<
    { mode: "create" } | { mode: "rename"; group: Group } | null
  >(null);
  const [deleteGroupTarget, setDeleteGroupTarget] = useState<Group | null>(null);
  const [suggestFiles, setSuggestFiles] = useState<SuggestFile[] | null>(null);
  const [addSourceOpen, setAddSourceOpen] = useState(false);
  const [cloudBrowserOpen, setCloudBrowserOpen] = useState(false);
  const [cloudDownload, setCloudDownload] = useState<CloudDownloadProgress | null>(null);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  // Grid ordering: field + direction, applied in the frontend over whatever
  // list the active view/search produced. Defaults match the backend order
  // (newest added first).
  const [sortBy, setSortBy] = useState<SortKey>(readSortKey);
  const [sortDir, setSortDir] = useState<SortDir>(readSortDir);
  // Grid vs list rendering, remembered across sessions in localStorage (best
  // effort: a blocked store just falls back to the grid default).
  const [view, setView] = useState<ViewMode>(readViewMode);
  // Card width in px, shared by both view modes and remembered the same way.
  const [cardSize, setCardSize] = useState<CardSize>(readCardSize);
  // 硬盘筛选：canonical volume key -> root as the read model spelled it. Kept in
  // component state (not localStorage) because it is a way of looking at the
  // list, not a display preference; it starts at "全部".
  const [selectedDisks, setSelectedDisks] = useState<Map<string, string>>(() => new Map());

  const toastSeq = useRef(0);

  const pushToast = useCallback((t: Omit<ToastItem, "id">, ttl = 4000) => {
    const id = ++toastSeq.current;
    setToasts((prev) => [...prev, { ...t, id }]);
    window.setTimeout(() => setToasts((prev) => prev.filter((x) => x.id !== id)), ttl);
  }, []);

  const dismissToast = useCallback((id: number) => {
    setToasts((prev) => prev.filter((x) => x.id !== id));
  }, []);

  // Debounced search: instant on clear, 300ms while typing.
  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedKeyword(keyword.trim()), keyword ? 300 : 0);
    return () => window.clearTimeout(timer);
  }, [keyword]);

  // The list read model: the whole library, or one group's members. A search
  // replaces the source with its results, as before.
  const fetchLibraryVideos = useCallback(async (view: ViewFilter, search: string) => {
    const found = search ? await searchVideos(search) : await listVideos();
    if (view.kind !== "group") return found;
    const ids = new Set(await videosInGroup(view.id));
    return found.filter((v) => ids.has(v.id));
  }, []);

  const panelView =
    filter.kind === "scrapeTasks" ||
    filter.kind === "actors" ||
    filter.kind === "duplicates" ||
    filter.kind === "trash";

  // The grid list is the shell's own live query. A panel view loads its own
  // data, so this one is skipped — except while a video tab is open, because
  // that tab shows a record from this list and has to follow edits and fresh
  // covers even when a panel owns the content.
  const library = useLiveQuery(
    () => fetchLibraryVideos(filter, debouncedKeyword),
    `library:${filter.kind}:${filter.kind === "group" ? filter.id : 0}:${debouncedKeyword}`,
    {
      enabled: !panelView || detailTabs.length > 0,
      keepPreviousData: true,
      onError: (e) => pushToast({ kind: "error", message: "加载收藏库失败", details: [String(e)] }),
    },
  );

  const groupsQuery = useLiveQuery(listGroups, "groups", {
    onError: (e) => pushToast({ kind: "error", message: "加载分组失败", details: [String(e)] }),
  });

  // Only the add-source dialog needs this summary. It is a local read (the
  // backend never touches the network here), so it is cheap enough to keep
  // mounted and refresh after cloud:status invalidations.
  const cloudQuery = useLiveQuery(listCloudConnections, "cloud-connections");

  // Cache progress is a payload event, so it lives with the view that draws it
  // (not in services/events.ts). A completed/error card hides itself after a
  // moment; the download itself keeps running in the backend until it finishes
  // or is cancelled.
  useEffect(() => {
    const off = EventsOn("cloud:download", (progress?: CloudDownloadProgress) => {
      if (!progress) return;
      setCloudDownload(progress);
      if (progress.status !== "downloading") {
        const ttl = progress.status === "error" ? 7000 : 3000;
        window.setTimeout(() => {
          setCloudDownload((current) => (current === progress ? null : current));
        }, ttl);
      }
    });
    return () => off();
  }, []);

  const videos = library.data ?? NO_VIDEOS;
  const groups = groupsQuery.data ?? NO_GROUPS;
  const loading = library.loading;

  // The list is the only read model carrying a full record, so it is what keeps
  // an open video tab current: a title edit, a new cover or a like lands here
  // without closing the tab. A deleted favorite keeps its tab until the shell
  // closes it.
  useEffect(() => {
    const found = library.data;
    if (!found) return;
    setDetailTabs((prev) => {
      let changed = false;
      const next = prev.map((tab) => {
        const fresh = found.find((v) => v.id === tab.id);
        if (!fresh || fresh === tab) return tab;
        changed = true;
        return fresh;
      });
      return changed ? next : prev;
    });
  }, [library.data]);

  // Switch the active tab without touching history. Used by history navigation
  // itself and by Ctrl+Tab, which in a browser cycles tabs without recording a
  // visit.
  const activateTab = useCallback((next: WorkspaceTab) => {
    if (next === activeTab) return;
    const main = mainRef.current;
    // A tab whose content is still loading shows 0 at the moment; filing that
    // would erase the position its in-flight restore is about to apply.
    if (main && pendingRestoreRef.current !== activeTab) {
      scrollPositionsRef.current.set(activeTab, main.scrollTop);
    }
    setActiveTab(next);
  }, [activeTab]);

  // User navigation: switch tabs and remember where we came from. This is what
  // feeds the browser-style back/forward buttons.
  const selectTab = useCallback((next: WorkspaceTab) => {
    if (next === activeTab) return;
    setBackStack((prev) => [...prev, activeTab].slice(-HISTORY_LIMIT));
    setForwardStack([]);
    activateTab(next);
  }, [activeTab, activateTab]);

  const goBack = useCallback(() => {
    const target = backStack[backStack.length - 1];
    if (target == null) return;
    setBackStack((prev) => prev.slice(0, -1));
    setForwardStack((prev) => [...prev, activeTab].slice(-HISTORY_LIMIT));
    activateTab(target);
  }, [backStack, activeTab, activateTab]);

  const goForward = useCallback(() => {
    const target = forwardStack[forwardStack.length - 1];
    if (target == null) return;
    setForwardStack((prev) => prev.slice(0, -1));
    setBackStack((prev) => [...prev, activeTab].slice(-HISTORY_LIMIT));
    activateTab(target);
  }, [forwardStack, activeTab, activateTab]);

  // Drop closed tabs from both stacks so back/forward can never point at a tab
  // that no longer exists.
  const forgetTabs = useCallback((ids: WorkspaceTab[]) => {
    if (ids.length === 0) return;
    const dead = new Set(ids);
    setBackStack((prev) => prev.filter((tab) => !dead.has(tab)));
    setForwardStack((prev) => prev.filter((tab) => !dead.has(tab)));
  }, []);

  useEffect(() => {
    const target = scrollPositionsRef.current.get(activeTab) ?? 0;
    pendingRestoreRef.current = activeTab;
    let cancelled = false;
    let frame = 0;
    // Actor films and 查重 fetch their rows after the tab mounts. Restoring on
    // the first frame would clamp to 0 while the pane is still empty, so wait
    // for enough content to hold the remembered offset. The deadline stops the
    // loop when the list genuinely shrank; the position then clamps to its end.
    const deadline = performance.now() + SCROLL_RESTORE_TIMEOUT_MS;
    const finish = () => {
      if (pendingRestoreRef.current === activeTab) pendingRestoreRef.current = null;
    };
    const attempt = () => {
      if (cancelled) return;
      const main = mainRef.current;
      if (!main) {
        if (performance.now() < deadline) {
          frame = window.requestAnimationFrame(attempt);
        } else {
          finish();
        }
        return;
      }
      const max = main.scrollHeight - main.clientHeight;
      if (target > 0 && max < target) {
        if (performance.now() < deadline) {
          frame = window.requestAnimationFrame(attempt);
        } else {
          main.scrollTop = max;
          finish();
        }
        return;
      }
      if (Math.abs(main.scrollTop - target) > 1) main.scrollTop = target;
      if (Math.abs(main.scrollTop - target) > 1 && performance.now() < deadline) {
        frame = window.requestAnimationFrame(attempt);
        return;
      }
      finish();
    };
    frame = window.requestAnimationFrame(attempt);
    return () => {
      cancelled = true;
      window.cancelAnimationFrame(frame);
      // Keep the remembered target: the entry is being left while its restore
      // has not finished, and activateTab relies on pendingRestoreRef to avoid
      // overwriting it with the empty pane's 0.
    };
  }, [activeTab]);

  // Ctrl/Cmd+F focus. Runs a frame later so the search box exists even when the
  // same keypress just switched to a view that renders it.
  useEffect(() => {
    if (searchFocusToken === 0) return;
    const frame = window.requestAnimationFrame(() => {
      const input = searchInputRef.current;
      if (!input) return;
      input.focus();
      // Select the current query so the next keystroke starts a new one, the
      // way a browser's own find box behaves.
      input.select();
    });
    return () => window.cancelAnimationFrame(frame);
  }, [searchFocusToken]);

  const openDetail = useCallback((video: Video) => {
    setDetailTabs((prev) => {
      const existing = prev.find((tab) => tab.id === video.id);
      return existing ? prev.map((tab) => (tab.id === video.id ? video : tab)) : [...prev, video];
    });
    selectTab(video.id);
  }, [selectTab]);

  // Open (or focus) an actor's film list as its own tab. Re-clicking an actor
  // chip reuses the existing tab rather than opening a duplicate.
  const openActor = useCallback((name: string) => {
    setActorTabs((prev) => (prev.includes(name) ? prev : [...prev, name]));
    selectTab(actorTabID(name));
  }, [selectTab]);

  const closeActor = useCallback((name: string) => {
    const tab = actorTabID(name);
    const index = actorTabs.indexOf(name);
    const nextActive = activeTab === tab
      ? actorTabs[index - 1] != null
        ? actorTabID(actorTabs[index - 1])
        : detailTabs[detailTabs.length - 1]?.id ?? "library"
      : activeTab;
    if (nextActive !== activeTab) selectTab(nextActive);
    scrollPositionsRef.current.delete(tab);
    forgetTabs([tab]);
    setActorTabs((prev) => prev.filter((n) => n !== name));
  }, [activeTab, actorTabs, detailTabs, selectTab, forgetTabs]);

  // After a merge/unmerge the actor grouping changes. A tab for an absorbed
  // alias would keep showing a stale film list, so close every actor tab and
  // return to the index (where the merged actor can be reopened). Video tabs
  // are untouched.
  const handleActorsMerged = useCallback(() => {
    if (actorTabs.length === 0) return;
    const dropped = actorTabs.map(actorTabID);
    dropped.forEach((tab) => scrollPositionsRef.current.delete(tab));
    forgetTabs(dropped);
    setActorTabs([]);
    if (actorTabName(activeTab) != null) selectTab("library");
  }, [actorTabs, activeTab, forgetTabs, selectTab]);

  const closeDetail = useCallback((videoID: number) => {
    const index = detailTabs.findIndex((tab) => tab.id === videoID);
    const nextActive = activeTab === videoID
      ? detailTabs[Math.max(0, index - 1)]?.id ?? "library"
      : activeTab;
    if (nextActive !== activeTab) selectTab(nextActive);
    scrollPositionsRef.current.delete(videoID);
    forgetTabs([videoID]);
    setDetailTabs((prev) => prev.filter((tab) => tab.id !== videoID));
  }, [activeTab, detailTabs, selectTab, forgetTabs]);

  // Workspace tab order: the library home first, then the open video tabs and
  // actor tabs.
  const tabOrder = useMemo<WorkspaceTab[]>(
    () => ["library", ...detailTabs.map((tab) => tab.id), ...actorTabs.map(actorTabID)],
    [detailTabs, actorTabs],
  );

  // Ctrl+Tab / Ctrl+PageUp/Down walk the tab strip in order (wrapping at both
  // ends) without recording history — the browser behaviour for these keys.
  const cycleTab = useCallback((delta: 1 | -1) => {
    if (tabOrder.length < 2) return;
    const index = tabOrder.indexOf(activeTab);
    activateTab(tabOrder[(index + delta + tabOrder.length) % tabOrder.length]);
  }, [activeTab, tabOrder, activateTab]);

  // Close every non-library tab, keeping the library tab (the "home page")
  // active.
  const closeAllTabs = useCallback(() => {
    selectTab("library"); // select first so it saves the outgoing scroll position
    detailTabs.forEach((tab) => scrollPositionsRef.current.delete(tab.id));
    actorTabs.forEach((name) => scrollPositionsRef.current.delete(actorTabID(name)));
    forgetTabs([...detailTabs.map((tab) => tab.id), ...actorTabs.map(actorTabID)]);
    setDetailTabs([]);
    setActorTabs([]);
  }, [detailTabs, actorTabs, selectTab, forgetTabs]);

  // Keep the right-clicked tab and drop the rest. The library tab can never be
  // closed, so keeping it means closing everything else.
  const closeOtherTabs = useCallback((keep: WorkspaceTab) => {
    selectTab(keep); // select first so it saves the outgoing scroll position
    const keepVideoId = typeof keep === "number" ? keep : null;
    const keepActor = actorTabName(keep);
    const dropped: WorkspaceTab[] = [];
    detailTabs.forEach((tab) => {
      if (tab.id !== keepVideoId) {
        scrollPositionsRef.current.delete(tab.id);
        dropped.push(tab.id);
      }
    });
    actorTabs.forEach((name) => {
      if (name !== keepActor) {
        scrollPositionsRef.current.delete(actorTabID(name));
        dropped.push(actorTabID(name));
      }
    });
    forgetTabs(dropped);
    setDetailTabs((prev) => prev.filter((tab) => tab.id === keepVideoId));
    setActorTabs((prev) => (keepActor == null ? [] : prev.filter((n) => n === keepActor)));
  }, [detailTabs, actorTabs, selectTab, forgetTabs]);

  const handleTabMenuAction = useCallback((action: TabMenuAction, tab: WorkspaceTab) => {
    const name = actorTabName(tab);
    if (action === "close" && tab !== "library") {
      if (name != null) closeActor(name);
      else closeDetail(tab as number);
    }
    else if (action === "closeOthers") closeOtherTabs(tab);
    else if (action === "closeAll") closeAllTabs();
  }, [closeDetail, closeActor, closeOtherTabs, closeAllTabs]);

  const selectFilter = useCallback((next: ViewFilter) => {
    selectTab("library");
    setFilter(next);
  }, [selectTab]);

  const selectScrapeStatus = useCallback((status: ScrapeTaskStatus) => {
    selectFilter({ kind: "scrapeTasks", status });
  }, [selectFilter]);

  // The visible list is the loaded list re-sorted in place (the grid is fully
  // rendered, so an in-memory sort covers every view). fileName uses a
  // locale-aware collator with numeric ordering so "2" sorts before "10"
  // across Chinese/English names; createdAt ties break by id to mirror the
  // backend's `created_at DESC, id DESC`.
  const nameCollator = useMemo(
    () => new Intl.Collator(undefined, { numeric: true, sensitivity: "base" }),
    [],
  );
  const sortedVideos = useMemo(() => {
    const list = [...videos];
    const dir = sortDir === "asc" ? 1 : -1;
    list.sort((a, b) => {
      let c: number;
      if (sortBy === "fileName") c = nameCollator.compare(a.fileName, b.fileName);
      else if (sortBy === "fileSize") c = a.fileSize - b.fileSize;
      else if (sortBy === "likeCount") c = a.likeCount - b.likeCount || a.id - b.id;
      else c = a.createdAt - b.createdAt || a.id - b.id;
      return c * dir;
    });
    return list;
  }, [videos, sortBy, sortDir, nameCollator]);

  const handleSortChange = useCallback((key: SortKey, dir: SortDir) => {
    setSortBy(key);
    setSortDir(dir);
    writeSortKey(key);
    writeSortDir(dir);
  }, []);

  // Each chip toggles one disk independently, so the result is the union of
  // the selected disks. Clicking "全部" clears the whole set.
  const toggleDisk = useCallback((option: DiskOption) => {
    setSelectedDisks((prev) => {
      const next = new Map(prev);
      if (next.has(option.key)) next.delete(option.key);
      else next.set(option.key, option.root);
      return next;
    });
  }, []);

  const clearDisks = useCallback(() => setSelectedDisks(new Map()), []);

  // The empty-result screen can clear everything that narrowed the list at
  // once, including the group view's own keyword.
  const clearViewFilters = useCallback(() => {
    setGroupKeyword("");
    setSelectedDisks(new Map());
  }, []);

  // The group view's own keyword narrows the list already loaded for that group.
  // Only group views have the box; every other view sees the unfiltered list.
  const visibleVideos = useMemo(
    () => (filter.kind === "group" ? filterVideosByKeyword(sortedVideos, groupKeyword) : sortedVideos),
    [sortedVideos, groupKeyword, filter.kind],
  );
  const groupFiltering = filter.kind === "group" && groupKeyword.trim() !== "";

  // 硬盘筛选 works on the loaded list, like the group keyword and the sort. The
  // options describe the disks that list currently holds; a selected disk that
  // a group or a search no longer contains stays selected (and visible with a
  // zero count) until the user clears it.
  const diskOptions = useMemo(() => collectDiskOptions(videos), [videos]);
  const diskBarOptions = useMemo(
    () => withSelectedDisks(diskOptions, selectedDisks),
    [diskOptions, selectedDisks],
  );
  const selectedDiskKeys = useMemo(() => new Set(selectedDisks.keys()), [selectedDisks]);
  const diskFiltering = selectedDisks.size > 0;
  const shownVideos = useMemo(
    () => filterVideosByDisk(visibleVideos, selectedDiskKeys),
    [visibleVideos, selectedDiskKeys],
  );
  const selectedDiskLabels = useMemo(
    () => diskBarOptions.filter((disk) => selectedDiskKeys.has(disk.key)).map((disk) => disk.label),
    [diskBarOptions, selectedDiskKeys],
  );
  const filtering = groupFiltering || diskFiltering;
  const filterSummary = [
    groupFiltering ? `关键词“${groupKeyword.trim()}”` : "",
    diskFiltering ? `硬盘 ${selectedDiskLabels.join("、")}` : "",
  ].filter(Boolean).join(" 和 ");

  // Switching groups (or leaving the group views) must not carry the previous
  // group's filter along; re-clicking the same group deliberately keeps it.
  const activeGroupKey = filter.kind === "group" ? filter.id : null;
  useEffect(() => {
    setGroupKeyword("");
  }, [activeGroupKey]);

  // A filter can hide an already-selected card. Drop those ids so the "已选"
  // badge and any batch action only ever describe videos the user can see.
  useEffect(() => {
    setSelectedIds((prev) => {
      if (prev.size === 0) return prev;
      const visible = new Set(shownVideos.map((v) => v.id));
      const next = new Set([...prev].filter((id) => visible.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [shownVideos]);

  const handleViewChange = useCallback((v: ViewMode) => {
    setView(v);
    writeViewMode(v);
  }, []);

  const handleCardSizeChange = useCallback((s: CardSize) => {
    setCardSize(s);
    writeCardSize(s);
  }, []);

  const toggleSelect = useCallback((id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  // Remove one id from the selection (a removed favorite must not linger as a
  // phantom highlight).
  const dropFromSelection = useCallback((id: number) => {
    setSelectedIds((prev) => {
      if (!prev.has(id)) return prev;
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
  }, []);

  // 查重 removed some versions: close their detail tabs (they no longer have a
  // record to show) and refresh the library and sidebar counts, exactly as a
  // removal from the grid does.
  const handleDuplicatesResolved = useCallback((removedIds: number[]) => {
    removedIds.forEach(closeDetail);
    removedIds.forEach(dropFromSelection);
    invalidate();
  }, [closeDetail, dropFromSelection]);

  // Which disks are attached is a property of the machine, so the read model
  // can only carry what the last observation saw. Ask once after mounting: a
  // card whose drive is unplugged gets its warning on the first screen instead
  // of after the next background poll. Best effort — the poll is the safety net.
  useEffect(() => {
    refreshVolumes()
      .then((volumes) => {
        // Reload when a disk is away, or when the probe saw one come back
        // (the backend already revived the favorites on it).
        if (volumes.some((volume) => !volume.online || volume.changed)) invalidate();
      })
      .catch(() => {
        /* the background poll retries */
      });
  }, []);

  // The list re-reads itself when `filter` (part of the query key) changes:
  // clicking a group shows its videos immediately, and clicking All Favorites
  // returns to the full library.

  const showAddSummary = useCallback(
    (results: AddVideoResult[] | null) => {
      if (!results || results.length === 0) return;
      const added = results.filter((r) => r.success);
      // A reconnected favorite is not a new one: the library already had these
      // bytes, under a path the file has left.
      const relinked = added.filter((r) => r.relocated);
      const fresh = added.length - relinked.length;
      const skipped = results.filter((r) => !r.success);
      const summary = [
        fresh > 0 ? `已添加 ${fresh} 个视频` : "",
        relinked.length > 0 ? `重连 ${relinked.length} 个已移动的文件` : "",
      ]
        .filter(Boolean)
        .join("，");
      if (skipped.length === 0) {
        pushToast({
          kind: "success",
          message: summary,
        });
        return;
      }
      const details = skipped
        .slice(0, 5)
        .map((r) => `${baseName(r.path)} — ${r.error ?? "失败"}`);
      if (skipped.length > 5) details.push(`…以及另外 ${skipped.length - 5} 个`);
      if (added.length === 0) {
        pushToast({ kind: "error", message: "未添加任何内容", details }, 6000);
      } else {
        pushToast(
          { kind: "info", message: `${summary}，跳过 ${skipped.length} 个`, details },
          6000,
        );
      }
    },
    [pushToast],
  );

  // After a batch add, infer per-file group suggestions from file names and open
  // the suggestion carousel for files that have any. Resolution-based grouping
  // (4K) is handled automatically by the backend, so only name matches appear.
  const showGroupSuggestions = useCallback(
    async (results: AddVideoResult[]) => {
      // A reconnected file is not a new one: it went through these rules when it
      // was first added, so suggesting its groups again is noise.
      const added = results.filter((r) => r.success && r.videoId && !r.relocated);
      if (added.length === 0) return;
      try {
        const byPath = new Map(added.map((r) => [r.path, r.videoId!]));
        const suggestions = await suggestGroupsForVideos(added.map((r) => r.path));
        const files: SuggestFile[] = suggestions
          .filter((s) => s.suggested.length > 0)
          .map((s) => {
            const videoId = byPath.get(s.path) ?? 0;
            if (!videoId) return null;
            return { videoId, path: s.path, fileName: s.fileName, suggested: s.suggested };
          })
          .filter((f): f is SuggestFile => f !== null);
        if (files.length > 0) setSuggestFiles(files);
      } catch (e) {
        // Suggestions are purely additive — a failure never blocks the add flow.
        pushToast({ kind: "error", message: "获取分组建议失败", details: [String(e)] });
      }
    },
    [pushToast],
  );

  // Data freshness is handled once in services/events.ts; this subscription is
  // only what the user needs to be *told* about a volume reconciliation that
  // removed records (or deliberately kept them).
  useEffect(() => {
    const off = EventsOn(
      "video:missing:swept",
      (data?: { root?: string; removed?: number; skipped?: number }) => {
        if (!data) return;
        const where = data.root ? [`卷：${data.root}`] : [];
        if (data.removed) {
          pushToast({
            kind: "info",
            message: `已自动将 ${data.removed} 个文件缺失的收藏移入回收站`,
            details: [...where, "可在回收站恢复；源文件未被删除"],
          });
        }
        if (data.skipped) {
          pushToast({
            kind: "error",
            message: `有 ${data.skipped} 个文件缺失的收藏未自动移入回收站`,
            details: [
              ...where,
              "该磁盘上的收藏全部找不到文件，已保留记录以防磁盘未正确接入；请确认后手动移入回收站",
            ],
          });
        }
      },
    );
    return off;
  }, [pushToast]);

  const handleApplySuggestedGroups = useCallback(
    async (videoId: number, groupIds: number[]) => {
      // The video is freshly added, so each group is an independent, idempotent
      // add (allows a file's existing memberships to be preserved).
      await Promise.all(groupIds.map((gid) => addVideoToGroup(videoId, gid)));
      invalidate();
    },
    [],
  );

  // Data freshness is handled once in services/events.ts; this subscription is
  // only what the user needs to be *told* about a folder-watch pass.
  useEffect(() => {
    const off = EventsOn(
      "watch:scanned",
      (data?: { path?: string; added?: number; relocated?: number; failed?: number }) => {
        if (!data || !(data.added || data.relocated || data.failed)) return;
        if (data.added) {
          pushToast({
            kind: "success",
            message: `目录监控：已自动添加 ${data.added} 个视频`,
            details: data.path ? [data.path] : undefined,
          });
        }
        if (data.relocated) {
          pushToast({
            kind: "success",
            message: `目录监控：已重新连接 ${data.relocated} 个移动的文件`,
            details: data.path ? [data.path] : undefined,
          });
        }
        if (data.failed) {
          pushToast({
            kind: "error",
            message: `目录监控：${data.failed} 个文件添加失败`,
            details: data.path ? [data.path] : undefined,
          });
        }
      },
    );
    return off;
  }, [pushToast]);

  // Native drag & drop of video files onto the window (spec-v1.1.md).
  useEffect(() => {
    OnFileDrop(async (_x: number, _y: number, paths: string[]) => {
      // Wails delivers file drops through its native event channel, which may
      // not emit the browser-level `drop` event used by the visual overlay.
      // Clear the overlay at this boundary so success, empty drops, and
      // failures cannot leave the drop hint stuck on screen.
      setDragActive(false);
      setAddSourceOpen(false);
      const validPaths = (paths ?? []).filter((path) => path.trim() !== "");
      if (validPaths.length === 0) return;
      try {
        const results = await addVideos(validPaths);
        showAddSummary(results);
        void showGroupSuggestions(results);
        invalidate();
      } catch (e) {
        pushToast({ kind: "error", message: "添加视频失败", details: [String(e)] });
      }
    }, true);
    return () => OnFileDropOff();
  }, [showAddSummary, showGroupSuggestions, pushToast]);

  // Drag-over visuals for file drops.
  useEffect(() => {
    let depth = 0;
    const hasFiles = (e: DragEvent) => Array.from(e.dataTransfer?.types ?? []).includes("Files");
    const onEnter = (e: DragEvent) => {
      if (!hasFiles(e)) return;
      depth++;
      setDragActive(true);
    };
    const onOver = (e: DragEvent) => {
      if (!hasFiles(e)) return;
      e.preventDefault();
    };
    const onLeave = (e: DragEvent) => {
      if (!hasFiles(e)) return;
      depth = Math.max(0, depth - 1);
      if (depth === 0) setDragActive(false);
    };
    const onDrop = () => {
      depth = 0;
      setDragActive(false);
    };
    document.addEventListener("dragenter", onEnter);
    document.addEventListener("dragover", onOver);
    document.addEventListener("dragleave", onLeave);
    document.addEventListener("drop", onDrop);
    return () => {
      document.removeEventListener("dragenter", onEnter);
      document.removeEventListener("dragover", onOver);
      document.removeEventListener("dragleave", onLeave);
      document.removeEventListener("drop", onDrop);
    };
  }, []);

  const handleAddLocal = async () => {
    // Close the source router first: the native picker is the next layer, and
    // a cancelled picker should leave the user on the library, not behind a
    // now-irrelevant modal.
    setAddSourceOpen(false);
    try {
      const paths = await pickVideoFiles();
      if (paths && paths.length > 0) {
        const results = await addVideos(paths);
        showAddSummary(results);
        void showGroupSuggestions(results);
        invalidate();
      }
    } catch (e) {
      pushToast({ kind: "error", message: "添加视频失败", details: [String(e)] });
    }
  };

  const handleAddFolder = async () => {
    setAddSourceOpen(false);
    let dir: string;
    try {
      dir = await pickWatchDir();
    } catch (e) {
      pushToast({ kind: "error", message: "无法选择文件夹", details: [String(e)] });
      return;
    }
    if (!dir) return;

    try {
      await addWatchDir(dir);
      pushToast({
        kind: "success",
        message: "已加入目录监控，正在扫描…",
        details: [dir],
      });
      // The scan runs asynchronously; watch:scanned reports the added count,
      // reconnects and failures when it finishes.
      invalidate();
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e);
      if (/already|已在/.test(message)) {
        pushToast({
          kind: "info",
          message: "该目录已在目录监控中",
          details: [dir],
        });
      } else {
        pushToast({ kind: "error", message: "添加监控目录失败", details: [message] });
      }
    }
  };

  const handleAddCloud = () => {
    setAddSourceOpen(false);
    setCloudBrowserOpen(true);
  };

  const handleCloudAdded = (results: AddVideoResult[]) => {
    showAddSummary(results);
    void showGroupSuggestions(results);
    invalidate();
  };

  const handleOpen = useCallback(async (video: Video) => {
    if (video.volumeOffline) {
      pushToast({
        kind: "error",
        message: "磁盘未接入",
        details: [`${video.volumeRoot || "磁盘"} 未接入，插回移动硬盘后即可播放`, video.path],
      });
      return;
    }
    try {
      await openVideo(video.id);
    } catch (e) {
      // The backend marks a file missing only when its volume is attached (or,
      // for a WebDAV source, when the remote path answers 404). Ask it which
      // case this was so a network failure never offers to remove a favorite
      // whose remote file is safe.
      invalidate();
      const checked = await refreshStatus(video.id).catch(() => null);
      if (checked?.volumeOffline) {
        pushToast({
          kind: "error",
          message: "磁盘未接入",
          details: [`${checked.volumeRoot || "磁盘"} 未接入，插回后即可播放`, video.path],
        });
        return;
      }
      if (video.sourceId > 0) {
        if (checked?.sourceOffline) {
          pushToast({
            kind: "error",
            message: "网盘未连接",
            details: [`「${video.sourceName || "网盘"}」当前离线，恢复后即可播放`, video.remotePath],
          });
          return;
        }
        if (checked?.status === "missing") {
          setMissingOpen(video);
          return;
        }
        pushToast({
          kind: "error",
          message: "网盘视频播放失败",
          details: [String(e)],
        });
        return;
      }
      setMissingOpen(video);
    }
  }, [pushToast]);

  // Reveal a file in the OS file manager. A failure (usually a missing file)
  // makes the backend mark it missing, so refresh the views and report it.
  const handleRevealPath = useCallback((video: Video) => {
    openContainingFolder(video.id).catch((e) => {
      invalidate();
      pushToast({
        kind: "error",
        message: video.volumeOffline ? "磁盘未接入" : "无法定位文件",
        details: [String(e)],
      });
    });
  }, [pushToast]);

  // ---- Single-selection keyboard control ----
  // With exactly one card selected: arrow keys move the selection, Enter opens
  // it, Delete opens the remove confirmation. Ctrl/Cmd+A selects every video in
  // the current list. Active only while a video view is showing and no
  // dialog/menu is open; any text field or focused button keeps its own keys.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // The panel views (scrape board, actor index, 查重) show no video cards, so
      // card navigation and Ctrl+A must not act on the stale grid list behind
      // them.
      const panelView = filter.kind === "scrapeTasks" || filter.kind === "actors" ||
        filter.kind === "duplicates";
      if (
        activeTab !== "library" || panelView || menu || groupMenu || groupDialog || deleteGroupTarget || suggestFiles ||
        removeTarget || editVideo || tabMenu
      ) {
        return;
      }
      const el = document.activeElement as HTMLElement | null;
      if (el) {
        const tag = el.tagName;
        if (tag === "INPUT" || tag === "TEXTAREA" || tag === "BUTTON" || el.isContentEditable) {
          return;
        }
      }

      // Ctrl/Cmd+A — select the whole current list (the grid rows), regardless
      // of how many are currently selected. Standard browser select-all is
      // prevented by default inside this app context, so handle it ourselves.
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "a") {
        e.preventDefault();
        if (shownVideos.length > 0) setSelectedIds(new Set(shownVideos.map((v) => v.id)));
        return;
      }

      // Esc — drop the whole selection (single-click or marquee/Ctrl+A alike).
      if (e.key === "Escape" && selectedIds.size > 0) {
        e.preventDefault();
        setSelectedIds(new Set());
        return;
      }

      // Modified keys belong to the app-level shortcuts (tab navigation and
      // friends) handled by their own listener below.
      if (e.altKey || e.ctrlKey || e.metaKey) return;

      if (selectedIds.size !== 1) return;
      const selectedId = [...selectedIds][0];
      const idx = shownVideos.findIndex((v) => v.id === selectedId);
      if (idx < 0) return;
      const current = shownVideos[idx];

      if (e.key === "Enter") {
        e.preventDefault();
        openDetail(current);
        return;
      }
      if (e.key === "Delete") {
        e.preventDefault();
        setRemoveTarget(current);
        return;
      }
      if (e.key === " ") {
        e.preventDefault();
        setEditVideo(current);
        return;
      }

      // Vertical step: one row of the grid (resolved column count from the
      // rendered CSS grid); the list view is single-column so step by one.
      let step = 1;
      if (view === "grid" && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
        const grid = document.querySelector(".grid");
        const cols = grid
          ? getComputedStyle(grid).gridTemplateColumns.split(" ").filter(Boolean).length
          : 1;
        step = Math.max(1, cols);
      }
      let next: number;
      if (e.key === "ArrowRight") next = idx + 1;
      else if (e.key === "ArrowLeft") next = idx - 1;
      else if (e.key === "ArrowDown") next = idx + step;
      else if (e.key === "ArrowUp") next = idx - step;
      else return;

      e.preventDefault();
      if (next < 0 || next >= shownVideos.length) return;
      // Selecting is enough: the grid/list window scrolls the newly selected
      // item into view, including when it was never mounted.
      setSelectedIds(new Set([shownVideos[next].id]));
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [
    selectedIds, shownVideos, view, filter, handleOpen, openDetail, activeTab,
    menu, groupMenu, groupDialog, deleteGroupTarget, suggestFiles,
    removeTarget, editVideo, tabMenu,
  ]);

  // ---- Workspace tab navigation ----
  // Alt+←/→ are the browser's back/forward and replay visit history (the same
  // thing the header buttons do); Ctrl+Tab and Ctrl+PageUp/Down cycle the tab
  // strip without recording history. Modifiers keep these distinct from the
  // plain arrow keys that move the grid selection, and a text field or open
  // dialog keeps the keys for itself.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (
        menu || groupMenu || tabMenu || groupDialog || deleteGroupTarget || suggestFiles ||
        removeTarget || editVideo
      ) {
        return;
      }
      const el = document.activeElement as HTMLElement | null;
      if (el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.isContentEditable)) return;

      if (e.altKey && !e.ctrlKey && !e.shiftKey && e.key === "ArrowLeft") {
        e.preventDefault();
        goBack();
        return;
      }
      if (e.altKey && !e.ctrlKey && !e.shiftKey && e.key === "ArrowRight") {
        e.preventDefault();
        goForward();
        return;
      }

      let delta: 1 | -1 | null = null;
      if (e.ctrlKey && e.key === "Tab") delta = e.shiftKey ? -1 : 1;
      else if (e.ctrlKey && e.key === "PageDown") delta = 1;
      else if (e.ctrlKey && e.key === "PageUp") delta = -1;
      if (delta == null) return;

      e.preventDefault();
      cycleTab(delta);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [
    goBack, goForward, cycleTab, menu, groupMenu, tabMenu, groupDialog, deleteGroupTarget,
    suggestFiles, removeTarget, editVideo,
  ]);

  // ---- Ctrl/Cmd+F: find a video ----
  // The library has no find bar of its own, and the WebView's native find only
  // sees what is on screen. The shortcut therefore always lands on a video list
  // — leaving a video/actor tab behind, and stepping out of the 演员 / 查重 /
  // 刮削任务 panels, which are not the video list (their filters would leave the
  // box searching something other than videos). The current list (全部收藏 or a
  // group) is kept; the header box is focused with its query selected.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey) || e.shiftKey || e.altKey) return;
      if (e.key.toLowerCase() !== "f") return;
      if (
        menu || groupMenu || tabMenu || groupDialog || deleteGroupTarget || suggestFiles ||
        removeTarget || editVideo
      ) {
        return;
      }
      e.preventDefault();
      if (activeTab !== "library") selectTab("library");
      if (filter.kind !== "all" && filter.kind !== "group") {
        setFilter({ kind: "all" });
      }
      setSearchFocusToken((n) => n + 1);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [
    activeTab, filter.kind, selectTab, menu, groupMenu, tabMenu, groupDialog,
    deleteGroupTarget, suggestFiles, removeTarget, editVideo,
  ]);

  const handleMenuAction = (action: MenuAction, video: Video) => {
    switch (action) {
      case "open":
        void handleOpen(video);
        break;
      case "details":
        openDetail(video);
        break;
      case "openFolder":
        if (video.sourceId > 0) {
          openCloudPage(video.id).catch((e) =>
            pushToast({ kind: "error", message: "无法打开 OpenList 管理页", details: [String(e)] }),
          );
        } else {
          handleRevealPath(video);
        }
        break;
      case "copyPath":
        copyPath(video.id)
          .then(() => pushToast({ kind: "success", message: "路径已复制" }))
          .catch((e) => pushToast({ kind: "error", message: String(e) }));
        break;
      case "edit":
        setEditVideo(video);
        break;
      case "remove":
        setRemoveTarget(video);
        break;
    }
  };

  // A failed removal means the record it named is not in the library any more,
  // so the screens showing it have to follow the backend: close its detail tab
  // and re-read the list, groups and duplicate panels.
  const dropStaleCard = useCallback((id: number) => {
    closeDetail(id);
    dropFromSelection(id);
    invalidate();
  }, [closeDetail, dropFromSelection]);

  const confirmRemove = async () => {
    if (!removeTarget) return;
    const target = removeTarget;
    setRemoveTarget(null);
    dropFromSelection(target.id);
    try {
      await removeVideo(target.id);
      closeDetail(target.id);
      pushToast({ kind: "info", message: `已将“${videoDisplayName(target)}”移入回收站` });
      // The removed video may have belonged to groups, and its card has to
      // leave the grid, the sidebar counts and any open panel.
      invalidate();
    } catch (e) {
      // The list this card came from is evidently out of date: the backend
      // answers "already gone" as success, so only a stale card can fail here.
      // Follow the backend instead of leaving something the user cannot dismiss
      // (a removed duplicate, another window, the LAN client).
      dropStaleCard(target.id);
      pushToast({ kind: "error", message: "移除失败", details: [String(e)] });
    }
  };

  const confirmRemoveMissing = async () => {
    if (!missingOpen) return;
    const target = missingOpen;
    setMissingOpen(null);
    dropFromSelection(target.id);
    try {
      await removeVideo(target.id);
      closeDetail(target.id);
      pushToast({ kind: "info", message: `已将“${videoDisplayName(target)}”移入回收站` });
      invalidate();
    } catch (e) {
      // Same as confirmRemove: never leave behind a stale card that can only
      // fail again.
      dropStaleCard(target.id);
      pushToast({ kind: "error", message: "移除失败", details: [String(e)] });
    }
  };

  // ---- Group handlers ----

  const handleCreateGroup = async (name: string) => {
    setGroupDialog(null);
    try {
      await createGroup(name);
      pushToast({ kind: "success", message: `已创建分组“${name}”` });
      invalidate();
    } catch (e) {
      pushToast({ kind: "error", message: "创建分组失败", details: [String(e)] });
    }
  };

  const handleRenameGroup = async (name: string) => {
    const g = groupDialog?.mode === "rename" ? groupDialog.group : null;
    setGroupDialog(null);
    if (!g) return;
    try {
      await renameGroup(g.id, name);
      pushToast({ kind: "success", message: `已重命名为“${name}”` });
      invalidate();
    } catch (e) {
      pushToast({ kind: "error", message: "重命名分组失败", details: [String(e)] });
    }
  };

  const confirmDeleteGroup = async () => {
    if (!deleteGroupTarget) return;
    const target = deleteGroupTarget;
    setDeleteGroupTarget(null);
    try {
      await deleteGroup(target.id);
      pushToast({ kind: "info", message: `已删除分组“${target.name}”` });
      // If the active view was this group, fall back to All Favorites.
      setFilter((f) => (f.kind === "group" && f.id === target.id ? { kind: "all" } : f));
      invalidate();
    } catch (e) {
      pushToast({ kind: "error", message: "删除分组失败", details: [String(e)] });
    }
  };

  const handleReorderGroups = async (order: Group[]) => {
    groupsQuery.mutate(order); // optimistic UI
    try {
      await reorderGroups(order.map((g) => g.id));
    } catch (e) {
      pushToast({ kind: "error", message: "调整顺序失败", details: [String(e)] });
      groupsQuery.reload(); // restore authoritative order
    }
  };

  // Parent/child relation (drag nesting or context-menu flyout). ParentId null
  // detaches the group to the top level. Refreshes sidebar counts + the active
  // view (whose video set is transitive through the hierarchy).
  const handleSetGroupParent = useCallback(
    async (id: number, parentId: number | null) => {
      await setGroupParent(id, parentId);
      invalidate();
    },
    [],
  );

  // The sidebar right-click menu delegates rename/delete to the existing
  // dialogs; parent/child editing stays inside the flyout.
  const handleGroupMenuAction = (action: GroupMenuAction) => {
    const target = groupMenu?.group;
    if (!target) return;
    setGroupMenu(null);
    if (action.kind === "rename") setGroupDialog({ mode: "rename", group: target });
    else if (action.kind === "delete") setDeleteGroupTarget(target);
  };

  // Live-save: called on every group toggle from the context-menu flyout.
  // Membership persists immediately; the grid and sidebar counts refresh.
  const handleSaveGroups = useCallback(
    async (videoId: number, groupIds: number[]) => {
      await setVideoGroups(videoId, groupIds);
      invalidate();
    },
    [],
  );

  // Start a drag from a card. The shared helper carries either the single id
  // or the whole multi-selection and paints the custom drag ghost.
  const handleVideoDragStart = (e: React.DragEvent, video: Video) => {
    beginVideoDrag(e, video, selectedIds);
  };

  // Like a video: the backend increments its counter (repeated likes stack).
  // The click only writes; the live query re-reads the count.
  const handleLike = useCallback(
    (id: number) => {
      likeVideo(id)
        .then(() => invalidate())
        .catch((e) => pushToast({ kind: "error", message: "点赞失败", details: [String(e)] }));
    },
    [pushToast],
  );

  // Both views (grid/list) share the exact same data and interaction wiring.
  // Declared after handleVideoDragStart, which it references.
  const videoListProps = {
    videos: shownVideos,
    selectedIds,
    onToggleSelect: toggleSelect,
    onOpen: openDetail,
    onPlay: handleOpen,
    onContextMenu: (e: React.MouseEvent, v: Video) => {
      e.preventDefault();
      setMenu({ x: e.clientX, y: e.clientY, video: v });
    },
    onDragStart: handleVideoDragStart,
    onLike: handleLike,
    // Card width in px, applied by both the card and the row renderers.
    cardSize,
  };

  // Dropping a video onto a group adds it to that group (idempotent: already
  // grouped videos produce no side effect). A drop carries either a single
  // video or a multi-selected batch.
  const handleDropVideo = async (group: Group, videoIds: number[]) => {
    const batch = videoIds.length > 1;
    const n = videoIds.length;
    try {
      await Promise.all(videoIds.map((videoId) => addVideoToGroup(videoId, group.id)));
      if (batch) pushToast({ kind: "success", message: `已将 ${n} 个视频添加到“${group.name}”` });
      invalidate();
    } catch (e) {
      pushToast({ kind: "error", message: "添加视频到分组失败", details: [String(e)] });
    }
  };

  const activeGroupTitle =
    filter.kind === "group" ? groups.find((g) => g.id === filter.id)?.name : null;

  // The list's own heading. It is the one piece of the toolbar that says where
  // the reader is; the count beside it stays secondary text.
  const viewTitle = filter.kind === "group" ? (activeGroupTitle ?? "分组") : "全部收藏";
  const viewCountText = filtering
    ? `匹配 ${shownVideos.length} / 共 ${videos.length} 个视频`
    : `${videos.length} 个视频`;
  const showDiskBar = diskBarOptions.length > 1 || selectedDisks.size > 0;

  // The actor currently shown in an actor tab, if the active tab is one.
  const activeActorName = actorTabName(activeTab);

  // Both actor views own a keyword box that filters the list they show. The
  // header box is a whole-library video query and cannot act on either of them,
  // so it is hidden rather than left sitting above the view's own box as a
  // second, inert field that looks like it should filter what is on screen.
  // The actor index only owns the content when the library tab is active; an
  // open video/detail tab takes precedence over `filter`.
  const actorView =
    activeActorName != null || (activeTab === "library" && filter.kind === "actors");

  // 查重 owns its own keyword box too (it filters 番号 and file names, not the
  // library), so the header search is hidden for it in the same way.
  const duplicateView = activeTab === "library" && filter.kind === "duplicates";

  // 回收站 owns its own toolbar, so the library search box would be a second,
  // inert field above it.
  const trashView = activeTab === "library" && filter.kind === "trash";

  // What <main> currently shows. The content error boundary uses it to recover
  // when the user navigates somewhere else, so a broken panel cannot trap them
  // on a dead screen.
  const contentKey =
    activeActorName != null
      ? `actor:${activeActorName}`
      : activeTab !== "library"
        ? `video:${activeTab}`
        : `filter:${filter.kind}`;

  // Tab strip items: video tabs then actor tabs, each with its display label.
  // The library tab is rendered separately by WorkspaceTabs.
  const tabItems = useMemo<WorkspaceTabItem[]>(
    () => [
      ...detailTabs.map((video) => ({
        tab: video.id as WorkspaceTab,
        label: videoDisplayName(video),
      })),
      ...actorTabs.map((name) => ({ tab: actorTabID(name), label: name })),
    ],
    [detailTabs, actorTabs],
  );

  // Title shown in the tab context menu; the library tab is the home page.
  const tabMenuVideo = tabMenu && typeof tabMenu.tab === "number"
    ? detailTabs.find((tab) => tab.id === tabMenu.tab)
    : undefined;
  const tabMenuActor = tabMenu ? actorTabName(tabMenu.tab) : null;
  const tabMenuLabel = tabMenuActor ?? (tabMenuVideo
    ? videoDisplayName(tabMenuVideo)
    : "影片库");
  // The video shown in the active detail tab, if the active tab is a video tab.
  const activeVideoTab = typeof activeTab === "number"
    ? detailTabs.find((tab) => tab.id === activeTab)
    : undefined;

  return (
    <div className="flex h-screen flex-col">
      <header className="flex items-center gap-4 border-b border-[var(--border)] bg-[var(--panel)] px-5 py-3">
        <h1 className="m-0 flex items-center gap-2 text-[17px] font-semibold tracking-[0.2px]">
          <img className="h-12 w-12 rounded-xl object-cover" src={museLogo} alt="Muse Video" />
          <span className="whitespace-nowrap">Muse Video</span>
        </h1>
        <div className="flex items-center gap-1">
          <Button
            className="px-2"
            disabled={backStack.length === 0}
            onClick={goBack}
            title="后退 (Alt+←)"
            aria-label="后退"
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <Button
            className="px-2"
            disabled={forwardStack.length === 0}
            onClick={goForward}
            title="前进 (Alt+→)"
            aria-label="前进"
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
        <Button data-tour="add-video" variant="primary" onClick={() => setAddSourceOpen(true)}>
          + 添加视频
        </Button>
        <div className="flex-1" />
        <ServerBar />
        {!actorView && !duplicateView && !trashView && (
          <SearchBar
            value={keyword}
            onChange={setKeyword}
            inputRef={searchInputRef}
            placeholder={filter.kind === "scrapeTasks" ? "按文件名搜索任务" : "搜索视频"}
          />
        )}
        {/* The help entry leads to the 新手引导 page: one place that owns both
            the replay and the shortcut list, instead of a second overlay that
            would fork the tour's state. */}
        <Button
          className="w-8 px-0"
          onClick={() => onOpenSettings("guide")}
          title="帮助与新手引导"
          aria-label="打开帮助与新手引导"
        >
          ?
        </Button>
        <Button
          data-tour="settings-open"
          className=""
          onClick={() => onOpenSettings()}
          title="设置"
          aria-label="打开设置"
        >
          ⚙ 设置
        </Button>
      </header>

      <WorkspaceTabs
        tabs={tabItems}
        active={activeTab}
        onSelect={selectTab}
        onClose={(tab) => {
          const name = actorTabName(tab);
          if (name != null) closeActor(name);
          else closeDetail(tab as number);
        }}
        onContextMenu={(e, tab) => {
          e.preventDefault();
          e.stopPropagation();
          setTabMenu({ x: e.clientX, y: e.clientY, tab });
        }}
      />

      <div className="flex min-h-0 flex-1">
        <Sidebar
          groups={groups}
          filter={filter}
          onCreate={() => setGroupDialog({ mode: "create" })}
          onSelect={selectFilter}
          onRename={(g) => setGroupDialog({ mode: "rename", group: g })}
          onDelete={(g) => setDeleteGroupTarget(g)}
          onReorder={handleReorderGroups}
          onSetParent={handleSetGroupParent}
          onDropVideo={(group, videoIds) => void handleDropVideo(group, videoIds)}
          onGroupContextMenu={(e, g) => {
            e.preventDefault();
            e.stopPropagation();
            setGroupMenu({ x: e.clientX, y: e.clientY, group: g });
          }}
        />

        <main
            ref={mainRef}
            className={cn(
              "min-w-0 flex-1 overflow-y-auto px-5 pb-8 pt-4",
              filter.kind !== "scrapeTasks" && "pt-0",
            )}        >
          {/* Nothing outside this boundary (sidebar, tabs, dialogs, toasts)
              should disappear because one panel failed to render. */}
          <ErrorBoundary scope="当前页面" resetKey={contentKey} variant="region">
          {activeActorName != null ? (
            <ActorFilmsView
              name={activeActorName}
              onBack={() => selectFilter({ kind: "actors" })}
              onOpen={openDetail}
              onPlay={handleOpen}
              onContextMenu={(e, v) => {
                e.preventDefault();
                setMenu({ x: e.clientX, y: e.clientY, video: v });
              }}
              onNotify={(message, kind) => pushToast({ kind, message })}
            />
          ) : activeTab !== "library" ? (
            activeVideoTab ? (
              <VideoDetailPanel
                video={activeVideoTab}
                onOpenSettings={onOpenSettings}
                onVideoChanged={invalidate}
                onSelectActor={openActor}
                onPlay={() => void handleOpen(activeVideoTab)}
                onRevealPath={() => handleRevealPath(activeVideoTab)}
              />
            ) : null
          ) : filter.kind === "scrapeTasks" ? (
            <ScrapeTaskView
              status={filter.status}
              keyword={keyword}
              onOpen={openDetail}
              onStatusChange={selectScrapeStatus}
            />
          ) : filter.kind === "actors" ? (
            <ActorIndexView
              onOpenActor={openActor}
              onMerged={handleActorsMerged}
              onNotify={(message, kind) => pushToast({ kind, message })}
            />
          ) : filter.kind === "duplicates" ? (
            <DuplicateView
              onOpen={openDetail}
              onContextMenu={(e, v) => {
                e.preventDefault();
                setMenu({ x: e.clientX, y: e.clientY, video: v });
              }}
              onResolved={handleDuplicatesResolved}
              onNotify={(message, kind) => pushToast({ kind, message })}
            />
          ) : filter.kind === "trash" ? (
            <TrashView
              onNotify={pushToast}
              onOpenSettings={onOpenSettings}
            />
          ) : (
            <>
              <div
                data-tour="library-toolbar"
                className="sticky top-0 z-[5] -mx-5 mb-3 border-b border-[var(--border)] bg-[var(--bg)] px-5"
              >
                <div className="flex min-h-10 flex-wrap items-center gap-x-3 gap-y-1">
                  <h2 className="m-0 text-[15px] font-semibold tracking-[0.1px] text-[var(--text)]">
                    {viewTitle}
                  </h2>
                  <span className="text-xs text-[var(--muted)]">{viewCountText}</span>
                  <div className="ml-auto flex flex-wrap items-center gap-2">
                    <FileScanButton onNotify={pushToast} />
                    {filter.kind === "group" && (
                      <SearchBar
                        value={groupKeyword}
                        onChange={setGroupKeyword}
                        placeholder="筛选本组视频"
                      />
                    )}
                    {selectedIds.size > 0 && (
                      <span className="inline-flex items-center gap-2 rounded-full border border-[rgba(124,92,255,0.35)] bg-[rgba(124,92,255,0.12)] py-0.5 pl-3 pr-1.5">
                        <span className="whitespace-nowrap text-xs text-[var(--text)]">已选 {selectedIds.size} 个</span>
                        <Button size="sm" onClick={() => setSelectedIds(new Set())}>
                          取消选择
                        </Button>
                        <Button
                          size="sm"
                          variant="primary"
                          onClick={() => setGroupDialog({ mode: "create" })}
                          title="先创建一个分组，再把选中的视频拖进去"
                        >
                          + 新建分组
                        </Button>
                      </span>
                    )}
                    <ViewControls
                      view={view}
                      onViewChange={handleViewChange}
                      cardSize={cardSize}
                      onCardSizeChange={handleCardSizeChange}
                    />
                  </div>
                </div>
                {showDiskBar && (
                  <div className="mb-2 min-w-0 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                    <DiskFilterBar
                      options={diskBarOptions}
                      selected={selectedDiskKeys}
                      onToggle={toggleDisk}
                      onClear={clearDisks}
                      className="w-max"
                    />
                  </div>
                )}
                <div className="mb-2 min-w-0 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                  <SortBar
                    sortBy={sortBy}
                    sortDir={sortDir}
                    onChange={handleSortChange}
                    view={view}
                    onViewChange={handleViewChange}
                    cardSize={cardSize}
                    onCardSizeChange={handleCardSizeChange}
                    showViewControls={false}
                    className="mb-0 w-max"
                  />
                </div>
              </div>
              {loading ? (
                <div className="flex min-h-[320px] h-full flex-col items-center justify-center text-center text-[var(--muted)]">加载中…</div>
              ) : videos.length === 0 ? (
                <EmptyState keyword={keyword} onAdd={() => setAddSourceOpen(true)} />
              ) : shownVideos.length === 0 ? (
                <div className="flex h-full min-h-[320px] flex-col items-center justify-center text-center text-[var(--muted)]">
                  <div className="mb-1.5 text-xl text-[var(--text)]">无结果</div>
                  <div>没有符合当前筛选条件的视频</div>
                  {filterSummary && <div className="mt-1 text-xs">{filterSummary}</div>}
                  {filtering && (
                    <Button size="sm" className="mt-3" onClick={clearViewFilters}>
                      清除筛选
                    </Button>
                  )}
                </div>
              ) : view === "grid" ? (
                <VideoGrid {...videoListProps} />
              ) : (
                <VideoList {...videoListProps} />
              )}
            </>
          )}
          </ErrorBoundary>
        </main>
      </div>

      {dragActive && <DropOverlay />}

      <AddSourceDialog
        open={addSourceOpen}
        onPickLocal={() => void handleAddLocal()}
        onPickFolder={() => void handleAddFolder()}
        onPickCloud={handleAddCloud}
        cloud={{
          count: cloudQuery.data?.length ?? 0,
          onlineCount: cloudQuery.data?.filter((c) => c.online).length ?? 0,
          loading: cloudQuery.loading,
          error: cloudQuery.error,
        }}
        onClose={() => setAddSourceOpen(false)}
      />

      <CloudBrowserDialog
        open={cloudBrowserOpen}
        onClose={() => setCloudBrowserOpen(false)}
        onOpenSettings={() => onOpenSettings("cloud")}
        onAdded={handleCloudAdded}
      />

      {menu && (
        <ContextMenu
          x={menu.x}
          y={menu.y}
          video={menu.video}
          groups={groups}
          onAction={handleMenuAction}
          onSaveGroups={handleSaveGroups}
          onClose={() => setMenu(null)}
        />
      )}

      {groupMenu && (
        <GroupContextMenu
          x={groupMenu.x}
          y={groupMenu.y}
          group={groupMenu.group}
          groups={groups}
          onAction={handleGroupMenuAction}
          onSetParent={handleSetGroupParent}
          onClose={() => setGroupMenu(null)}
        />
      )}

      {tabMenu && (
        <TabContextMenu
          x={tabMenu.x}
          y={tabMenu.y}
          tab={tabMenu.tab}
          label={tabMenuLabel}
          tabCount={detailTabs.length + actorTabs.length}
          onAction={handleTabMenuAction}
          onClose={() => setTabMenu(null)}
        />
      )}

      {removeTarget && (
        <ConfirmDialog
          title="移入回收站"
          message={`确定将“${videoDisplayName(removeTarget)}”移入回收站吗？\n\n可随时从回收站恢复；原始视频文件不会被删除。`}
          confirmLabel="移入回收站"
          cancelLabel="取消"
          danger
          onConfirm={() => void confirmRemove()}
          onCancel={() => setRemoveTarget(null)}
        />
      )}

      {missingOpen && (
        <ConfirmDialog
          title="文件缺失"
          message={`找不到文件：\n${missingOpen.path}\n\n要将其移入回收站吗？`}
          confirmLabel="移入回收站"
          cancelLabel="保留"
          danger
          onConfirm={() => void confirmRemoveMissing()}
          onCancel={() => setMissingOpen(null)}
        />
      )}

      {editVideo && (
        <EditVideoDialog
          video={editVideo}
          onClose={() => setEditVideo(null)}
          onSaved={(v) => {
            // Keep the dialog's own copy current (clearing likes happens in
            // place); the grid, the sidebar and any open video tab follow the
            // write through the live query.
            setEditVideo(v);
            invalidate();
          }}
        />
      )}

      {groupDialog && (
        <GroupDialog
          title={groupDialog.mode === "create" ? "新建分组" : "重命名分组"}
          initialName={groupDialog.mode === "rename" ? groupDialog.group.name : ""}
          confirmLabel={groupDialog.mode === "create" ? "创建" : "重命名"}
          cancelLabel="取消"
          onConfirm={groupDialog.mode === "create" ? handleCreateGroup : handleRenameGroup}
          onCancel={() => setGroupDialog(null)}
        />
      )}

      {deleteGroupTarget && (
        <ConfirmDialog
          title="删除分组"
          message={`确定删除分组“${deleteGroupTarget.name}”吗？\n\n组内视频会被保留——仅删除该分组。其子分组会变为顶层分组。`}
          confirmLabel="删除分组"
          cancelLabel="取消"
          danger
          onConfirm={() => void confirmDeleteGroup()}
          onCancel={() => setDeleteGroupTarget(null)}
        />
      )}

      {suggestFiles && (
        <GroupSuggestDialog
          files={suggestFiles}
          onApplyGroups={handleApplySuggestedGroups}
          onClose={() => setSuggestFiles(null)}
        />
      )}

      {cloudDownload && (
        <CloudDownloadBar
          progress={cloudDownload}
          onCancel={() => {
            void cancelCloudDownload(cloudDownload.videoId);
          }}
          onDismiss={() => setCloudDownload(null)}
        />
      )}

      <Toasts toasts={toasts} onDismiss={dismissToast} />
    </div>
  );
}
