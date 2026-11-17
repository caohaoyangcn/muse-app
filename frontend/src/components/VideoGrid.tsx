import { useRef } from "react";
import type { CardSize, Video } from "../types";
import { usePagedList } from "../hooks/usePagedList";
import { useRevealInPage } from "../hooks/useRevealInPage";
import { scaleBetween } from "../utils/cardSize";
import { DESKTOP_CARD_SIZE } from "../utils/viewPrefs";
import { Grid } from "../ui/astryx";
import { Pagination } from "../ui/pagination";
import VideoCard from "./VideoCard";

/** One page is about two screens of cards at the default size, whether the
 *  wall or the compact list is showing. Both views share the size so switching
 *  between them keeps the same items on screen. */
const PAGE_SIZE = 60;

/**
 * Fixed-width card grid, one page at a time.
 *
 * The slider sets the exact track width, so every step changes the cards
 * instead of only reflowing columns; the leftover width is centred. The track
 * list is left to CSS (`repeat(auto-fill, min(...))`) — no layout measurement,
 * which is what used to leave the list stuck after its starter rows.
 *
 * `memoryKey` identifies the list being paged (filters, sort, keyword), so a
 * return from a detail tab lands on the page the user left.
 */
export default function VideoGrid({
  videos,
  memoryKey,
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
  memoryKey: string;
  selectedIds: Set<number>;
  onToggleSelect: (id: number) => void;
  onOpen: (video: Video) => void;
  onPlay: (video: Video) => void;
  onContextMenu: (e: React.MouseEvent, video: Video) => void;
  onDragStart: (e: React.DragEvent, video: Video) => void;
  onLike: (id: number) => void;
  cardSize: CardSize;
}) {
  // Keyboard navigation selects items that may sit on another page, so the
  // single selected card is what the paging hook has to follow.
  const selected = selectedIds.size === 1 ? [...selectedIds][0] : null;
  const revealIndex = selected == null ? null : videos.findIndex((v) => v.id === selected);
  const listRef = useRef<HTMLDivElement | null>(null);

  const paging = usePagedList({
    total: videos.length,
    listKey: memoryKey,
    pageSize: PAGE_SIZE,
    revealIndex: revealIndex != null && revealIndex >= 0 ? revealIndex : null,
  });
  useRevealInPage({
    listRef,
    items: videos,
    startIndex: paging.startIndex,
    endIndex: paging.endIndex,
    revealIndex: revealIndex != null && revealIndex >= 0 ? revealIndex : null,
  });

  // The gap grows a little with the cards and is mirrored into the inline
  // track list, so the centred tracks and the spacing stay in step.
  const gap = Math.round(scaleBetween(cardSize, DESKTOP_CARD_SIZE, 12, 20));

  const turnPage = (step: () => void) => {
    step();
    // The sticky toolbar sits over the top of the pane; the wrapper's
    // scroll-margin keeps the first row visible below it.
    listRef.current?.scrollIntoView({ block: "start", inline: "nearest" });
  };

  return (
    <div className="scroll-mt-28" ref={listRef}>
      <Grid
        gap={2}
        style={{
          gridTemplateColumns: `repeat(auto-fill, min(${cardSize}px, 100%))`,
          justifyContent: "center",
          columnGap: gap,
          rowGap: gap,
        }}
      >
        {videos.slice(paging.startIndex, paging.endIndex).map((v) => (
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
      </Grid>
      <Pagination
        page={paging.page}
        pageCount={paging.pageCount}
        total={videos.length}
        unit="部"
        onPrev={() => turnPage(paging.prevPage)}
        onNext={() => turnPage(paging.nextPage)}
      />
    </div>
  );
}
