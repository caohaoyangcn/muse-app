import type { ComponentProps } from "react";
import type { ViewMode } from "../types";
import type { RecentPlayedGroup } from "../utils/recent";
import VideoGrid from "./VideoGrid";
import VideoList from "./VideoList";

type VideoItemProps = Omit<ComponentProps<typeof VideoGrid>, "videos">;

/**
 * The 最近播放 history, split into age groups. Each group renders the same
 * grid/list the library uses, so opening, playing, selecting, dragging and the
 * context menu behave identically; only the data set differs. Empty groups are
 * dropped by groupRecentlyPlayed, so a heading is never shown with zero rows.
 */
export default function RecentlyPlayedView({
  groups,
  view,
  ...itemProps
}: { groups: RecentPlayedGroup[]; view: ViewMode } & VideoItemProps) {
  return (
    <div className="flex flex-col gap-6 pb-2">
      {groups.map((group) => (
        <section key={group.key} aria-labelledby={`recent-group-${group.key}`}>
          <div className="mb-2 flex items-baseline gap-2 border-b border-[var(--border)] pb-1.5">
            <h3
              id={`recent-group-${group.key}`}
              className="m-0 text-sm font-semibold tracking-[0.1px] text-[var(--text)]"
            >
              {group.label}
            </h3>
            <span className="text-xs text-[var(--muted)]">{group.videos.length} 个视频</span>
          </div>
          {view === "grid" ? (
            <VideoGrid {...itemProps} videos={group.videos} />
          ) : (
            <VideoList {...itemProps} videos={group.videos} />
          )}
        </section>
      ))}
    </div>
  );
}
