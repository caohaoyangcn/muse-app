import { useMemo, useState } from "react";
import { refreshStatus } from "../services/desktop";
import type { JavDuplicateGroup, Video } from "../types";
import { formatDuration, formatFileSize, videoCoverURL, videoResolution } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import ContentDuplicatePanel from "./ContentDuplicateView";
import {
  AspectRatio,
  Badge,
  CheckboxInput,
  ClickableCard,
  EmptyState,
  List,
  ListItem,
  SegmentedControl,
  SegmentedControlItem,
  Spinner,
} from "../ui/astryx";
import { Button } from "../ui/button";
import { Icon } from "../ui/icon";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";
import { Input } from "../ui/input";
import { filterJavGroups, useJavDuplicates } from "../views/duplicates/useJavDuplicates";

/** What both duplicate panels need from the library shell. */
type DuplicatePanelProps = {
  /** Open a version's detail — the same action as clicking a library card. */
  onOpen: (video: Video) => void;
  /** The library card menu (open / reveal / copy path / edit / remove). */
  onContextMenu: (e: React.MouseEvent, video: Video) => void;
  /** Called after versions are removed, with their ids, so the shell refreshes. */
  onResolved: (removedIds: number[]) => void;
  onNotify: (message: string, kind: "success" | "error") => void;
};

/**
 * 查重 (duplicates) holds two worklists that answer different questions, so they
 * live behind one entry with a tab each rather than two sidebar entries the user
 * would have to tell apart by judgment:
 *
 * - 同番号多版本 — one work, several encodes. Grouped by 番号 on the backend.
 * - 内容重复 — the same bytes stored twice. Grouped by content fingerprint.
 *
 * A pair may honestly appear in both: two identical copies of one work are the
 * same work at two encodes AND the same bytes twice. Neither list is a subset of
 * the other.
 */
export default function DuplicateView(props: DuplicatePanelProps) {
  const [tab, setTab] = useState<"jav" | "content">("jav");
  return (
    <div className="pt-4">
      <div className="mb-4 border-b border-[var(--border)] pb-2">
        <SegmentedControl
          label="查重方式"
          onChange={(value) => setTab(value as "jav" | "content")}
          value={tab}
        >
          <SegmentedControlItem label="同番号多版本" value="jav" />
          <SegmentedControlItem label="内容重复" value="content" />
        </SegmentedControl>
      </div>
      {tab === "jav" ? <JavDuplicatePanel {...props} /> : <ContentDuplicatePanel {...props} />}
    </div>
  );
}

/**
 * 番号 worklist: the works the library holds more than one copy of, recognized by
 * 番号. Copies sharing a 番号 but carrying different episode markers
 * ("abc-123-1" / "abc-123-2") are separate works and never appear here — the
 * grouping is done by the backend, which owns those rules.
 *
 * For each work the user either keeps one version (the other records are removed
 * from the favorites; the files themselves are never touched) or keeps them all,
 * which records the decision so the work stops being reported.
 *
 * Versions carry the same right-click menu as library cards. The panel cannot
 * delete files — by design — but "打开所在文件夹" / "复制路径" let the user remove
 * a duplicate in the file manager themselves; once the file is gone the folder
 * watcher has nothing to re-import, so the decision sticks. "移入回收站" then
 * drops the leftover record without leaving the panel.
 */
function JavDuplicatePanel({ onOpen, onContextMenu, onResolved, onNotify }: DuplicatePanelProps) {
  const [keyword, setKeyword] = useState("");
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
    // 桌面端可以把服务器/磁盘的现状重新问一遍；浏览器没有这个端点。
    refreshVersion: refreshStatus,
    onResolved,
    notify: (notice) =>
      onNotify(notice.text, notice.kind === "error" ? "error" : "success"),
  });

  const filtered = useMemo(() => filterJavGroups(groups, keyword), [groups, keyword]);

  // Keep one version: every other record in the group leaves the favorites. The
  // underlying files are not deleted, matching the app-wide rule that a
  // favorite is only a reference.
  const confirmKeep = async () => {
    if (!pendingKeep) return;
    const { group, keep } = pendingKeep;
    setPendingKeep(null);
    await keepVersion(group, keep);
  };

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <span className="text-xs text-[var(--muted)]">
          {activeCount === 0 ? "没有识别到重复作品" : `${activeCount} 部作品存在多个版本`}
          {includeIgnored && groups.length !== activeCount
            ? ` · 另有 ${groups.length - activeCount} 部已忽略`
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
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            placeholder="筛选番号或文件名"
            autoComplete="off"
            spellCheck={false}
          />
        </div>
      </div>

      {error ? (
        <EmptyState
          actions={<Button onClick={reload} size="sm" variant="primary">重试</Button>}
          description={error}
          title="加载查重结果失败"
        />
      ) : loading ? (
        <div className="flex min-h-[320px] items-center justify-center">
          <Spinner label="加载中…" size="lg" />
        </div>
      ) : groups.length === 0 ? (
        <EmptyState
          description="同一番号被收藏多个版本时会显示在这里；带 -1 / -2 上下集标记的文件视为不同作品"
          title="没有重复作品"
        />
      ) : filtered.length === 0 ? (
        <EmptyState description={`没有匹配“${keyword.trim()}”的重复作品`} title="无结果" />
      ) : (
        <div className="flex flex-col gap-4 pb-8">
          {filtered.map((group) => (
            <section
              key={group.key}
              className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)]"
            >
              <header className="flex flex-wrap items-center gap-2 border-b border-[var(--border)] px-3 py-2">
                <span className="font-semibold">{group.code}</span>
                <Badge
                  label={group.tier === "cloud" ? "网盘重复" : "本地重复"}
                  variant="neutral"
                />
                {group.part && (
                  <span className="text-xs text-[var(--muted)]">第 {group.part} 集</span>
                )}
                <Badge label={`${group.videos.length} 个版本`} variant="neutral" />
                {group.ignored ? <Badge label="已忽略" variant="neutral" /> : null}
                <Button
                  size="sm"
                  className="ml-auto"
                  disabled={busy}
                  title="保留全部版本，并不再提示这部作品（等价于忽略重复提示）"
                  onClick={() => void keepAll(group)}
                >
                  {group.videos.length > 2 ? "保留全部" : "保留两者"}
                </Button>
              </header>
              <div className="flex flex-wrap gap-3 p-3">
                {group.videos.map((video) => (
                  <VersionCard
                    key={video.id}
                    video={video}
                    busy={busy}
                    onOpen={() => onOpen(video)}
                    onContextMenu={(e) => onContextMenu(e, video)}
                    onKeep={() => setPendingKeep({ group, keep: video })}
                  />
                ))}
              </div>
            </section>
          ))}
        </div>
      )}

      {pendingKeep && (
        <KeepVersionDialog
          keep={pendingKeep.keep}
          others={pendingKeep.group.videos.filter((v) => v.id !== pendingKeep.keep.id)}
          onCancel={() => setPendingKeep(null)}
          onConfirm={() => void confirmKeep()}
        />
      )}
    </div>
  );
}

/** One copy of a duplicated work, with the action that keeps it alone. */
function VersionCard({
  video,
  busy,
  onOpen,
  onContextMenu,
  onKeep,
}: {
  video: Video;
  busy: boolean;
  onOpen: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
  onKeep: () => void;
}) {
  // A version on an unplugged drive is not missing (see VideoCard): keep the
  // cover, and say which disk it is on instead.
  const offline = video.volumeOffline === true;
  const missing = !offline && video.status === "missing";
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
          {!missing && video.thumbnailPath ? (
            <img className="block size-full object-contain" draggable={false} src={videoCoverURL(video)} alt="" />
          ) : (
            <div className="flex size-full items-center justify-center opacity-50">
              <Icon className="size-6" name="alert" />
            </div>
          )}
          {missing ? <Badge className="absolute left-1.5 top-1.5" label="文件缺失" variant="error" /> : null}
          {offline ? (
            <Badge className="absolute bottom-1.5 right-1.5" label="磁盘未接入" variant="error" />
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
      </div>
      <div className="px-2.5 pb-2.5 pt-2">
        <Button
          className="w-full"
          disabled={busy}
          onClick={onKeep}
          size="sm"
          title="保留这个版本，并把同一作品的其他版本移入回收站（不会删除文件）"
          variant="primary"
        >
          保留此版本
        </Button>
      </div>
    </ClickableCard>
  );
}

/** Confirmation for the destructive half of 查重: the other records go away. */
function KeepVersionDialog({
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
    <Dialog open onOpenChange={(open) => !open && onCancel()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>保留这个版本？</DialogTitle>
          <DialogDescription>
            将保留「{keep.fileName}」，并把同一作品的其他 {others.length} 个版本移入回收站：
          </DialogDescription>
        </DialogHeader>
        <List className="max-h-[220px] overflow-y-auto">
          {others.map((v) => (
            <ListItem key={v.id} label={<span title={v.path}>{v.fileName}</span>} />
          ))}
        </List>
        <div className="text-xs text-[var(--muted)]">原始视频文件不会被删除，移入回收站后仍可恢复。</div>
        <DialogFooter>
          <Button onClick={onCancel}>取消</Button>
          <Button variant="danger" onClick={onConfirm}>保留此版本，其余移入回收站</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
