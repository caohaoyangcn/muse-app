import type { Video } from "../../types";
import { actorAvatarURL, actorProfileMeta } from "../../utils/format";
import { Icon } from "../../ui/icon";
import { VideoGrid } from "./VideoGrid";
import type { VideoMenuActions } from "../videoMenu";
import { useActorFilms } from "../../views/actors/useActorFilms";
import { Avatar, EmptyState, IconButton, Spinner } from "../../ui/astryx";
import { Button } from "../../ui/button";
import { Input } from "../../ui/input";

/** One actor's film list, opened from the actor index or a detail chip. */
export function ActorFilmsView({
  name,
  onBack,
  onOpenVideo,
  actions,
  openMenuId,
  onMenuOpenChange,
}: {
  name: string;
  onBack: () => void;
  onOpenVideo: (v: Video) => void;
  actions: VideoMenuActions;
  openMenuId: number | null;
  onMenuOpenChange: (video: Video, open: boolean) => void;
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
        <IconButton
          className="size-8 shrink-0 rounded-full text-[var(--muted)]"
          icon={<Icon className="size-4" name="arrow-left" />}
          label="返回"
          onClick={onBack}
          size="sm"
          tooltip="返回"
          variant="ghost"
        />
        <div className="flex min-w-0 flex-1 items-center gap-2">
          {profile ? (
            <Avatar name={name} size="md" src={actorAvatarURL(profile) || undefined} />
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
        {/* The field itself is Astryx's 100%-wide control, so the width belongs
            on a wrapper: sizing the control would let the field claim the whole
            row and push the avatar underneath it. */}
        <div className="w-[9.5rem] shrink-0">
          <Input
            aria-label="筛选影片"
            onChange={(e) => setFilter(e.target.value)}
            placeholder="筛选影片"
            type="search"
            value={filter}
          />
        </div>
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
          <VideoGrid
            actions={actions}
            memoryKey={`actor:${name}:${filter}`}
            onMenuOpenChange={onMenuOpenChange}
            onOpen={onOpenVideo}
            openMenuId={openMenuId}
            videos={shown}
          />
        )}
      </div>
    </div>
  );
}
