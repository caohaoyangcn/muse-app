import { useEffect, useRef, useState } from "react";

import type { JavDuplicateGroup, Video } from "../../types";
import { coverURL } from "../../services/web";
import { usePagedList } from "../../hooks/usePagedList";
import { useJavDuplicates } from "../../views/duplicates/useJavDuplicates";
import { videoMeta, videoName } from "../videoDisplay";
import { ContentDuplicateView } from "./ContentDuplicateView";
import { ConfirmSheet } from "../../ui/sheet";
import {
  Badge,
  CheckboxInput,
  EmptyState,
  List,
  ListItem,
  SegmentedControl,
  SegmentedControlItem,
  Spinner,
} from "../../ui/astryx";
import { Button } from "../../ui/button";
import { CoverImage } from "../../ui/cover";
import { Pagination } from "../../ui/pagination";
import { cn } from "../../lib/utils";

/** One page of duplicate works; each work can hold several versions. */
const GROUP_PAGE_SIZE = 20;

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
      <div className="border-b border-[var(--border)] px-4 py-2">
        <SegmentedControl
          className="w-full"
          label="查重方式"
          onChange={(value) => setTab(value as "jav" | "content")}
          value={tab}
        >
          <SegmentedControlItem label="同番号多版本" value="jav" />
          <SegmentedControlItem label="内容重复" value="content" />
        </SegmentedControl>
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

  const listKey = `mobile:dups:jav:${includeIgnored}`;
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
    await keepVersion(group, keep);
  };

  if (loading) return <div className="flex items-center justify-center py-20"><Spinner size="lg" /></div>;
  if (error) return <EmptyState actions={<Button onClick={reload} variant="primary">重试</Button>} description={error} title="加载失败" />;

  return (
    <div className="flex min-h-0 flex-1 flex-col px-4">
      <div className="flex items-center gap-3 py-2">
        <span className="text-xs text-[var(--muted)]">
          {activeCount === 0 ? "没有重复作品" : `${activeCount} 部作品有多个版本`}
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

      <div className="scroll-area min-h-0 flex-1 pb-4" data-scroll-key="main" ref={scrollRef}>
        {groups.length === 0 ? (
          <EmptyState
            description="带 -1 / -2 上下集标记的文件视为不同作品"
            title="没有重复作品"
          />
        ) : (
          <>
            <div className="flex flex-col gap-3">
              {pageGroups.map((group) => (
              <section
                className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)]"
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
                  {group.ignored ? <Badge label="已忽略" variant="neutral" /> : null}
                  <Button
                    className="ml-auto"
                    onClick={() => void keepAll(group)}
                    size="sm"
                    variant="ghost"
                  >
                    {group.videos.length > 2 ? "保留全部" : "保留两者"}
                  </Button>
                </header>
                <List className="flex flex-col">
                  {group.videos.map((v) => (
                    <ListItem
                      className={cn(
                        "border-b border-[var(--border)] last:border-b-0",
                        v.status === "missing" && !v.volumeOffline && "opacity-60",
                      )}
                      description={
                        <>
                          <span>{videoMeta(v)}</span>
                          {v.volumeOffline ? (
                            <span className="mt-0.5 block text-[var(--danger)]">磁盘未接入</span>
                          ) : null}
                        </>
                      }
                      endContent={
                        <Button
                          disabled={busy}
                          onClick={() => setPendingKeep({ group, keep: v })}
                          size="sm"
                          variant="primary"
                        >
                          保留此版本
                        </Button>
                      }
                      key={v.id}
                      label={videoName(v)}
                      onClick={() => onOpenVideo(v)}
                      startContent={
                        <CoverImage
                          className="h-14 w-24 shrink-0 rounded-md bg-[#111114]"
                          loading="lazy"
                          src={coverURL(v)}
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
