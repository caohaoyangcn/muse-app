import { useMemo, useState } from "react";

import type { ActorSummary } from "../../types";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import { client } from "../../services/client";

// Stable identity for the empty index.
const NO_ACTORS: ActorSummary[] = [];

/**
 * 演员索引：按刮削结果聚合出的演员，附带出演数。
 *
 * 筛选同时匹配本名与已合并的艺名：合并之后用户仍然会按艺名找这个人，只匹配本名会
 * 让被合并掉的搜索词突然失效。
 */
export function useActorIndex() {
  const { data, loading, error, reload } = useLiveQuery(client.listActors, "actors");
  const actors = data ?? NO_ACTORS;
  const [keyword, setKeyword] = useState("");

  const filtered = useMemo(() => {
    const needle = keyword.trim().toLowerCase();
    if (!needle) return actors;
    return actors.filter(
      (a) =>
        a.name.toLowerCase().includes(needle) ||
        a.aliases?.some((alias) => alias.toLowerCase().includes(needle)),
    );
  }, [actors, keyword]);

  return { actors, filtered, keyword, setKeyword, loading, error, reload };
}
