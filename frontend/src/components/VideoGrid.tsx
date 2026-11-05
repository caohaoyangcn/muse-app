import type { CardSize, Video } from "../types";
import { useVirtualRows } from "../hooks/useVirtualRows";
import { scaleBetween } from "../utils/cardSize";
import { DESKTOP_CARD_SIZE } from "../utils/viewPrefs";
import VideoCard from "./VideoCard";

/**
 * Fixed-width card grid. The slider sets the exact track width, so every step
 * changes the cards instead of only reflowing columns (a `minmax(..., 1fr)`
 * track would make the rendered width a function of the column count). The
 * leftover width is centred; at the default size it is only a few tens of
 * pixels.
 */
export default function VideoGrid({
  videos,
  selectedIds,
  onToggleSelect,
  onOpen,
  onPlay,
  onContextMenu,
  onDragStart,
  onLike,
  cardSize,
}: {
  videos: Video[];
  selectedIds: Set<number>;
  onToggleSelect: (id: number) => void;
  onOpen: (video: Video) => void;
  onPlay: (video: Video) => void;
  onContextMenu: (e: React.MouseEvent, video: Video) => void;
  onDragStart: (e: React.DragEvent, video: Video) => void;
  onLike: (id: number) => void;
  cardSize: CardSize;
}) {
  // Keyboard navigation selects items that may sit far outside the viewport, so
  // the single selected card is the one the window has to keep reachable.
  const selected = selectedIds.size === 1 ? [...selectedIds][0] : null;
  const revealIndex = selected == null ? null : videos.findIndex((v) => v.id === selected);

  const virtual = useVirtualRows({
    count: videos.length,
    minColumnWidth: cardSize,
    measureKey: cardSize,
    revealIndex: revealIndex != null && revealIndex >= 0 ? revealIndex : null,
  });
  const { startIndex, endIndex, columns, windowed, offsetTop, totalHeight } = virtual;
  // The gap grows a little with the cards; the virtualizer measures the
  // computed value from the DOM, so it never drifts from the layout.
  const gap = Math.round(scaleBetween(cardSize, DESKTOP_CARD_SIZE, 12, 20));

  return (
    <div
      ref={virtual.containerRef}
      className="relative"
      style={windowed ? { height: totalHeight } : undefined}
    >
      <div
        ref={virtual.windowRef}
        className="grid"
        // An explicit track list keeps the rendered rows aligned with the
        // window's row maths; the count still follows the container width.
        // `min(..., 100%)` is a safety net when the window is narrower than
        // the chosen card size.
        style={{
          gridTemplateColumns: `repeat(${columns > 0 ? columns : "auto-fill"}, min(${cardSize}px, 100%))`,
          justifyContent: "center",
          columnGap: gap,
          rowGap: gap,
          ...(windowed ? { position: "absolute", top: offsetTop, left: 0, right: 0 } : null),
        }}
      >
        {videos.slice(startIndex, endIndex).map((v) => (
          <VideoCard
            key={v.id}
            video={v}
            selected={selectedIds.has(v.id)}
            onToggleSelect={onToggleSelect}
            onOpen={onOpen}
            onPlay={onPlay}
            onContextMenu={onContextMenu}
            onDragStart={onDragStart}
            onLike={onLike}
          />
        ))}
      </div>
    </div>
  );
}
