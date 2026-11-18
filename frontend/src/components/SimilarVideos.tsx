import type { Video } from "../types";
import { useSimilarVideos } from "../views/similar/useSimilarVideos";
import { videoCoverURL } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import { ClickableCard, Spinner, Text } from "../ui/astryx";

/**
 * 详情页底部的「类似影片」横排。
 *
 * 只负责展示：取数与状态在 `useSimilarVideos` 里（两端共用），打分在
 * `backend/similar` 里。没有可推荐的影片时整段不渲染——空的标题比没有这段更难看；
 * 加载中才占位，因为那是一次真实存在、马上会结束的等待。
 */
export default function SimilarVideos({
  videoId,
  onOpen,
}: {
  videoId: number;
  /** 打开一条推荐：桌面端把它作为一个新的详情标签页。 */
  onOpen: (video: Video) => void;
}) {
  const { items, loading } = useSimilarVideos(videoId);
  if (!loading && items.length === 0) return null;

  return (
    <section className="border-t border-[var(--border)] pt-5">
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="text-sm font-semibold">类似影片</h3>
        {!loading && <span className="text-xs text-[var(--muted)]">{items.length} 部</span>}
      </div>

      {loading ? (
        <div className="flex min-h-[140px] items-center justify-center">
          <Spinner label="正在挑选类似影片…" />
        </div>
      ) : (
        <div className="mt-3 flex gap-3 overflow-x-auto pb-2">
          {items.map((item) => (
            <ClickableCard
              className="w-[140px] shrink-0 overflow-hidden"
              key={item.video.id}
              label={`打开 ${videoDisplayName(item.video)}`}
              onClick={() => onOpen(item.video)}
              padding={0}
            >
              <div className="aspect-[3/4] w-full bg-[#111114]">
                {/* A wide packshot carries the front and back cover side by side;
                    a centre crop lands on the seam and shows half of each. Align
                    right so the poster shows the front cover, the same offset the
                    library cards use. */}
                <img
                  alt=""
                  className="size-full object-cover object-right"
                  draggable={false}
                  src={videoCoverURL(item.video)}
                />
              </div>
              <div className="px-2 pb-2 pt-1.5">
                <div className="min-w-0 text-xs font-medium">
                  <Text color="inherit" maxLines={2} type="inherit">
                    {videoDisplayName(item.video)}
                  </Text>
                </div>
                {item.reasons.length > 0 && (
                  <p className="mt-1 truncate text-[11px] text-[var(--muted)]">
                    {item.reasons.join(" · ")}
                  </p>
                )}
              </div>
            </ClickableCard>
          ))}
        </div>
      )}
    </section>
  );
}
