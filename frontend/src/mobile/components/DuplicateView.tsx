import { useState } from "react";

import type { JavDuplicateGroup, Video } from "../../types";
import { coverURL } from "../../services/http";
import { useJavDuplicates } from "../../views/duplicates/useJavDuplicates";
import { videoMeta, videoName } from "../videoDisplay";
import { ContentDuplicateView } from "./ContentDuplicateView";
import { ConfirmSheet } from "./Sheet";
import { EmptyState, ErrorState, Loading } from "./States";
import { cn } from "../../lib/utils";

/**
 * 查重 (duplicates) on the phone holds two worklists that answer different
 * questions, so they share one bottom tab with a switch between them rather than
 * two tabs the user would have to tell apart by judgment:
 *
 * - 同番号多版本 — one work, several encodes. Grouped by 番号 on the server.
 * - 内容重复 — the same bytes stored twice. Grouped by content fingerprint.
 *
 * A pair may honestly appear in both; neither list is a subset of the other.
 */
export function DuplicateView({
  onOpenVideo,
  onToast,
}: {
  onOpenVideo: (v: Video) => void;
  onToast: (message: string, tone?: "info" | "error") => void;
}) {
  const [tab, setTab] = useState<"jav" | "content">("jav");
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-1 border-b border-[var(--border)] px-4">
        <button
          className={cn(
            "-mb-px border-b-2 px-3 py-2 text-sm",
            tab === "jav"
              ? "border-[var(--accent)] font-semibold"
              : "border-transparent text-[var(--muted)]",
          )}
          onClick={() => setTab("jav")}
          type="button"
        >
          同番号多版本
        </button>
        <button
          className={cn(
            "-mb-px border-b-2 px-3 py-2 text-sm",
            tab === "content"
              ? "border-[var(--accent)] font-semibold"
              : "border-transparent text-[var(--muted)]",
          )}
          onClick={() => setTab("content")}
          type="button"
        >
          内容重复
        </button>
      </div>
      {tab === "jav" ? (
        <JavDuplicatePanel onOpenVideo={onOpenVideo} onToast={onToast} />
      ) : (
        <ContentDuplicateView onOpenVideo={onOpenVideo} onToast={onToast} />
      )}
    </div>
  );
}

/**
 * 番号 worklist: the works the library holds more than one copy of, recognized by
 * 番号. Copies carrying different episode markers ("abc-123-1" / "abc-123-2") are
 * separate works — the backend owns those rules and never lists them here.
 *
 * Each work offers the same two outcomes as the desktop panel: keep one version
 * (the other records are removed; files are never touched) or keep them all.
 * Dismissed works stay reachable behind a switch so a decision can be revisited.
 */
function JavDuplicatePanel({
  onOpenVideo,
  onToast,
}: {
  onOpenVideo: (v: Video) => void;
  onToast: (message: string, tone?: "info" | "error") => void;
}) {
  const [pendingKeep, setPendingKeep] = useState<{ group: JavDuplicateGroup; keep: Video } | null>(null);

  const {
    groups,
    activeCount,
    loading,
    error,
    reload,
    includeIgnored,
    setIncludeIgnored,
    busy,
    keepVersion,
    keepAll,
  } = useJavDuplicates({
    notify: (notice) => onToast(notice.text, notice.kind === "error" ? "error" : "info"),
  });

  const confirmKeep = async () => {
    if (!pendingKeep) return;
    const { group, keep } = pendingKeep;
    setPendingKeep(null);
    await keepVersion(group, keep);
  };

  if (loading) return <Loading />;
  if (error) return <ErrorState message={error} onRetry={reload} />;

  return (
    <div className="flex min-h-0 flex-1 flex-col px-4">
      <div className="flex items-center gap-3 py-2">
        <span className="text-xs text-[var(--muted)]">
          {activeCount === 0 ? "没有重复作品" : `${activeCount} 部作品有多个版本`}
        </span>
        <label className="ml-auto flex items-center gap-1.5 text-xs text-[var(--muted)]">
          <input
            checked={includeIgnored}
            className="accent-[var(--accent)]"
            onChange={(e) => setIncludeIgnored(e.target.checked)}
            type="checkbox"
          />
          显示已忽略
        </label>
      </div>

      <div className="scroll-area min-h-0 flex-1 pb-4" data-scroll-key="main">
        {groups.length === 0 ? (
          <EmptyState
            hint="带 -1 / -2 上下集标记的文件视为不同作品"
            title="没有重复作品"
          />
        ) : (
          <div className="flex flex-col gap-3">
            {groups.map((group) => (
              <section
                className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)]"
                key={group.key}
              >
                <header className="flex items-center gap-2 border-b border-[var(--border)] px-3 py-2">
                  <span className="text-sm font-semibold">{group.code}</span>
                  {group.part ? (
                    <span className="text-xs text-[var(--muted)]">第 {group.part} 集</span>
                  ) : null}
                  <span className="text-xs text-[var(--muted)]">
                    {group.videos.length} 个版本
                  </span>
                  {group.ignored ? (
                    <span className="border border-[var(--border)] px-1.5 py-px text-[11px] text-[var(--muted)]">
                      已忽略
                    </span>
                  ) : null}
                  <button
                    className="ml-auto rounded-full border border-[var(--border)] px-3 py-1 text-xs text-[var(--muted)] active:bg-[var(--panel-2)]"
                    onClick={() => void keepAll(group)}
                    type="button"
                  >
                    {group.videos.length > 2 ? "保留全部" : "保留两者"}
                  </button>
                </header>
                <div className="flex flex-col divide-y divide-[var(--border)]">
                  {group.videos.map((v) => (
                    <div className="flex items-center gap-3 p-2" key={v.id}>
                      <button
                        className="flex min-w-0 flex-1 items-center gap-3 text-left"
                        onClick={() => onOpenVideo(v)}
                        type="button"
                      >
                        <img
                          alt=""
                          className={cn(
                            "h-14 w-24 shrink-0 rounded-md bg-[#111114] object-cover",
                            v.status === "missing" && !v.volumeOffline && "opacity-60",
                          )}
                          draggable={false}
                          loading="lazy"
                          src={coverURL(v)}
                        />
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-sm font-medium">
                            {videoName(v)}
                          </span>
                          <span className="mt-0.5 block truncate text-xs text-[var(--muted)]">
                            {videoMeta(v)}
                          </span>
                          {v.volumeOffline ? (
                            <span className="mt-0.5 block truncate text-[11px] text-[var(--danger)]">
                              ⚠ 磁盘未接入
                            </span>
                          ) : null}
                        </span>
                      </button>
                      <button
                        className="shrink-0 rounded-[var(--radius)] bg-[var(--accent)] px-3 py-2 text-xs font-medium text-white active:opacity-90"
                        disabled={busy}
                        onClick={() => setPendingKeep({ group, keep: v })}
                        type="button"
                      >
                        保留此版本
                      </button>
                    </div>
                  ))}
                </div>
              </section>
            ))}
          </div>
        )}
      </div>

      <ConfirmSheet
        busy={busy}
        confirmLabel="保留此版本，其余移入回收站"
        danger
        message={`将保留「${pendingKeep ? videoName(pendingKeep.keep) : ""}」，并把同一作品的其他 ${
          pendingKeep ? pendingKeep.group.videos.length - 1 : 0
        } 个版本。\n\n原始视频文件不会被删除；移入回收站后可恢复。`}
        onClose={() => setPendingKeep(null)}
        onConfirm={() => void confirmKeep()}
        open={pendingKeep !== null}
        title="保留这个版本？"
      />
    </div>
  );
}
