import type { Video } from "../../types";
import { useSimilarVideos } from "../../views/similar/useSimilarVideos";
import { videoCoverURL } from "../../utils/format";
import { videoDisplayName } from "../../utils/videoName";
import { ClickableCard, Spinner, Text } from "../../ui/astryx";

/**
 * 详情页底部的「类似影片」横排（手机尺寸）。
 *
 * 与桌面端同一份取数（`useSimilarVideos`）与同一套排序，只是卡片更小、一屏能
 * 滑到的张数更少。没有可推荐的影片时不渲染这一段。
 */
export function SimilarVideos({
  videoId,
  onOpen,
}: {
  videoId: number;
  /** 打开一条推荐：手机端压栈到那条影片的详情。 */
  onOpen: (video: Video) => void;
}) {
  const { items, loading } = useSimilarVideos(videoId, 6);
  if (!loading && items.length === 0) return null;

  return (
    <section className="mt-6 border-t border-[var(--border)] pt-4">
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="text-sm font-semibold">类似影片</h3>
        {!loading && <span className="text-xs text-[var(--muted)]">{items.length} 部</span>}
      </div>

      {loading ? (
        <div className="flex min-h-[120px] items-center justify-center">
          <Spinner label="正在挑选类似影片…" />
        </div>
      ) : (
        <div className="mt-3 flex gap-2.5 overflow-x-auto pb-2">
          {items.map((item) => (
            <ClickableCard
              className="w-[112px] shrink-0 overflow-hidden"
              key={item.video.id}
              label={`打开 ${videoDisplayName(item.video)}`}
              onClick={() => onOpen(item.video)}
              padding={0}
            >
              <div className="aspect-[3/4] w-full bg-[#111114]">
                <img
                  alt=""
                  className="size-full object-cover"
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
