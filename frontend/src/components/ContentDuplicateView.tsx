import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { EventsOn } from "../../wailsjs/runtime";
import type { ContentDuplicateGroup, ContentDuplicateMember, ContentScanStatus, Video } from "../types";
import { usePagedList } from "../hooks/usePagedList";
import { formatDuration, formatFileSize, videoCoverURL, videoResolution } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import {
  AspectRatio,
  Badge,
  Banner,
  CheckboxInput,
  ClickableCard,
  EmptyState,
  List,
  ListItem,
  ProgressBar,
  Spinner,
} from "../ui/astryx";
import { Button } from "../ui/button";
import { Icon } from "../ui/icon";
import { Pagination } from "../ui/pagination";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
} from "../ui/dialog";
import { Input } from "../ui/input";
import {
  filterContentGroups,
  scanProgress,
  scanStatusText,
  useContentDuplicates,
  type ConfirmProgress,
} from "../views/duplicates/useContentDuplicates";

/** One page of duplicate content groups; each group can carry several copies. */
const GROUP_PAGE_SIZE = 20;

/**
 * 内容重复 panel: the favorites that hold byte-identical files, grouped by the
 * content fingerprint the backend computed from each file's size plus its first
 * and last megabyte.
 *
 * This is a different question from the 番号 worklist next to it. That one groups
 * the different encodes of one work; this one groups the same bytes stored twice
 * under another name or on another drive. A pair can honestly appear in both.
 *
 * The same two outcomes apply: keep one copy (the other records are removed, the
 * files themselves are never touched) or keep them all, which records the
 * decision until the membership changes. A member whose file changed under the
 * app is flagged, and the group cannot be acted on until it is re-read — the
 * whole point of the feature is that the verdict is about the bytes on disk now.
 */
export default function ContentDuplicatePanel({
  onOpen,
  onContextMenu,
  onResolved,
  onNotify,
}: {
  /** Open a copy's detail — the same action as clicking a library card. */
  onOpen: (video: Video) => void;
  /** The library card menu (open / reveal / copy path / edit / remove). */
  onContextMenu: (e: React.MouseEvent, video: Video) => void;
  /** Called after copies are removed, with their ids, so the shell refreshes. */
  onResolved: (removedIds: number[]) => void;
  onNotify: (message: string, kind: "success" | "error") => void;
}) {
  const [keyword, setKeyword] = useState("");
  const [pendingKeep, setPendingKeep] = useState<{ group: ContentDuplicateGroup; keep: Video } | null>(
    null,
  );

  // 逐字节校验的进度只有桌面有事件通道；浏览器那边靠 group.confirming 与重读。
  const subscribeConfirmProgress = useCallback(
    (onProgress: (progress: ConfirmProgress | null) => void) => {
      const offProgress = EventsOn(
        "fingerprint:confirm:progress",
        (data?: { hash?: string; done?: number; total?: number }) => {
          if (!data?.hash || typeof data.done !== "number" || typeof data.total !== "number") return;
          onProgress({ hash: data.hash, done: data.done, total: data.total });
        },
      );
      const offDone = EventsOn("fingerprint:confirm:done", () => onProgress(null));
      return () => {
        offProgress();
        offDone();
      };
    },
    [],
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
    confirmProgress,
    busy,
    startScan,
    keepCopy,
    keepAll,
    reverify,
    confirmGroup,
  } = useContentDuplicates({
    subscribeConfirmProgress,
    onResolved,
    notify: (notice) => onNotify(notice.text, notice.kind === "error" ? "error" : "success"),
  });

  const filtered = useMemo(() => filterContentGroups(groups, keyword), [groups, keyword]);
  const listKey = `desktop:dups:content:${includeIgnored}:${keyword}`;
  const paging = usePagedList({ total: filtered.length, listKey, pageSize: GROUP_PAGE_SIZE });
  const pageGroups = filtered.slice(paging.startIndex, paging.endIndex);
  const listTopRef = useRef<HTMLDivElement | null>(null);
  const firstListKey = useRef(true);
  useEffect(() => {
    if (firstListKey.current) {
      firstListKey.current = false;
      return;
    }
    listTopRef.current?.scrollIntoView({ block: "start" });
  }, [listKey]);
  const turnPage = (step: () => void) => {
    step();
    listTopRef.current?.scrollIntoView({ block: "start" });
  };

  const confirmKeep = async () => {
    if (!pendingKeep) return;
    const { group, keep } = pendingKeep;
    setPendingKeep(null);
    await keepCopy(group, keep);
  };

  return (
    <>
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <span className="text-xs text-[var(--muted)]">
          {activeCount === 0 ? "没有内容重复" : `${activeCount} 组文件内容完全相同`}
          {includeIgnored && groups.length !== activeCount
            ? ` · 另有 ${groups.length - activeCount} 组已忽略`
            : ""}
        </span>
        <CheckboxInput
          label="显示已忽略"
          onChange={setIncludeIgnored}
          size="sm"
          value={includeIgnored}
        />
        <div className="ml-auto w-[240px] max-w-full">
          <Input
            aria-label="筛选文件名或指纹"
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            placeholder="筛选文件名或指纹"
            autoComplete="off"
            spellCheck={false}
          />
        </div>
      </div>

      <ScanBar status={scan} onScan={() => void startScan()} />

      <div className="scroll-mt-4" ref={listTopRef} />

      {error ? (
        <EmptyState
          actions={<Button onClick={reload} size="sm" variant="primary">重试</Button>}
          description={error}
          title="加载内容重复结果失败"
        />
      ) : loading ? (
        <div className="flex min-h-[280px] items-center justify-center">
          <Spinner label="加载中…" size="lg" />
        </div>
      ) : groups.length === 0 ? (
        <EmptyState
          description="只有内容完全相同的两个文件才会出现在这里；同一作品的不同压制不会被判为重复。若尚未扫描过，点上方「重新扫描」建立内容指纹；它也会为没有指纹的收藏补算。"
          title="没有内容重复"
        />
      ) : filtered.length === 0 ? (
        <EmptyState description={`没有匹配“${keyword.trim()}”的重复内容`} title="无结果" />
      ) : (
        <div className="flex flex-col gap-4 pb-8">
          {pageGroups.map((group) => (
            <section
              key={group.hash}
              className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)]"
            >
              <header className="flex flex-wrap items-center gap-2 border-b border-[var(--border)] px-3 py-2">
                <span className="font-semibold">{group.members.length} 份相同内容</span>
                <Badge
                  className="font-mono"
                  label={group.hash.slice(0, 8)}
                  variant="neutral"
                />
                <span className="text-xs text-[var(--muted)]">
                  占用 {formatFileSize(group.totalBytes)} · 保留一份可省{" "}
                  {formatFileSize(group.reclaimableBytes)}
                </span>
                {group.ignored ? <Badge label="已忽略" variant="neutral" /> : null}
                {group.confirmed ? <Badge label="已完整校验" variant="success" /> : null}
                {group.refuted ? <Badge label="完整校验不一致" variant="error" /> : null}
                <GroupActions
                  group={group}
                  busy={busy}
                  keepBlocked={group.members.some((member) => member.volumeOffline)}
                  progress={confirmProgress?.hash === group.hash ? confirmProgress : null}
                  onReverify={() => void reverify(group)}
                  onConfirm={() => void confirmGroup(group)}
                  onKeepAll={() => void keepAll(group)}
                />
              </header>
              {group.members.some((member) => member.volumeOffline) ? (
                <Banner status="warning" title="有拷贝所在的磁盘未接入；插回后再整理，避免把还能找回的收藏移入回收站。" />
              ) : null}
              {group.needsReverify ? (
                <Banner status="error" title="有成员的文件在应用之外发生了变化，其指纹已不能代表当前内容。重新校验完成前不能据此移除文件。" />
              ) : null}
              {group.refuted ? (
                <Banner status="error" title="完整校验表明这些文件的内容并不相同（首尾与大小恰好一致）。请勿据此删除任何文件；可先「重新扫描」更新它们的指纹。" />
              ) : null}
              <div className="flex flex-wrap gap-3 p-3">
                {group.members.map((member) => (
                  <MemberCard
                    key={member.video.id}
                    member={member}
                    busy={busy || group.needsReverify || group.refuted || group.members.some((m) => m.volumeOffline)}
                    onOpen={() => onOpen(member.video)}
                    onContextMenu={(e) => onContextMenu(e, member.video)}
                    onKeep={() => setPendingKeep({ group, keep: member.video })}
                  />
                ))}
              </div>
            </section>
          ))}
        </div>
      )}

      <Pagination
        page={paging.page}
        pageCount={paging.pageCount}
        total={filtered.length}
        unit="组"
        onPrev={() => turnPage(paging.prevPage)}
        onNext={() => turnPage(paging.nextPage)}
      />

      {pendingKeep && (
        <KeepCopyDialog
          keep={pendingKeep.keep}
          others={pendingKeep.group.members
            .map((m) => m.video)
            .filter((v) => v.id !== pendingKeep.keep.id)}
          onCancel={() => setPendingKeep(null)}
          onConfirm={() => void confirmKeep()}
        />
      )}
    </>
  );
}

/** The actions a group offers, which depend on how far its verdict has got. */
function GroupActions({
  group,
  busy,
  keepBlocked,
  progress,
  onReverify,
  onConfirm,
  onKeepAll,
}: {
  group: ContentDuplicateGroup;
  busy: boolean;
  /** A member's disk is not attached: its record must not be cleaned up now. */
  keepBlocked: boolean;
  /** Progress of this group's confirmation, when one is running. */
  progress: { done: number; total: number } | null;
  onReverify: () => void;
  onConfirm: () => void;
  onKeepAll: () => void;
}) {
  const confirming = group.confirming || progress !== null;
  const confirmLabel =
    progress && progress.total > 0 ? `校验中 ${progress.done}/${progress.total}` : "完整校验中…";
  return (
    <div className="ml-auto flex items-center gap-2">
      {group.needsReverify ? (
        <Button
          size="sm"
          disabled={busy}
          title="有成员在应用之外被改动，重新读取内容指纹后才能据其取舍"
          onClick={onReverify}
        >
          重新校验
        </Button>
      ) : (
        <>
          {!group.confirmed && !group.refuted && (
            <Button
              size="sm"
              disabled={busy || confirming}
              title="逐字节比较整份文件，确认首尾采样没有看错（大文件较慢）"
              onClick={onConfirm}
            >
              {confirming ? confirmLabel : "完整校验"}
            </Button>
          )}
          <Button
            size="sm"
            disabled={busy || group.refuted || keepBlocked}
            title={
              keepBlocked
                ? "有拷贝所在的磁盘未接入，插回后再整理"
                : "保留全部拷贝，并不再提示这组文件（等价于忽略重复提示）"
            }
            onClick={onKeepAll}
          >
            {group.members.length > 2 ? "保留全部" : "保留两份"}
          </Button>
        </>
      )}
    </div>
  );
}

/** The fingerprint scan's state, with the action that starts one. */
function ScanBar({ status, onScan }: { status: ContentScanStatus | null; onScan: () => void }) {
  const running = status?.running ?? false;
  const progress = scanProgress(status);
  return (
    <div className="mb-4 flex flex-wrap items-center gap-2 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-xs text-[var(--muted)]">
      <span>{scanStatusText(status)}</span>
      {progress && progress.total > 0 ? (
        <ProgressBar
          className="w-[160px]"
          isLabelHidden
          label="内容指纹扫描进度"
          max={progress.total}
          value={progress.done}
        />
      ) : null}
      <Button size="sm" className="ml-auto" disabled={running} onClick={onScan}>
        重新扫描
      </Button>
    </div>
  );
}

/** One copy of duplicated content, with the action that keeps it alone. */
function MemberCard({
  member,
  busy,
  onOpen,
  onContextMenu,
  onKeep,
}: {
  member: ContentDuplicateMember;
  busy: boolean;
  onOpen: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
  onKeep: () => void;
}) {
  const { video } = member;
  const meta = [formatDuration(video.durationMs), videoResolution(video), formatFileSize(video.fileSize)]
    .filter(Boolean)
    .join(" · ");
  return (
    <ClickableCard
      className="overflow-hidden"
      label={videoDisplayName(video)}
      maxWidth="100%"
      onClick={onOpen}
      onContextMenu={onContextMenu}
      padding={0}
      width={220}
    >
      <AspectRatio ratio={16 / 9}>
        <div className="relative size-full bg-[#111114]">
          {!member.missing && video.thumbnailPath ? (
            <img className="block size-full object-contain" draggable={false} src={videoCoverURL(video)} alt="" />
          ) : (
            <div className="flex size-full items-center justify-center opacity-50">
              <Icon className="size-6" name="alert" />
            </div>
          )}
          {member.missing ? <Badge className="absolute left-1.5 top-1.5" label="文件缺失" variant="error" /> : null}
          {member.volumeOffline ? (
            <Badge className="absolute left-1.5 top-1.5" label="磁盘未接入" variant="error" />
          ) : null}
          {member.needsReverify && !member.missing ? (
            <Badge className="absolute left-1.5 top-1.5" label="内容已变化" variant="warning" />
          ) : null}
        </div>
      </AspectRatio>
      <div className="px-2.5 pt-2">
        <div className="truncate text-sm font-medium" title={videoDisplayName(video)}>
          {videoDisplayName(video)}
        </div>
        <div className="truncate text-xs text-[var(--muted)]" title={video.fileName}>
          {video.fileName}
        </div>
        {meta && <div className="mt-0.5 text-xs text-[var(--muted)]">{meta}</div>}
        <div className="truncate text-xs text-[var(--muted)]" title={video.path}>
          {video.path}
        </div>
      </div>
      <div className="px-2.5 pb-2.5 pt-2">
        <Button
          className="w-full"
          disabled={busy}
          onClick={onKeep}
          size="sm"
          title="保留这个拷贝，并把内容相同的其他拷贝移入回收站（不会删除文件）"
          variant="primary"
        >
          保留此份
        </Button>
      </div>
    </ClickableCard>
  );
}

/** Confirmation for the destructive half: the other records go away. */
function KeepCopyDialog({
  keep,
  others,
  onCancel,
  onConfirm,
}: {
  keep: Video;
  others: Video[];
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <Dialog open onOpenChange={(open) => !open && onCancel()} purpose="form">
      <DialogContent>
        <DialogHeader
          title="保留这个拷贝？"
          subtitle={<>将保留「{keep.fileName}」，并把内容相同的其他 {others.length} 个拷贝移入回收站：</>}
        />
        <List className="max-h-[220px] overflow-y-auto">
          {others.map((v) => (
            <ListItem key={v.id} label={<span title={v.path}>{v.path}</span>} />
          ))}
        </List>
        <div className="text-xs text-[var(--muted)]">
          原始视频文件不会被删除，移入回收站后仍可恢复。要真正释放磁盘空间，请在「打开所在文件夹」后用资源管理器删除多余的那份文件。
        </div>
        <DialogFooter>
          <Button onClick={onCancel}>取消</Button>
          <Button variant="danger" onClick={onConfirm}>保留此拷贝，其余移入回收站</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
