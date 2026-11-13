import { useState } from "react";

import type { ContentDuplicateGroup, ContentDuplicateMember, Video } from "../../types";
import { coverURL } from "../../services/http";
import {
  scanStatusText,
  useContentDuplicates,
} from "../../views/duplicates/useContentDuplicates";
import { videoMeta, videoName } from "../videoDisplay";
import { ConfirmSheet } from "../../ui/sheet";
import { EmptyState, ErrorState, Loading } from "./States";
import { cn } from "../../lib/utils";

/**
 * 内容重复 on the phone: the favorites that hold byte-identical files, grouped by
 * the fingerprint the server computed from each file's size plus its first and
 * last megabyte. It answers a different question than the 番号 worklist beside it
 * — that one groups one work's encodes, this one groups the same bytes stored
 * twice — so a pair can appear in both.
 *
 * The phone cannot reveal a folder in the server's file manager, so "keep this
 * copy" only removes the other records. Deleting the redundant file itself stays
 * a desktop action; the sheet says so rather than implying the space is freed.
 */
export function ContentDuplicateView({
  onOpenVideo,
  onToast,
}: {
  onOpenVideo: (v: Video) => void;
  onToast: (message: string, tone?: "info" | "error") => void;
}) {
  const [pendingKeep, setPendingKeep] = useState<{ group: ContentDuplicateGroup; keep: Video } | null>(
    null,
  );

  const {
    groups,
    activeCount,
    loading,
    error,
    reload,
    includeIgnored,
    setIncludeIgnored,
    scan,
    busy,
    startScan,
    keepCopy,
    keepAll,
    reverify,
    confirmGroup,
  } = useContentDuplicates({
    notify: (notice) => onToast(notice.text, notice.kind === "error" ? "error" : "info"),
  });

  const confirmKeep = async () => {
    if (!pendingKeep) return;
    const { group, keep } = pendingKeep;
    setPendingKeep(null);
    await keepCopy(group, keep);
  };

  if (loading) return <Loading />;
  if (error) return <ErrorState message={error} onRetry={reload} />;

  return (
    <div className="flex min-h-0 flex-1 flex-col px-4">
      <div className="flex flex-wrap items-center gap-3 py-2">
        <span className="text-xs text-[var(--muted)]">
          {activeCount === 0 ? "没有内容重复" : `${activeCount} 组文件内容完全相同`}
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

      <div className="mb-2 flex items-center gap-2 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-xs text-[var(--muted)]">
        <span className="min-w-0 flex-1">{scanStatusText(scan)}</span>
        <button
          className="shrink-0 rounded-full border border-[var(--border)] px-3 py-1 active:bg-[var(--panel-2)] disabled:opacity-50"
          disabled={scan?.running ?? false}
          onClick={() => void startScan()}
          type="button"
        >
          重新扫描
        </button>
      </div>

      <div className="scroll-area min-h-0 flex-1 pb-4" data-scroll-key="main">
        {groups.length === 0 ? (
          <EmptyState
            hint="只有内容完全相同的两个文件才会出现在这里"
            title="没有内容重复"
          />
        ) : (
          <div className="flex flex-col gap-3">
            {groups.map((group) => (
              <section
                className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)]"
                key={group.hash}
              >
                <header className="flex items-center gap-2 border-b border-[var(--border)] px-3 py-2">
                  <span className="text-sm font-semibold">{group.members.length} 份相同内容</span>
                  <span className="truncate text-xs text-[var(--muted)]">
                    可省 {formatBytes(group.reclaimableBytes)}
                  </span>
                  {group.ignored ? (
                    <span className="border border-[var(--border)] px-1.5 py-px text-[11px] text-[var(--muted)]">
                      已忽略
                    </span>
                  ) : null}
                  {group.needsReverify ? (
                    <button
                      className="ml-auto shrink-0 rounded-full border border-[var(--border)] px-3 py-1 text-xs text-[var(--muted)] active:bg-[var(--panel-2)]"
                      onClick={() => void reverify(group)}
                      type="button"
                    >
                      重新校验
                    </button>
                  ) : (
                    <span className="ml-auto flex shrink-0 items-center gap-1.5">
                      {!group.confirmed && !group.refuted ? (
                        <button
                          className="rounded-full border border-[var(--border)] px-3 py-1 text-xs text-[var(--muted)] active:bg-[var(--panel-2)] disabled:opacity-50"
                          disabled={group.confirming}
                          onClick={() => void confirmGroup(group)}
                          type="button"
                        >
                          {group.confirming ? "校验中…" : "完整校验"}
                        </button>
                      ) : null}
                      <button
                        className="rounded-full border border-[var(--border)] px-3 py-1 text-xs text-[var(--muted)] active:bg-[var(--panel-2)] disabled:opacity-50"
                        disabled={group.refuted || groupDetached(group)}
                        onClick={() => void keepAll(group)}
                        type="button"
                      >
                        {group.members.length > 2 ? "保留全部" : "保留两份"}
                      </button>
                    </span>
                  )}
                </header>
                {group.needsReverify ? (
                  <div className="border-b border-[var(--border)] px-3 py-2 text-xs text-[var(--danger)]">
                    ⚠ 有成员的文件在应用之外发生了变化，重新校验完成前不能据此移除记录。
                  </div>
                ) : null}
                {groupDetached(group) ? (
                  <div className="border-b border-[var(--border)] px-3 py-2 text-xs text-[var(--muted)]">
                    ⚠ 有拷贝所在的磁盘未接入；插回后再整理，避免移除还能找回的收藏。
                  </div>
                ) : null}
                {group.refuted ? (
                  <div className="border-b border-[var(--border)] px-3 py-2 text-xs text-[var(--danger)]">
                    ⚠ 完整校验表明这些文件内容并不相同（首尾与大小恰好一致），请勿据此删除任何文件。
                  </div>
                ) : null}
                <div className="flex flex-col divide-y divide-[var(--border)]">
                  {group.members.map((member) => (
                    <div className="flex items-center gap-3 p-2" key={member.video.id}>
                      <button
                        className="flex min-w-0 flex-1 items-center gap-3 text-left"
                        onClick={() => onOpenVideo(member.video)}
                        type="button"
                      >
                        <img
                          alt=""
                          className={cn(
                            "h-14 w-24 shrink-0 rounded-md bg-[#111114] object-cover",
                            member.missing && "opacity-60",
                          )}
                          draggable={false}
                          loading="lazy"
                          src={coverURL(member.video)}
                        />
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-sm font-medium">
                            {videoName(member.video)}
                          </span>
                          <span className="mt-0.5 block truncate text-xs text-[var(--muted)]">
                            {videoMeta(member.video)}
                          </span>
                          <span className="mt-0.5 block truncate text-[11px] text-[var(--muted)]">
                            {memberBadge(member)}
                          </span>
                        </span>
                      </button>
                      <button
                        className="shrink-0 rounded-[var(--radius)] bg-[var(--accent)] px-3 py-2 text-xs font-medium text-white active:opacity-90 disabled:opacity-50"
                        disabled={busy || group.needsReverify || group.refuted || groupDetached(group)}
                        onClick={() => setPendingKeep({ group, keep: member.video })}
                        type="button"
                      >
                        保留此份
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
        confirmLabel="保留此拷贝，其余移入回收站"
        danger
        message={`将保留「${pendingKeep ? videoName(pendingKeep.keep) : ""}」，并把内容相同的其他 ${
          pendingKeep ? pendingKeep.group.members.length - 1 : 0
        } 个拷贝。\n\n原始视频文件不会被删除；移入回收站后可恢复。释放磁盘空间需要在电脑上删除多余文件。`}
        onClose={() => setPendingKeep(null)}
        onConfirm={() => void confirmKeep()}
        open={pendingKeep !== null}
        title="保留这个拷贝？"
      />
    </div>
  );
}

/** One compact line describing the member beyond its metadata. */
function memberBadge(member: ContentDuplicateMember): string {
  if (member.volumeOffline) return "⚠ 磁盘未接入（插回后再整理）";
  if (member.missing) return "⚠ 文件缺失（记录可移除）";
  if (member.needsReverify) return "⚠ 内容已变化，需重新校验";
  return member.video.path;
}

/** A group with a member on an unplugged drive must not be cleaned up yet. */
function groupDetached(group: ContentDuplicateGroup): boolean {
  return group.members.some((member) => member.volumeOffline);
}

/** Byte sizes as the phone shows them (no dependency on the desktop formatter). */
function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 10 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}
