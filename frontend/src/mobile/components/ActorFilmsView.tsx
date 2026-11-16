import type { Video } from "../../types";
import { actorAvatarURL, actorProfileMeta } from "../../utils/format";
import { Icon } from "../../ui/icon";
import { VideoGrid } from "./VideoGrid";
import { useActorFilms } from "../../views/actors/useActorFilms";
import { EmptyState, Spinner } from "../../ui/astryx";
import { Button } from "../../ui/button";

/** One actor's film list, opened from the actor index or a detail chip. */
export function ActorFilmsView({
  name,
  onBack,
  onOpenVideo,
  onMenu,
}: {
  name: string;
  onBack: () => void;
  onOpenVideo: (v: Video) => void;
  onMenu: (v: Video) => void;
}) {
  // A removal made on this screen goes through the shell's confirm sheet; the
  // live query is what takes the card off the list.
  const {
    videos,
    visible: shown,
    profile,
    keyword: filter,
    setKeyword: setFilter,
    filtering,
    loading,
    error,
    reload,
  } = useActorFilms(name);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-2 border-b border-[var(--border)] px-2 py-2">
        <button
          aria-label="返回"
          className="flex size-8 shrink-0 items-center justify-center rounded-full text-[var(--muted)] active:bg-[var(--panel-2)]"
          onClick={onBack}
          type="button"
        >
          <Icon className="size-4" name="arrow-left" />
        </button>
        <div className="flex min-w-0 flex-1 items-center gap-2">
          {profile ? (
            <span className="h-9 w-9 shrink-0 overflow-hidden rounded-full border border-[var(--border)] bg-[var(--panel-2)]">
              {actorAvatarURL(profile) ? (
                <img
                  alt=""
                  className="h-full w-full object-cover"
                  loading="lazy"
                  src={actorAvatarURL(profile)}
                />
              ) : (
                <span className="flex h-full w-full items-center justify-center text-xs text-[var(--muted)]">
                  {name.slice(0, 1)}
                </span>
              )}
            </span>
          ) : null}
          <div className="flex min-w-0 flex-1 flex-col">
            <div className="truncate text-sm font-medium">{name}</div>
            {!loading && !error ? (
              <div className="truncate text-xs text-[var(--muted)]">
                {filtering
                  ? `匹配 ${shown.length} / 共 ${videos.length} 部`
                  : `${videos.length} 部`}
                {profile ? ` · ${actorProfileMeta(profile)}` : ""}
              </div>
            ) : null}
          </div>
        </div>
        <input
          className="w-[9.5rem] shrink-0 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)] px-2.5 py-1.5 text-sm outline-none focus:border-[var(--accent)]"
          onChange={(e) => setFilter(e.target.value)}
          placeholder="筛选影片"
          type="search"
          value={filter}
        />
      </div>
      <div className="scroll-area min-h-0 flex-1 px-3 pb-4 pt-2" data-scroll-key="main">
        {loading ? (
          <div className="flex items-center justify-center py-20"><Spinner size="lg" /></div>
        ) : error ? (
          <EmptyState actions={<Button onClick={reload} variant="primary">重试</Button>} description={error} title="加载失败" />
        ) : videos.length === 0 ? (
          <EmptyState title="该演员暂无作品" />
        ) : shown.length === 0 ? (
          <EmptyState description="换个关键词试试" title="没有匹配的影片" />
        ) : (
          <VideoGrid onMenu={onMenu} onOpen={onOpenVideo} videos={shown} />
        )}
      </div>
    </div>
  );
}
