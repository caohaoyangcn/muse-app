import { useLiveQuery } from "../../hooks/useLiveQuery";
import { client } from "../../services/client";
import type { SimilarVideo } from "../../types";

// Stable identity for the empty row.
const NO_SIMILAR: SimilarVideo[] = [];

/**
 * 详情页底部的「类似影片」。
 *
 * 推荐是详情页的附属信息：读失败时这一栏静默消失，不占用错误提示，也不影响
 * 页面上其余数据的可用性——所以这里不暴露 error，只把结果归一成"有"或"没有"。
 *
 * `limit` 参与查询键：换一个条数就是换一个问题，不会复用上一批结果。条数本身
 * 是两端的展示选择（手机一排放得下几张），后端只负责夹取上限。
 */
export function useSimilarVideos(videoId: number, limit = 8) {
  const { data, loading } = useLiveQuery(
    () => client.similarVideos(videoId, limit),
    `similar:${videoId}:${limit}`,
  );
  return { items: data ?? NO_SIMILAR, loading };
}
