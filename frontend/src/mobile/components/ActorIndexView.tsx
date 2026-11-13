import { useState } from "react";

import type { ActorSummary } from "../../types";
import { actorAvatarURL } from "../../utils/format";
import { listActors } from "../../services/http";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import { EmptyState, ErrorState, Loading } from "./States";

// Stable identity for the empty index.
const NO_ACTORS: ActorSummary[] = [];

/**
 * The aggregated actor index. Merged stage names are folded into their
 * canonical entry, whose count already includes the aliases — shown as a hint
 * so the user can see which names were merged.
 */
export function ActorIndexView({ onSelect }: { onSelect: (name: string) => void }) {
  const { data, loading, error, reload } = useLiveQuery(listActors, "actors");
  const actors = data ?? NO_ACTORS;
  const [filter, setFilter] = useState("");

  const needle = filter.trim().toLowerCase();
  const shown = needle
    ? actors.filter(
        (a) =>
          a.name.toLowerCase().includes(needle) ||
          a.aliases?.some((alias) => alias.toLowerCase().includes(needle)),
      )
    : actors;

  if (loading) return <Loading />;
  if (error) return <ErrorState message={error} onRetry={reload} />;

  return (
    <div className="flex min-h-0 flex-1 flex-col px-4">
      <div className="py-2">
        <input
          className="w-full rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)] px-3 py-2.5 outline-none focus:border-[var(--accent)]"
          onChange={(e) => setFilter(e.target.value)}
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
          <div className="flex flex-col gap-2">
            {shown.map((a) => (
              <button
                className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)] px-3 py-3 text-left active:scale-[0.99]"
                key={a.name}
                onClick={() => onSelect(a.name)}
                type="button"
              >
                <div className="flex items-center gap-2.5">
                  <ActorAvatar actor={a} />
                  <span className="min-w-0 flex-1 truncate font-medium">{a.name}</span>
                  <span className="shrink-0 text-xs text-[var(--muted)]">{a.count} 部</span>
                </div>
                {a.aliases && a.aliases.length > 0 ? (
                  <div className="mt-1 truncate text-xs text-[var(--muted)]">
                    已合并：{a.aliases.join("、")}
                  </div>
                ) : null}
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

/** 演员头像；没有档案时用名字首字兜底，避免列表出现空洞。 */
function ActorAvatar({ actor }: { actor: ActorSummary }) {
  const src = actorAvatarURL(actor.profile);
  return (
    <span className="h-10 w-10 shrink-0 overflow-hidden rounded-full border border-[var(--border)] bg-[var(--panel-2)]">
      {src ? (
        <img alt="" className="h-full w-full object-cover" loading="lazy" src={src} />
      ) : (
        <span className="flex h-full w-full items-center justify-center text-sm text-[var(--muted)]">
          {actor.name.slice(0, 1)}
        </span>
      )}
    </span>
  );
}
