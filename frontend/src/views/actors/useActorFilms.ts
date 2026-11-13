import { useEffect, useMemo, useState } from "react";

import type { Video } from "../../types";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import { client } from "../../services/client";
import { filterVideosByKeyword } from "../../utils/videoFilter";

// Stable identity for the empty list.
const NO_VIDEOS: Video[] = [];

/**
 * 一位演员的作品列表与档案。
 *
 * 档案的来源两平台不同（桌面有专用绑定，浏览器从演员索引里取），但都经
 * `client.actorProfile`，所以视图只管"有没有档案"。
 *
 * 筛选状态按演员名重置：同一个组件实例会在切换演员时被复用，上一个演员的关键词不能
 * 跟进下一个演员的列表。
 */
export function useActorFilms(name: string) {
  const { data, loading, error, reload } = useLiveQuery(
    () => client.videosByActor(name),
    `actor:${name}`,
  );
  // 档案读不到不影响作品列表，所以不做错误处理。
  const { data: profile } = useLiveQuery(
    () => client.actorProfile(name),
    `actorProfile:${name}`,
  );
  const videos = data ?? NO_VIDEOS;
  const [keyword, setKeyword] = useState("");

  useEffect(() => {
    setKeyword("");
  }, [name]);

  const visible = useMemo(() => filterVideosByKeyword(videos, keyword), [videos, keyword]);

  return {
    videos,
    visible,
    profile: profile ?? null,
    keyword,
    setKeyword,
    filtering: keyword.trim() !== "",
    loading,
    error,
    reload,
  };
}
