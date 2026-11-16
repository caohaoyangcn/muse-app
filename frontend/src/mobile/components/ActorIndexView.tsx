import { actorAvatarURL } from "../../utils/format";
import { Avatar, EmptyState, List, ListItem, Spinner } from "../../ui/astryx";
import { Button } from "../../ui/button";
import { Input } from "../../ui/input";
import { useActorIndex } from "../../views/actors/useActorIndex";

/**
 * The aggregated actor index. Merged stage names are folded into their
 * canonical entry, whose count already includes the aliases — shown as a hint
 * so the user can see which names were merged.
 */
export function ActorIndexView({ onSelect }: { onSelect: (name: string) => void }) {
  const { actors, filtered: shown, keyword: filter, setKeyword: setFilter, loading, error, reload } =
    useActorIndex();

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <Spinner size="lg" />
      </div>
    );
  }
  if (error) {
    return (
      <EmptyState
        actions={
          <Button onClick={reload} variant="default">
            重试
          </Button>
        }
        description={error}
        title="加载失败"
      />
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col px-4">
      <div className="py-2">
        <Input
          aria-label="筛选演员"
          onChange={(event) => setFilter(event.target.value)}
          placeholder="筛选演员"
          type="search"
          value={filter}
        />
      </div>
      <div className="pb-1 text-xs text-[var(--muted)]">{shown.length} 位演员</div>
      <div className="scroll-area min-h-0 flex-1 pb-4" data-scroll-key="main">
        {shown.length === 0 ? (
          <EmptyState title={actors.length === 0 ? "暂无演员数据" : "没有匹配的演员"} />
        ) : (
          <List className="flex flex-col gap-2">
            {shown.map((actor) => {
              const src = actorAvatarURL(actor.profile);
              return (
                <ListItem
                  className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)]"
                  description={actor.aliases && actor.aliases.length > 0 ? `已合并：${actor.aliases.join("、")}` : undefined}
                  endContent={<span className="text-xs text-[var(--muted)]">{actor.count} 部</span>}
                  key={actor.name}
                  label={actor.name}
                  onClick={() => onSelect(actor.name)}
                  startContent={<Avatar name={actor.name} size="md" src={src || undefined} />}
                />
              );
            })}
          </List>
        )}
      </div>
    </div>
  );
}
