import { useCallback, useEffect, useState } from "react";

import type { ContentDuplicateGroup, ContentDuplicateMember, ContentScanStatus, Video } from "../../types";
import {
  ApiError,
  confirmContentDuplicate,
  contentScanStatus,
  coverURL,
  ignoreContentDuplicate,
  listContentDuplicates,
  removeVideo,
  reverifyContentDuplicate,
  scanContentDuplicates,
} from "../api";
import { invalidate } from "../../services/live";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import { videoMeta, videoName } from "../videoDisplay";
import { ConfirmSheet } from "./Sheet";
import { EmptyState, ErrorState, Loading } from "./States";
import { cn } from "../../lib/utils";

// Stable identity for the empty worklist.
const NO_GROUPS: ContentDuplicateGroup[] = [];

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
  const [includeIgnored, setIncludeIgnored] = useState(false);
  const [scan, setScan] = useState<ContentScanStatus | null>(null);
  const [pendingKeep, setPendingKeep] = useState<{ group: ContentDuplicateGroup; keep: Video } | null>(
    null,
  );
  const [busy, setBusy] = useState(false);

  const { data, loading, error, reload } = useLiveQuery(
    () => listContentDuplicates(includeIgnored).then((list) => list ?? NO_GROUPS),
    `content-duplicates:${includeIgnored}`,
  );
  const groups = data ?? NO_GROUPS;

  useEffect(() => {
    void contentScanStatus().then(setScan).catch(() => {});
  }, []);

  // The pass runs on the desktop; the phone follows its progress by polling
  // while it is running, which survives a phone that slept through the events.
  useEffect(() => {
    if (!scan?.running) return;
    const timer = window.setInterval(() => {
      void contentScanStatus()
        .then((status) => {
          setScan(status);
          if (!status.running) reload();
        })
        .catch(() => {});
    }, 1500);
    return () => window.clearInterval(timer);
  }, [scan?.running, reload]);

  // A full-file confirmation can take minutes on a phone's connection to the
  // desktop; while one is running, the group's own state is the progress report.
  useEffect(() => {
    if (!groups.some((g) => g.confirming)) return;
    const timer = window.setInterval(() => reload(), 3000);
    return () => window.clearInterval(timer);
  }, [groups, reload]);

  const startScan = useCallback(async () => {
    try {
      setScan(await scanContentDuplicates());
      onToast("已开始扫描内容指纹");
    } catch (e) {
      onToast(e instanceof ApiError ? e.message : "无法开始扫描", "error");
    }
  }, [onToast]);

  const confirmKeep = useCallback(async () => {
    if (!pendingKeep) return;
    const { group, keep } = pendingKeep;
    setPendingKeep(null);
    const removed = group.members.map((m) => m.video).filter((v) => v.id !== keep.id);
    setBusy(true);
    try {
      for (const v of removed) await removeVideo(v.id);
      onToast(`已保留「${videoName(keep)}」，移入回收站 ${removed.length} 个重复拷贝`);
      invalidate();
    } catch (e) {
      onToast(e instanceof ApiError ? e.message : "移入回收站失败", "error");
      reload();
    } finally {
      setBusy(false);
    }
  }, [pendingKeep, onToast, reload]);

  const keepAll = useCallback(
    async (group: ContentDuplicateGroup) => {
      try {
        await ignoreContentDuplicate(group.hash);
        onToast("已保留全部拷贝");
        reload();
      } catch (e) {
        onToast(e instanceof ApiError ? e.message : "忽略重复提示失败", "error");
      }
    },
    [onToast, reload],
  );

  const reverify = useCallback(
    async (group: ContentDuplicateGroup) => {
      try {
        await reverifyContentDuplicate(group.hash);
        onToast("已重新读取内容指纹");
        window.setTimeout(reload, 800);
      } catch (e) {
        onToast(e instanceof ApiError ? e.message : "重新校验失败", "error");
      }
    },
    [onToast, reload],
  );

  const confirm = useCallback(
    async (group: ContentDuplicateGroup) => {
      try {
        await confirmContentDuplicate(group.hash);
        onToast("已开始完整校验，逐字节读取整份文件，大文件较慢");
        reload();
      } catch (e) {
        onToast(e instanceof ApiError ? e.message : "完整校验失败", "error");
      }
    },
    [onToast, reload],
  );

  if (loading) return <Loading />;
  if (error) return <ErrorState message={error} onRetry={reload} />;

  const activeCount = groups.filter((g) => !g.ignored).length;

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
                          onClick={() => void confirm(group)}
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

/**
 * What the pass is doing, with the running phase's own counters. The size phase
 * reads every favorite's size and the comparing phase fingerprints only the ones
 * whose size is shared, so each phase reports its own pair — the comparing total
 * does not exist until the size pass ends.
 */
function scanStatusText(status: ContentScanStatus | null): string {
  if (!status) return "尚未扫描过内容指纹";
  if (status.running) {
    if (status.phase === "examining") {
      return `正在检查文件大小… ${status.examined}/${status.examineTotal}`;
    }
    if (status.phase === "comparing") {
      return `正在比对内容指纹… ${status.done}/${status.total}`;
    }
    return "正在准备扫描…";
  }
  if (status.finishedAt > 0) {
    let text = `上次扫描：检查 ${status.examineTotal} 个文件`;
    text += status.total > 0 ? ` · 比对 ${status.total} 个` : " · 无需比对内容";
    if (status.failed > 0) text += ` · ${status.failed} 个无法读取`;
    return text;
  }
  return "尚未扫描过内容指纹";
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
