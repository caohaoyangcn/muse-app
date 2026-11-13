import { useMemo, useState } from "react";
import { refreshStatus } from "../services/api";
import type { JavDuplicateGroup, Video } from "../types";
import { formatDuration, formatFileSize, videoCoverURL, videoResolution } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import ContentDuplicatePanel from "./ContentDuplicateView";
import { Button } from "../ui/button";
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
      <div className="mb-4 flex items-center gap-1 border-b border-[var(--border)]">
        <TabButton
          active={tab === "jav"}
          hint="同一番号被收藏了多个版本（不同清晰度、不同压制组）"
          onClick={() => setTab("jav")}
        >
          同番号多版本
        </TabButton>
        <TabButton
          active={tab === "content"}
          hint="同一份文件被收藏了两份（改名、换目录、跨盘备份）"
          onClick={() => setTab("content")}
        >
          内容重复
        </TabButton>
      </div>
      {tab === "jav" ? <JavDuplicatePanel {...props} /> : <ContentDuplicatePanel {...props} />}
    </div>
  );
}

/** One entry in the panel's tab strip. */
function TabButton({
  active,
  hint,
  onClick,
  children,
}: {
  active: boolean;
  hint: string;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      title={hint}
      onClick={onClick}
      className={`-mb-px border-b-2 px-3 py-1.5 text-sm ${
        active
          ? "border-[var(--accent)] font-semibold text-[var(--text)]"
          : "border-transparent text-[var(--muted)] hover:text-[var(--text)]"
      }`}
    >
      {children}
    </button>
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
        <label className="flex cursor-pointer items-center gap-1.5 text-xs text-[var(--muted)]">
          <input
            type="checkbox"
            className="accent-[var(--accent)]"
            checked={includeIgnored}
            onChange={(e) => setIncludeIgnored(e.target.checked)}
          />
          显示已忽略
        </label>
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
        <div className="flex min-h-[320px] flex-col items-center justify-center gap-3 text-center text-[var(--danger)]">
          <div>加载查重结果失败</div>
          <div className="max-w-[680px] break-words text-xs">{error}</div>
          <Button size="sm" onClick={reload}>重试</Button>
        </div>
      ) : loading ? (
        <div className="flex min-h-[320px] items-center justify-center text-[var(--muted)]">加载中…</div>
      ) : groups.length === 0 ? (
        <div className="flex min-h-[320px] flex-col items-center justify-center text-center text-[var(--muted)]">
          <div className="mb-1.5 text-xl text-[var(--text)]">没有重复作品</div>
          <div>同一番号被收藏多个版本时会显示在这里；带 -1 / -2 上下集标记的文件视为不同作品</div>
        </div>
      ) : filtered.length === 0 ? (
        <div className="flex min-h-[320px] flex-col items-center justify-center text-center text-[var(--muted)]">
          <div className="mb-1.5 text-xl text-[var(--text)]">无结果</div>
          <div>没有匹配“{keyword.trim()}”的重复作品</div>
        </div>
      ) : (
        <div className="flex flex-col gap-4 pb-8">
          {filtered.map((group) => (
            <section
              key={group.key}
              className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)]"
            >
              <header className="flex flex-wrap items-center gap-2 border-b border-[var(--border)] px-3 py-2">
                <span className="font-semibold">{group.code}</span>
                <span className="rounded-full bg-[var(--panel-2)] px-2 py-0.5 text-[10px] text-[var(--muted)]">
                  {group.tier === "cloud" ? "网盘重复" : "本地重复"}
                </span>
                {group.part && (
                  <span className="text-xs text-[var(--muted)]">第 {group.part} 集</span>
                )}
                <span className="rounded-[10px] bg-[var(--border)] px-2 py-px text-xs text-[var(--muted)]">
                  {group.videos.length} 个版本
                </span>
                {group.ignored && (
                  <span className="border border-[var(--border)] px-2 py-px text-xs text-[var(--muted)]">
                    已忽略
                  </span>
                )}
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
    <div
      className="w-[220px] max-w-full overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)]"
      onContextMenu={onContextMenu}
    >
      <button
        type="button"
        className="block w-full text-left transition hover:border-[var(--accent)]"
        onClick={onOpen}
        title={video.path}
      >
        <div className="relative aspect-video bg-[#111114]">
          {!missing && video.thumbnailPath ? (
            <img className="block h-full w-full object-contain" src={videoCoverURL(video)} alt="" draggable={false} />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-[26px] opacity-50">
              {missing ? "⚠" : "▣"}
            </div>
          )}
          {missing && (
            <span className="absolute left-1.5 top-1.5 rounded bg-[rgba(229,83,75,0.9)] px-2 py-0.5 text-[11px] text-white">
              ⚠ 文件缺失
            </span>
          )}
          {offline && (
            <span
              className="absolute bottom-1.5 right-1.5 rounded bg-[rgba(229,83,75,0.9)] px-2 py-0.5 text-[11px] text-white"
              title={`${video.volumeRoot || "磁盘"} 未接入：${video.path}`}
            >
              ⚠ 磁盘未接入
            </span>
          )}
        </div>
        <div className="px-2.5 pt-2">
          <div className="truncate text-sm font-medium" title={videoDisplayName(video)}>
            {videoDisplayName(video)}
          </div>
          <div className="truncate text-xs text-[var(--muted)]" title={video.fileName}>
            {video.fileName}
          </div>
          {meta && <div className="mt-0.5 text-xs text-[var(--muted)]">{meta}</div>}
        </div>
      </button>
      <div className="px-2.5 pb-2.5 pt-2">
        <Button
          size="sm"
          variant="primary"
          className="w-full"
          disabled={busy}
          title="保留这个版本，并把同一作品的其他版本移入回收站（不会删除文件）"
          onClick={onKeep}
        >
          保留此版本
        </Button>
      </div>
    </div>
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
        <ul className="max-h-[220px] overflow-y-auto text-xs text-[var(--muted)]">
          {others.map((v) => (
            <li key={v.id} className="truncate" title={v.path}>· {v.fileName}</li>
          ))}
        </ul>
        <div className="text-xs text-[var(--muted)]">原始视频文件不会被删除，移入回收站后仍可恢复。</div>
        <DialogFooter>
          <Button onClick={onCancel}>取消</Button>
          <Button variant="danger" onClick={onConfirm}>保留此版本，其余移入回收站</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
