import { useState } from "react";

import type { CardSize, Video } from "../../types";
import { actorAvatarURL, actorProfileMeta } from "../../utils/format";
import { readCardSize } from "../prefs";
import { EmptyState, ErrorState, Loading } from "./States";
import { VideoGrid } from "./VideoGrid";
import { useActorFilms } from "../../views/actors/useActorFilms";

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
  // No size switcher on this screen, but it honours the library's choice so the
  // two grids match. Read once on mount: the view remounts on each navigation.
  const [cardSize] = useState<CardSize>(readCardSize);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-2 border-b border-[var(--border)] px-2 py-2">
        <button
          aria-label="返回"
          className="px-2 text-xl leading-none text-[var(--muted)]"
          onClick={onBack}
          type="button"
        >
          ←
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
      <div className="scroll-area min-h-0 flex-1 px-4 py-3" data-scroll-key="main">
        {loading ? (
          <Loading />
        ) : error ? (
          <ErrorState message={error} onRetry={reload} />
        ) : videos.length === 0 ? (
          <EmptyState title="该演员暂无作品" />
        ) : shown.length === 0 ? (
          <EmptyState hint="换个关键词试试" title="没有匹配的影片" />
        ) : (
          <VideoGrid cardSize={cardSize} onMenu={onMenu} onOpen={onOpenVideo} videos={shown} />
        )}
      </div>
    </div>
  );
}
