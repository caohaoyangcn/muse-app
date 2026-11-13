import { useCallback, useState, type ReactNode } from "react";

import type { JavDetail, Video } from "../../types";
import {
  formatBitrate,
  formatDuration,
  formatFileSize,
  formatFrameRate,
  videoResolution,
} from "../../utils/format";
import { formatDateTime } from "../../utils/task";
import { client, coverURL, getVideo } from "../../services/web";
import RestoredBadge from "../../ui/restored-badge";
import { invalidate } from "../../services/live";
import { useLiveQuery } from "../../hooks/useLiveQuery";
import { videoName } from "../videoDisplay";
import { ErrorState, Loading } from "./States";

const errorLabels: Record<string, string> = {
  no_code: "未识别到番号",
  not_found: "未找到该番号",
  blocked: "被站点拦截（可能需要 Cookie）",
  provider_auth: "站点鉴权失败（请更新 Cookie）",
  network: "网络错误",
  http: "站点返回异常",
  rate_limited: "请求过于频繁",
  parse: "解析失败",
  cover_download: "封面下载失败",
  provider_unavailable: "数据源不可用",
  video_missing: "文件缺失",
  database: "数据库错误",
};

/** Metadata table rows for one scraped detail. */
function javRows(d: JavDetail): [string, string][] {
  return [
    ["番号", d.code || "未提供"],
    ["标题", d.title || "未提供"],
    ["制作商", d.studio || "未提供"],
    ["系列", d.series || "未提供"],
    ["发行日期", d.releaseUnix ? formatDateTime(d.releaseUnix) : "未提供"],
    ["时长", d.durationMin ? `${d.durationMin} 分钟` : "未提供"],
    ["数据来源", d.provider || "未提供"],
    ["抓取时间", d.fetchedAt ? formatDateTime(d.fetchedAt) : "未提供"],
    ["是否无码", d.isUncensored == null ? "未提供" : d.isUncensored ? "是" : "否"],
  ];
}

/**
 * A favorite's detail screen. Reads exactly what the desktop
 * VideoDetailPanel reads (video + JAV metadata + scrape task), but lays it out
 * as one scrollable phone column and replaces the desktop's scrape/cover
 * controls with a read-only status line — scraping and cover picking stay on
 * the desktop.
 *
 * The screen addresses a favorite by id rather than receiving an object: a
 * detail URL can be opened (or refreshed) without a list behind it, so the
 * screen fetches the record itself through a live query — an edit saved in a
 * sheet the shell owns, or a like, reaches it without a refresh prop.
 */
export function VideoDetailView({
  videoId,
  onBack,
  onPlay,
  onChoosePlayer,
  onSelectActor,
  onEdit,
}: {
  videoId: number;
  onBack: () => void;
  onPlay: (v: Video) => void;
  onChoosePlayer: (v: Video) => void;
  onSelectActor: (name: string) => void;
  onEdit: (v: Video) => void;
}) {
  const [lightbox, setLightbox] = useState(false);
  const [busy, setBusy] = useState(false);

  // One query for the whole screen: the record, its scraped metadata and its
  // scrape task. Keyed by id, so a different favorite never shows the previous
  // one's data; invalidated by every write the shell performs.
  const fetchDetail = useCallback(async () => {
    const video = await getVideo(videoId);
    // A metadata failure must not hide the file info: the status section simply
    // reports "not scraped".
    const [detail, task] = await Promise.all([
      client.videoJav(videoId).catch(() => null),
      client.videoJavTask(videoId).catch(() => null),
    ]);
    return { video, detail, task };
  }, [videoId]);

  const { data, loading, error, reload } = useLiveQuery(fetchDetail, `video:${videoId}`);
  const v = data?.video ?? null;
  const detail = data?.detail ?? null;
  const task = data?.task ?? null;

  const like = useCallback(async () => {
    if (!v) return;
    setBusy(true);
    try {
      await client.likeVideo(v.id);
      invalidate();
    } finally {
      setBusy(false);
    }
  }, [v]);

  // A detail URL can outlive its favorite (a deep link to a removed record, a
  // refresh after removal). Keep the back arrow usable instead of rendering a
  // blank screen.
  if (!v) {
    return (
      <div className="flex h-full flex-col">
        <DetailHeader onBack={onBack} title="视频详情" />
        {error ? (
          <ErrorState message={error} onRetry={reload} />
        ) : (
          <Loading label="正在加载…" />
        )}
      </div>
    );
  }

  const status = task?.status || detail?.scrapeStatus;
  const currentError = detail?.errorCode
    ? errorLabels[detail.errorCode] ?? detail.lastError
    : detail?.lastError;
  // Legacy scrape rows can arrive with null actor/tag lists; the panel must
  // render regardless, so treat a missing list as empty.
  const actors = detail?.actors ?? [];
  const tags = detail?.tags ?? [];

  const fileRows: [string, string][] = [
    ["文件名", v.fileName],
    ["大小", formatFileSize(v.fileSize) || "未提供"],
    ["时长", formatDuration(v.durationMs) || "未提供"],
    ["分辨率", videoResolution(v) || "未提供"],
    ["帧率", formatFrameRate(v.frameRate) || "未提供"],
    ["比特率", formatBitrate(v.bitrate) || "未提供"],
    ["视频编码", v.videoCodec ? v.videoCodec.toUpperCase() : "未提供"],
    ["音频编码", v.audioCodec ? v.audioCodec.toUpperCase() : "无音轨"],
    ["最近播放", v.lastOpenedAt ? formatDateTime(v.lastOpenedAt) : "从未播放"],
    [v.sourceId > 0 ? "网盘路径" : "路径", v.sourceId > 0 ? v.remotePath : v.path],
  ];
  if (v.sourceId > 0) fileRows.push(["网盘", v.sourceName || "已配置连接"]);

  return (
    <div className="flex h-full flex-col">
      <DetailHeader
        badge={v.mosaicRestored ? <RestoredBadge className="text-xs" /> : null}
        onBack={onBack}
        onEdit={() => onEdit(v)}
        title={videoName(v)}
      />

      <div className="scroll-area flex-1 px-4 pb-8 pt-3" data-scroll-key="main">
        {/* Cover */}
        <button
          className="relative block w-full overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[#111114]"
          onClick={() => setLightbox(true)}
          type="button"
        >
          <img alt="" className="w-full object-contain" src={coverURL(v)} />
        </button>

        <div className="mt-4">
          <div className="flex gap-2">
            <button
              className="flex-1 rounded-[var(--radius)] bg-[var(--accent)] py-3 font-medium text-white disabled:opacity-50"
              disabled={v.status === "missing" || v.volumeOffline || v.sourceOffline}
              onClick={() => onPlay(v)}
              type="button"
            >
              ▶ 播放
            </button>
            <button
              className="rounded-[var(--radius)] border border-[var(--border)] px-4 py-3 active:bg-[var(--panel-2)] disabled:opacity-50"
              disabled={busy}
              onClick={() => void like()}
              type="button"
            >
              ♥ {v.likeCount > 0 ? v.likeCount : "点赞"}
            </button>
          </div>
          {v.volumeOffline ? (
            <p className="mt-2 text-xs text-[var(--muted)]">
              磁盘 {v.volumeRoot || ""} 未接入，插回移动硬盘后即可播放。
            </p>
          ) : v.sourceOffline ? (
            <p className="mt-2 text-xs text-[var(--muted)]">
              网盘「{v.sourceName || "连接"}」当前离线，恢复后即可播放。
            </p>
          ) : null}
          <button
            className="mt-2 w-full py-1 text-xs text-[var(--muted)] active:text-[var(--text)]"
            disabled={v.status === "missing" || v.volumeOffline || v.sourceOffline || v.sourceId > 0}
            onClick={() => onChoosePlayer(v)}
            type="button"
          >
            播放器选项
          </button>
        </div>

        {v.notes ? (
          <div className="mt-4 whitespace-pre-wrap rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)] p-3 text-sm text-[var(--muted)]">
            {v.notes}
          </div>
        ) : null}

        {/* Scrape status — read-only on mobile. */}
        <Section title="刮削状态">
          <div className="text-sm">
            {loading ? (
              <span className="text-[var(--muted)]">加载中…</span>
            ) : status ? (
              <span>
                状态：{status}
                {currentError ? (
                  <span className="text-[var(--danger)]"> · {currentError}</span>
                ) : null}
              </span>
            ) : (
              <span className="text-[var(--muted)]">尚未刮削（可在桌面端发起刮削）</span>
            )}
          </div>
        </Section>

        {detail ? (
          <>
            <Section title="元数据">
              <Table rows={javRows(detail)} />
            </Section>

            {actors.length > 0 ? (
              <Section title="演员">
                <div className="flex flex-wrap gap-2">
                  {actors.map((name) => (
                    <button
                      className="rounded-full border border-[var(--border)] bg-[var(--panel-2)] px-3 py-1.5 text-sm active:bg-[var(--panel)]"
                      key={name}
                      onClick={() => onSelectActor(name)}
                      type="button"
                    >
                      {name}
                    </button>
                  ))}
                </div>
              </Section>
            ) : null}

            {tags.length > 0 ? (
              <Section title="标签">
                <div className="flex flex-wrap gap-2">
                  {tags.map((tag) => (
                    <span
                      className="rounded-full bg-[var(--panel-2)] px-3 py-1 text-xs text-[var(--muted)]"
                      key={tag}
                    >
                      {tag}
                    </span>
                  ))}
                </div>
              </Section>
            ) : null}
          </>
        ) : null}

        <Section title="文件信息">
          <Table rows={fileRows} />
        </Section>
      </div>

      {lightbox ? (
        <button
          aria-label="关闭封面"
          className="fade-enter fixed inset-0 z-50 flex items-center justify-center bg-black/90 p-4"
          onClick={() => setLightbox(false)}
          type="button"
        >
          <img alt="" className="max-h-full max-w-full object-contain" src={coverURL(v)} />
        </button>
      ) : null}
    </div>
  );
}

function DetailHeader({
  title,
  badge,
  onBack,
  onEdit,
}: {
  title: string;
  badge?: ReactNode;
  onBack: () => void;
  onEdit?: () => void;
}) {
  return (
    <div className="flex items-center gap-2 border-b border-[var(--border)] px-2 py-2 pt-[calc(var(--safe-top)+6px)]">
      <button
        aria-label="返回"
        className="px-2 text-xl leading-none text-[var(--muted)]"
        onClick={onBack}
        type="button"
      >
        ←
      </button>
      <div className="flex min-w-0 flex-1 items-center gap-1.5">
        <div className="truncate text-sm font-medium">{title}</div>
        {badge}
      </div>
      {onEdit ? (
        <button
          className="rounded-full border border-[var(--border)] px-3 py-1.5 text-xs active:bg-[var(--panel-2)]"
          onClick={onEdit}
          type="button"
        >
          编辑
        </button>
      ) : null}
    </div>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="mt-5">
      <div className="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--muted)]">
        {title}
      </div>
      {children}
    </div>
  );
}

function Table({ rows }: { rows: [string, string][] }) {
  return (
    <dl className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
      {rows.map(([k, val], i) => (
        <div
          className={`flex gap-3 px-3 py-2.5 text-sm ${
            i % 2 ? "bg-[var(--panel)]" : "bg-[var(--card)]"
          }`}
          key={k}
        >
          <dt className="w-20 shrink-0 text-[var(--muted)]">{k}</dt>
          <dd className="min-w-0 flex-1 break-all">{val}</dd>
        </div>
      ))}
    </dl>
  );
}
