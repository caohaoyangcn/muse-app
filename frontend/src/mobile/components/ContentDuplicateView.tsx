import { useEffect, useRef, useState } from "react";

import type { ContentDuplicateGroup, ContentDuplicateMember, Video } from "../../types";
import { coverURL } from "../../services/web";
import { usePagedList } from "../../hooks/usePagedList";
import {
  scanStatusText,
  useContentDuplicates,
} from "../../views/duplicates/useContentDuplicates";
import { videoMeta, videoName } from "../videoDisplay";
import { ConfirmSheet } from "../../ui/sheet";
import { cn } from "../../lib/utils";
import { Badge, Banner, CheckboxInput, EmptyState, List, ListItem, Spinner } from "../../ui/astryx";
import { Button } from "../../ui/button";
import { Pagination } from "../../ui/pagination";

/** One page of duplicate content groups; each group can hold several copies. */
const GROUP_PAGE_SIZE = 20;

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

  const listKey = `mobile:dups:content:${includeIgnored}`;
  const paging = usePagedList({ total: groups.length, listKey, pageSize: GROUP_PAGE_SIZE });
  const pageGroups = groups.slice(paging.startIndex, paging.endIndex);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const firstListKey = useRef(true);
  useEffect(() => {
    if (firstListKey.current) {
      firstListKey.current = false;
      return;
    }
    scrollRef.current?.scrollTo({ top: 0 });
  }, [listKey]);
  const turnPage = (next: number) => {
    paging.goToPage(next);
    scrollRef.current?.scrollTo({ top: 0 });
  };

  const confirmKeep = async () => {
    if (!pendingKeep) return;
    const { group, keep } = pendingKeep;
    setPendingKeep(null);
    await keepCopy(group, keep);
  };

  if (loading) return <div className="flex items-center justify-center py-20"><Spinner size="lg" /></div>;
  if (error) return <EmptyState actions={<Button onClick={reload} variant="primary">重试</Button>} description={error} title="加载失败" />;

  return (
    <div className="flex min-h-0 flex-1 flex-col px-4">
      <div className="flex flex-wrap items-center gap-3 py-2">
        <span className="text-xs text-[var(--muted)]">
          {activeCount === 0 ? "没有内容重复" : `${activeCount} 组文件内容完全相同`}
        </span>
        <span className="ml-auto">
          <CheckboxInput
            label="显示已忽略"
            onChange={setIncludeIgnored}
            size="sm"
            value={includeIgnored}
          />
        </span>
      </div>

      <div className="mb-2 flex items-center gap-2 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-xs text-[var(--muted)]">
        <span className="min-w-0 flex-1">{scanStatusText(scan)}</span>
        <Button
          disabled={scan?.running ?? false}
          onClick={() => void startScan()}
          size="sm"
          variant="ghost"
        >
          重新扫描
        </Button>
      </div>

      <div className="scroll-area min-h-0 flex-1 pb-4" data-scroll-key="main" ref={scrollRef}>
        {groups.length === 0 ? (
          <EmptyState
            description="只有内容完全相同的两个文件才会出现在这里"
            title="没有内容重复"
          />
        ) : (
          <>
            <div className="flex flex-col gap-3">
              {pageGroups.map((group) => (
              <section
                className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)]"
                key={group.hash}
              >
                <header className="flex items-center gap-2 border-b border-[var(--border)] px-3 py-2">
                  <span className="text-sm font-semibold">{group.members.length} 份相同内容</span>
                  <span className="truncate text-xs text-[var(--muted)]">
                    可省 {formatBytes(group.reclaimableBytes)}
                  </span>
                  {group.ignored ? <Badge label="已忽略" variant="neutral" /> : null}
                  {group.needsReverify ? (
                    <Button
                      className="ml-auto shrink-0"
                      onClick={() => void reverify(group)}
                      size="sm"
                      variant="ghost"
                    >
                      重新校验
                    </Button>
                  ) : (
                    <span className="ml-auto flex shrink-0 items-center gap-1.5">
                      {!group.confirmed && !group.refuted ? (
                        <Button
                          disabled={group.confirming}
                          onClick={() => void confirmGroup(group)}
                          size="sm"
                          variant="ghost"
                        >
                          {group.confirming ? "校验中…" : "完整校验"}
                        </Button>
                      ) : null}
                      <Button
                        disabled={group.refuted || groupDetached(group)}
                        onClick={() => void keepAll(group)}
                        size="sm"
                        variant="ghost"
                      >
                        {group.members.length > 2 ? "保留全部" : "保留两份"}
                      </Button>
                    </span>
                  )}
                </header>
                {group.needsReverify ? (
                  <Banner status="error" title="有成员的文件在应用之外发生了变化，重新校验完成前不能据此移除记录。" />
                ) : null}
                {groupDetached(group) ? (
                  <Banner status="warning" title="有拷贝所在的磁盘未接入；插回后再整理，避免移除还能找回的收藏。" />
                ) : null}
                {group.refuted ? (
                  <Banner status="error" title="完整校验表明这些文件内容并不相同（首尾与大小恰好一致），请勿据此删除任何文件。" />
                ) : null}
                <List className="flex flex-col">
                  {group.members.map((member) => (
                    <ListItem
                      className={cn("border-b border-[var(--border)] last:border-b-0", member.missing && "opacity-60")}
                      description={
                        <>
                          <span>{videoMeta(member.video)}</span>
                          <span className="mt-0.5 block truncate">{memberBadge(member)}</span>
                        </>
                      }
                      endContent={
                        <Button
                          disabled={busy || group.needsReverify || group.refuted || groupDetached(group)}
                          onClick={() => setPendingKeep({ group, keep: member.video })}
                          size="sm"
                          variant="primary"
                        >
                          保留此份
                        </Button>
                      }
                      key={member.video.id}
                      label={videoName(member.video)}
                      onClick={() => onOpenVideo(member.video)}
                      startContent={
                        <img
                          alt=""
                          className="h-14 w-24 shrink-0 rounded-md bg-[#111114] object-cover object-right"
                          draggable={false}
                          loading="lazy"
                          src={coverURL(member.video)}
                        />
                      }
                    />
                  ))}
                </List>
              </section>
              ))}
            </div>
            <Pagination
              page={paging.page}
              pageCount={paging.pageCount}
              total={groups.length}
              unit="组"
              onPageChange={turnPage}
            />
          </>
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
