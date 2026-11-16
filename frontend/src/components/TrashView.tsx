import { useState } from "react";
import type { Video } from "../types";
import { formatFileSize, videoCoverURL } from "../utils/format";
import { videoDisplayName } from "../utils/videoName";
import ConfirmDialog from "./ConfirmDialog";
import type { ToastItem } from "../ui/toast";
import {
  Badge,
  EmptyState,
  List,
  ListItem,
  SegmentedControl,
  SegmentedControlItem,
  Spinner,
  Toolbar,
} from "../ui/astryx";
import { Button } from "../ui/button";
import { Icon } from "../ui/icon";
import { groupTrashBySource, remainingDays, useTrash } from "../views/trash/useTrash";

/**
 * 回收站: favorites removed from the library, kept restorable for the retention
 * period. This is not the filesystem recycle bin — nothing here ever holds or
 * deletes a source video. Rows keep their id, cover, group memberships and
 * scraped metadata, so 恢复 puts the exact favorite back.
 */
export default function TrashView({
  onNotify,
  onOpenSettings,
}: {
  onNotify: (toast: Omit<ToastItem, "id">, ttl?: number) => void;
  onOpenSettings: (section?: string) => void;
}) {
  const {
    mode,
    setMode,
    trashVideos,
    archivedVideos,
    videos,
    retentionDays,
    totalSize,
    loading,
    error,
    reload,
    busyId,
    emptying,
    clearingArchive,
    restore,
    purge,
    empty,
    clearArchive,
  } = useTrash((notice) =>
    onNotify({ kind: notice.kind, message: notice.text, details: notice.details }),
  );

  // Which confirmation is open is presentation, not data: each shell owns it.
  const [purgeTarget, setPurgeTarget] = useState<Video | null>(null);
  const [emptyTarget, setEmptyTarget] = useState(false);
  const [clearArchiveTarget, setClearArchiveTarget] = useState(false);

  const groupedArchive = groupTrashBySource(archivedVideos);

  if (loading) {
    return (
      <div className="flex min-h-[320px] items-center justify-center">
        <Spinner label="加载中…" size="lg" />
      </div>
    );
  }
  if (error && videos.length === 0) {
    return (
      <EmptyState
        actions={<Button onClick={reload} variant="primary">重试</Button>}
        description={error}
        title="回收站加载失败"
      />
    );
  }

  return (
    <div className="flex min-h-0 flex-col">
      <Toolbar
        className="sticky top-0 z-[5] -mx-5 mb-3 rounded-none border-b border-[var(--border)] bg-[var(--bg)] px-5"
        endContent={
          mode === "trash" ? (
            <Button
              disabled={videos.length === 0 || emptying}
              icon={<Icon className="size-3.5" name="archive" />}
              onClick={() => setEmptyTarget(true)}
              variant="danger"
            >
              {emptying ? "清空中…" : "清空回收站"}
            </Button>
          ) : (
            <Button
              disabled={videos.length === 0 || clearingArchive}
              icon={<Icon className="size-3.5" name="archive" />}
              onClick={() => setClearArchiveTarget(true)}
              variant="danger"
            >
              {clearingArchive ? "清理中…" : "清空归档"}
            </Button>
          )
        }
        label="归档与回收站"
        size="sm"
        startContent={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <h2 className="m-0 text-[15px] font-semibold tracking-[0.1px]">归档与回收站</h2>
            <SegmentedControl
              className="w-auto text-xs"
              label="归档与回收站"
              onChange={(value) => setMode(value as "archive" | "trash")}
              value={mode}
            >
              <SegmentedControlItem label={`归档 ${archivedVideos.length}`} value="archive" />
              <SegmentedControlItem label={`回收站 ${trashVideos.length}`} value="trash" />
            </SegmentedControl>
            <span className="text-xs text-[var(--muted)]">
              {videos.length} 项
              {totalSize > 0 ? ` · ${formatFileSize(totalSize)}` : ""}
            </span>
            {mode === "trash" ? (
              <Button
                className="text-xs text-[var(--muted)]"
                icon={<Icon className="size-3" name="filter" />}
                onClick={() => onOpenSettings("trash")}
                variant="ghost"
              >
                保留 {retentionDays} 天
              </Button>
            ) : null}
          </div>
        }
      />

      {videos.length === 0 ? (
        <EmptyState
          description={
            mode === "archive"
              ? "停用网盘来源后，它的收藏会长期保留在这里。归档不会自动清理，也不会出现在片库、分组和查重里。"
              : `从收藏中移除的影片会先来到这里，保留 ${retentionDays} 天后自动彻底清理。期间可以随时恢复，分组、点赞和刮削资料都还在。`
          }
          title={mode === "archive" ? "归档是空的" : "回收站是空的"}
        />
      ) : mode === "archive" ? (
        <div className="flex flex-col gap-3">
          {groupedArchive.map((group) => (
            <section key={group.name}>
              <h3 className="mb-1.5 flex items-center gap-2 text-xs font-medium text-[var(--muted)]">
                <span>{group.name}</span>
                <span className="font-normal">· {group.videos.length} 项</span>
              </h3>
              <List className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
                {group.videos.map((video) => (
                  <TrashRow
                    archived
                    busy={busyId === video.id}
                    key={video.id}
                    onPurge={() => setPurgeTarget(video)}
                    onRestore={() => void restore(video)}
                    retentionDays={0}
                    video={video}
                  />
                ))}
              </List>
            </section>
          ))}
        </div>
      ) : (
        <List className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
          {videos.map((video) => (
            <TrashRow
              busy={busyId === video.id}
              key={video.id}
              onPurge={() => setPurgeTarget(video)}
              onRestore={() => void restore(video)}
              retentionDays={retentionDays}
              video={video}
            />
          ))}
        </List>
      )}

      <p className="mt-3 text-xs leading-relaxed text-[var(--muted)]">
        这里只保存收藏记录与已生成的缩略图/封面，不复制视频内容。普通回收站条目按保留期自动清理；
        归档长期保留，直到你点「清空归档」或彻底删除。源视频与网盘文件都不会被删除。
      </p>

      {purgeTarget && (
        <ConfirmDialog
          cancelLabel="取消"
          confirmLabel="彻底删除"
          danger
          message={`确定彻底删除“${videoDisplayName(purgeTarget)}”吗？\n\n这条收藏记录与生成的缩略图/封面都会被删除；原始视频或网盘文件不会被删除。此操作无法撤销。`}
          onCancel={() => setPurgeTarget(null)}
          onConfirm={() => {
            const target = purgeTarget;
            setPurgeTarget(null);
            if (target) void purge(target);
          }}
          title="彻底删除"
        />
      )}

      {emptyTarget && (
        <ConfirmDialog
          cancelLabel="取消"
          confirmLabel="清空回收站"
          danger
          message={`确定彻底删除回收站里的 ${trashVideos.length} 项收藏吗？\n\n只删除 Muse 的收藏记录与生成的缩略图/封面；原始视频文件不会被删除。此操作无法撤销。`}
          onCancel={() => setEmptyTarget(false)}
          onConfirm={() => {
            setEmptyTarget(false);
            void empty();
          }}
          title="清空回收站"
        />
      )}

      {clearArchiveTarget && (
        <ConfirmDialog
          cancelLabel="取消"
          confirmLabel="清空归档"
          danger
          message={`确定彻底删除归档里的 ${archivedVideos.length} 项收藏吗？\n\n只删除 Muse 的收藏记录与生成的缩略图/封面；网盘源文件不会被删除。此操作无法撤销。`}
          onCancel={() => setClearArchiveTarget(false)}
          onConfirm={() => {
            setClearArchiveTarget(false);
            void clearArchive();
          }}
          title="清空归档"
        />
      )}
    </div>
  );
}

function TrashRow({
  video,
  retentionDays,
  busy,
  archived = false,
  onRestore,
  onPurge,
}: {
  video: Video;
  retentionDays: number;
  busy: boolean;
  archived?: boolean;
  onRestore: () => void;
  onPurge: () => void;
}) {
  const reason =
    video.deleteReason === "missing"
      ? "文件缺失自动移入"
      : video.deleteReason === "archived"
        ? "网盘归档"
        : "手动移入";
  const groupNames = video.trashGroupNames ?? [];
  const deletedAt = video.deletedAt > 0 ? formatDateTime(video.deletedAt) : "未知时间";
  const remaining = remainingDays(video.deletedAt, retentionDays);
  const restoreBlocked = video.sourceId > 0 && !video.sourceEnabled;
  const variant =
    video.deleteReason === "missing" ? "warning" : archived ? "purple" : "neutral";
  return (
    <ListItem
      className="border-b border-[var(--border)] bg-[var(--panel)] last:border-b-0"
      description={
        <>
          <span>
            {formatFileSize(video.fileSize) ? `${formatFileSize(video.fileSize)} · ` : ""}
            {archived
              ? `归档于 ${deletedAt} · 长期保留${video.sourceName ? ` · ${video.sourceName}` : ""}`
              : `移入于 ${deletedAt} · ${remaining > 0 ? `${remaining} 天后自动清理` : "即将清理"}`}
          </span>
          {groupNames.length > 0 ? (
            <span className="mt-1 block truncate">原分组：{groupNames.join(" / ")}</span>
          ) : null}
          <span className="mt-1 block truncate" title={video.remotePath || video.path}>
            {video.remotePath || video.path}
          </span>
        </>
      }
      endContent={
        <span className="flex shrink-0 items-center gap-2">
          <Button
            disabled={busy || restoreBlocked}
            icon={<Icon className="size-3.5" name="history" />}
            onClick={onRestore}
            size="sm"
            tooltip={restoreBlocked ? "先在设置中启用该网盘，再恢复这条收藏" : undefined}
          >
            恢复
          </Button>
          <Button disabled={busy} onClick={onPurge} size="sm" variant="danger">
            彻底删除
          </Button>
        </span>
      }
      label={
        <span className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-medium">{videoDisplayName(video)}</span>
          <Badge label={reason} variant={variant} />
        </span>
      }
      startContent={
        <span className="block h-[64px] w-[112px] shrink-0 overflow-hidden rounded-[6px] bg-[#111114]">
          {video.thumbnailPath ? (
            <img
              alt=""
              className="block h-full w-full object-contain"
              draggable={false}
              src={videoCoverURL(video)}
            />
          ) : (
            <span className="flex h-full w-full items-center justify-center text-[var(--muted)]">
              <Icon className="size-5" name="archive" />
            </span>
          )}
        </span>
      }
    />
  );
}

function formatDateTime(ms: number): string {
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
