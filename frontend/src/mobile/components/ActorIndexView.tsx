import { useEffect, useRef } from "react";

import { actorAvatarURL } from "../../utils/format";
import { usePagedList } from "../../hooks/usePagedList";
import { Avatar, EmptyState, List, ListItem, Spinner } from "../../ui/astryx";
import { Button } from "../../ui/button";
import { Input } from "../../ui/input";
import { Pagination } from "../../ui/pagination";
import { useActorIndex } from "../../views/actors/useActorIndex";

/** One page of actor rows; the index can carry several hundred. */
const PAGE_SIZE = 50;

/**
 * The aggregated actor index. Merged stage names are folded into their
 * canonical entry, whose count already includes the aliases — shown as a hint
 * so the user can see which names were merged.
 */
export function ActorIndexView({ onSelect }: { onSelect: (name: string) => void }) {
  const { actors, filtered: shown, keyword: filter, setKeyword: setFilter, loading, error, reload } =
    useActorIndex();
  const listKey = `mobile:actors:${filter}`;
  const paging = usePagedList({ total: shown.length, listKey, pageSize: PAGE_SIZE });
  const pageActors = shown.slice(paging.startIndex, paging.endIndex);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const firstFilter = useRef(true);
  useEffect(() => {
    if (firstFilter.current) {
      firstFilter.current = false;
      return;
    }
    scrollRef.current?.scrollTo({ top: 0 });
  }, [filter]);
  const turnPage = (next: number) => {
    paging.goToPage(next);
    scrollRef.current?.scrollTo({ top: 0 });
  };

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
      <div className="scroll-area min-h-0 flex-1 pb-4" data-scroll-key="main" ref={scrollRef}>
        {shown.length === 0 ? (
          <EmptyState title={actors.length === 0 ? "暂无演员数据" : "没有匹配的演员"} />
        ) : (
          <>
            <List className="flex flex-col gap-2">
              {pageActors.map((actor) => {
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
            <Pagination
              page={paging.page}
              pageCount={paging.pageCount}
              total={shown.length}
              unit="位"
              onPageChange={turnPage}
            />
          </>
        )}
      </div>
    </div>
  );
}
