import { useCallback, useEffect, useMemo, useState } from "react";
import { likeVideo } from "../services/api";
import { invalidate } from "../services/live";
import type { ActorProfile, CardSize, SortDir, SortKey, Video, ViewMode } from "../types";
import { beginVideoDrag } from "../utils/dragGhost";
import { actorAvatarURL, actorProfileMeta as profileMeta } from "../utils/format";
import { compareLastOpened } from "../utils/sortVideos";
import { readCardSize, readViewMode, writeCardSize, writeViewMode } from "../utils/viewPrefs";
import SearchBar from "./SearchBar";
import SortBar from "./SortBar";
import { StickyBar } from "../ui/sticky-bar";
import VideoGrid from "./VideoGrid";
import VideoList from "./VideoList";
import { useActorFilms } from "../views/actors/useActorFilms";

/**
 * One actor's film list, rendered as its own workspace tab. It reuses the same
 * card/row views as the library, so opening, playing, liking and drag-select
 * all behave identically; only the data source differs (one actor's merged
 * cast). Sorting is local, mirroring the library grid.
 *
 * The list is a live query: a favorite removed from this tab, or edited in
 * another view, leaves the screen without this component knowing which write or
 * event caused it.
 */
export default function ActorFilmsView({
  name,
  onBack,
  onOpen,
  onPlay,
  onContextMenu,
  onNotify,
}: {
  name: string;
  /** Return to the aggregated actor index (the tab strip keeps this tab open). */
  onBack: () => void;
  onOpen: (video: Video) => void;
  onPlay: (video: Video) => void;
  onContextMenu: (e: React.MouseEvent, video: Video) => void;
  onNotify: (message: string, kind: "success" | "error") => void;
}) {
  const { videos, visible: visibleVideos, profile, keyword, setKeyword, filtering, loading, error } =
    useActorFilms(name);
  const [sortBy, setSortBy] = useState<SortKey>("createdAt");
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const [view, setView] = useState<ViewMode>(readViewMode);
  const [cardSize, setCardSize] = useState<CardSize>(readCardSize);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());

  // A removal takes its card off the list, so a selection it was part of must
  // not linger as a phantom highlight.
  useEffect(() => {
    setSelectedIds((prev) => {
      if (prev.size === 0) return prev;
      const alive = new Set(videos.map((v) => v.id));
      const next = new Set([...prev].filter((id) => alive.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [videos]);

  const nameCollator = useMemo(
    () => new Intl.Collator(undefined, { numeric: true, sensitivity: "base" }),
    [],
  );
  const sortedVideos = useMemo(() => {
    const list = [...visibleVideos];
    const dir = sortDir === "asc" ? 1 : -1;
    list.sort((a, b) => {
      if (sortBy === "lastOpenedAt") return compareLastOpened(a, b, sortDir);
      let c: number;
      if (sortBy === "fileName") c = nameCollator.compare(a.fileName, b.fileName);
      else if (sortBy === "fileSize") c = a.fileSize - b.fileSize;
      else if (sortBy === "likeCount") c = a.likeCount - b.likeCount || a.id - b.id;
      else c = a.createdAt - b.createdAt || a.id - b.id;
      return c * dir;
    });
    return list;
  }, [visibleVideos, sortBy, sortDir, nameCollator]);


  const toggleSelect = useCallback((id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  // The like count is part of this list, so the click only writes; the live
  // query re-reads and the card follows.
  const handleLike = useCallback(
    (id: number) => {
      likeVideo(id)
        .then(() => invalidate())
        .catch((e) => onNotify(`点赞失败：${String(e)}`, "error"));
    },
    [onNotify],
  );

  const handleViewChange = useCallback((v: ViewMode) => {
    setView(v);
    writeViewMode(v);
  }, []);

  const handleCardSizeChange = useCallback((s: CardSize) => {
    setCardSize(s);
    writeCardSize(s);
  }, []);

  const videoListProps = {
    videos: sortedVideos,
    selectedIds,
    onToggleSelect: toggleSelect,
    onOpen,
    onPlay,
    onContextMenu,
    onDragStart: (e: React.DragEvent, v: Video) => beginVideoDrag(e, v, selectedIds),
    onLike: handleLike,
    cardSize,
  };

  return (
    <div>
      <StickyBar>
        <button
          type="button"
          className="mr-1 underline-offset-2 hover:text-[var(--accent)] hover:underline"
          onClick={onBack}
          title="返回演员列表"
        >
          ← 演员
        </button>
        <span>
          「{name}」 —{" "}
          {loading
            ? "…"
            : filtering
              ? `匹配 ${sortedVideos.length} / 共 ${videos.length} 个视频`
              : `${videos.length} 个视频`}
        </span>
        <div className="ml-auto">
          <SearchBar value={keyword} onChange={setKeyword} placeholder="筛选该演员影片" />
        </div>
      </StickyBar>
      {profile && <ActorProfileHeader profile={profile} />}
      {loading ? (
        <div className="flex min-h-[320px] items-center justify-center text-[var(--muted)]">
          加载中…
        </div>
      ) : error ? (
        <div className="flex min-h-[320px] items-center justify-center text-[var(--danger)]">
          加载失败：{error}
        </div>
      ) : videos.length === 0 ? (
        <div className="flex min-h-[320px] flex-col items-center justify-center text-center text-[var(--muted)]">
          <div className="mb-1.5 text-xl text-[var(--text)]">暂无影片</div>
          <div>未找到「{name}」出演的收藏影片</div>
        </div>
      ) : sortedVideos.length === 0 ? (
        <div className="flex min-h-[320px] flex-col items-center justify-center text-center text-[var(--muted)]">
          <div className="mb-1.5 text-xl text-[var(--text)]">无结果</div>
          <div>没有匹配“{keyword.trim()}”的影片</div>
        </div>
      ) : (
        <>
          <SortBar
            sortBy={sortBy}
            sortDir={sortDir}
            onChange={(key, dir) => {
              setSortBy(key);
              setSortDir(dir);
            }}
            view={view}
            onViewChange={handleViewChange}
            cardSize={cardSize}
            onCardSizeChange={handleCardSizeChange}
          />
          {view === "grid" ? <VideoGrid {...videoListProps} /> : <VideoList {...videoListProps} />}
        </>
      )}
    </div>
  );
}

/**
 * 演员档案头：头像 + 生日/身高/三围/出演数。数据来自刮削，可能不完整，所以每一项
 * 都单独判断——缺项直接不显示，而不是留一个空标签。
 */
function ActorProfileHeader({ profile }: { profile: ActorProfile }) {
  const avatar = actorAvatarURL(profile);
  const meta = profileMeta(profile);
  return (
    <div className="mb-4 flex items-start gap-3">
      <span className="h-16 w-16 shrink-0 overflow-hidden rounded-full border border-[var(--border)] bg-[var(--panel-2)]">
        {avatar ? (
          <img src={avatar} alt="" className="h-full w-full object-cover" />
        ) : (
          <span className="flex h-full w-full items-center justify-center text-lg text-[var(--muted)]">
            {profile.name.slice(0, 1)}
          </span>
        )}
      </span>
      <div className="min-w-0">
        <div className="text-sm font-semibold">{profile.name}</div>
        {meta && <div className="mt-0.5 text-xs text-[var(--muted)]">{meta}</div>}
        {profile.aliases && profile.aliases.length > 0 && (
          <div className="mt-0.5 truncate text-xs text-[var(--muted)]">
            数据源别名：{profile.aliases.join("、")}
          </div>
        )}
      </div>
    </div>
  );
}
