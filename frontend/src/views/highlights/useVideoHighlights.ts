import { useEffect } from "react";

import { useLiveQuery } from "../../hooks/useLiveQuery";
import { client } from "../../services/client";
import type { Highlight } from "../../types";
import { isHighlightPending } from "../../utils/highlightTime";

// Stable identity for the empty list, so a re-render does not look like a new answer.
const NO_HIGHLIGHTS: Highlight[] = [];

/** 手机轮询的间隔：一条 GIF 通常几秒内生成完，3 秒足够让"排队中"很快变成图。 */
const POLL_INTERVAL_MS = 3000;

/**
 * 一部影片的精彩时刻（详情页共享视图模型）。
 *
 * 桌面端靠后端事件：`video:highlight:updated` 已经接进 `services/events.ts` 的
 * 失效列表，`useLiveQuery` 会自己静默重读，所以桌面不传 `pollWhilePending`
 * （默认 `false`）——再叠一个定时器只会让同一次状态变化读两遍。
 *
 * 手机端没有推送：局域网服务只有只读 HTTP 端点，浏览器也收不到桌面端的 Wails
 * 事件，所以传 `true`。轮询只在真的存在排队/生成中的行时才建立；一旦没有 pending，
 * 定时器被清掉并且不再新建——不是建一个空转的 3 秒循环。
 */
export function useVideoHighlights(
  videoId: number,
  options: { pollWhilePending?: boolean } = {},
) {
  const { pollWhilePending = false } = options;

  const { data, loading, error, reload } = useLiveQuery(
    () => client.listVideoHighlights(videoId),
    `highlights:${videoId}`,
  );
  const items = data ?? NO_HIGHLIGHTS;
  const hasPending = items.some(isHighlightPending);

  useEffect(() => {
    // 依赖是布尔值而不是 items：一次"同状态"的重读（比如另一条变成 ready）不该
    // 重建定时器，把还没走完的 3 秒重新计时。
    if (!pollWhilePending || !hasPending) return;
    const timer = window.setInterval(reload, POLL_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [pollWhilePending, hasPending, reload]);

  return { items, loading, error, reload };
}
