import { useCallback, useEffect, useMemo, useState } from "react";
import type { JavDetail, JavTask, Video } from "../types";
import { client, scrapeVideoJav, setVideoJavOverride } from "../services/desktop";
import { useLiveQuery } from "../hooks/useLiveQuery";
import {
  formatBitrate,
  formatDuration,
  formatFileSize,
  formatFrameRate,
  videoCoverURL,
  videoResolution,
} from "../utils/format";
import { formatDateTime, formatReleaseDate } from "../utils/datetime";
import { videoDisplayName } from "../utils/videoName";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import {
  Badge,
  Banner,
  ClickableCard,
  MetadataList,
  MetadataListItem,
  RadioList,
  RadioListItem,
  Spinner,
  Toolbar,
} from "../ui/astryx";
import { Dialog, DialogCloseButton, DialogContent } from "../ui/dialog";

type OverrideMode = "auto" | "manual" | "skip";

const statusLabels: Record<string, string> = {
  queued: "排队中",
  running: "刮削中",
  succeeded: "已成功",
  no_code: "未识别到番号",
  not_found: "未找到作品",
  retryable: "等待重试",
  blocked: "请求被阻止",
  disabled: "已跳过",
};

const errorLabels: Record<string, string> = {
  no_code: "文件名中没有识别到番号。",
  not_found: "JavBus 中没有找到对应作品。",
  provider_auth: "JavBus 需要有效 Cookie 或 Cookie 已过期。",
  blocked: "JavBus 拒绝了请求，请检查 Cookie 或稍后重试。",
  rate_limited: "请求过于频繁，任务会在稍后自动重试。",
  network: "网络请求失败，任务会在稍后自动重试。",
  parse: "JavBus 页面结构无法识别。",
};

function valueOrUnknown(value?: string | number | null): string {
  return value === undefined || value === null || value === "" ? "未提供" : String(value);
}

function statusText(status?: string): string {
  return status ? statusLabels[status] ?? status : "未开始";
}

function isMissingJavDetail(error: unknown): boolean {
  const message = String(error).toLowerCase();
  return message.includes("no rows in result set") || message.includes("sql: no rows");
}

export default function VideoDetailPanel({
  video,
  onOpenSettings,
  onVideoChanged,
  onSelectActor,
  onPlay,
  onRevealPath,
}: {
  video: Video;
  onOpenSettings: () => void;
  onVideoChanged: () => void;
  /** Jump to the aggregated film list for one actor (演员词条快捷跳转). */
  onSelectActor?: (name: string) => void;
  /** Play the video with the system default player. */
  onPlay?: () => void;
  /** Reveal the video file in the OS file manager (定位到文件). */
  onRevealPath?: () => void;
}) {
  const [mode, setMode] = useState<OverrideMode>("auto");
  const [code, setCode] = useState("");
  const [actionError, setActionError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [scraping, setScraping] = useState(false);
  const [coverPreviewOpen, setCoverPreviewOpen] = useState(false);
  // True once the user has touched the 番号/模式 form: a background scrape of
  // some other video must not overwrite what they are typing.
  const [formTouched, setFormTouched] = useState(false);

  const fetchJav = useCallback(async (): Promise<{ detail: JavDetail | null; task: JavTask | null }> => {
    const [detail, task] = await Promise.all([
      client.videoJav(video.id).catch((e) => {
        if (isMissingJavDetail(e)) return null;
        throw e;
      }),
      client.videoJavTask(video.id).catch(() => null),
    ]);
    return { detail, task };
  }, [video.id]);

  // The live query re-reads after every scrape event, so a finished scrape
  // updates this panel without it listening to event names.
  const jav = useLiveQuery(fetchJav, `jav:${video.id}`);
  const detail = jav.data?.detail ?? null;
  const task = jav.data?.task ?? null;
  const error = actionError ?? jav.error;

  // Seed the form from the record, but never step on an edit in progress.
  useEffect(() => {
    if (formTouched) return;
    const d = jav.data?.detail ?? null;
    setMode((d?.overrideMode as OverrideMode) || "auto");
    setCode(d?.overrideMode === "manual" ? d.code : "");
  }, [jav.data, formTouched]);

  const effectiveStatus = task?.status || detail?.scrapeStatus;
  const currentError = detail?.errorCode ? errorLabels[detail.errorCode] ?? detail.lastError : detail?.lastError;
  // Scraped records written before the backend normalized empty lists can still
  // carry null here. A missing list must never throw: the error boundary would
  // replace the whole window with a stack trace.
  const actors = detail?.actors ?? [];
  const tags = detail?.tags ?? [];
  const remote = video.sourceId > 0;
  const sourceOffline = remote && video.sourceOffline === true;
  const metadataRows = useMemo<[string, string][]>(() => {
    // 路径 is rendered separately so it can be clicked to reveal the file.
    const rows: [string, string][] = [
      ["文件名", video.fileName],
      ["大小", formatFileSize(video.fileSize) || "未提供"],
      ["时长", formatDuration(video.durationMs) || "未提供"],
      ["分辨率", videoResolution(video) || "未提供"],
      ["帧率", formatFrameRate(video.frameRate) || "未提供"],
      ["比特率", formatBitrate(video.bitrate) || "未提供"],
      [
        "文件状态",
        video.volumeOffline
          ? "磁盘未接入"
          : remote && video.sourceOffline
            ? "网盘离线"
            : video.status === "missing"
              ? "文件缺失"
              : "可用",
      ],
      ["最近播放", video.lastOpenedAt ? formatDateTime(video.lastOpenedAt) : "从未播放"],
    ];
    if (remote) rows.push(["网盘", video.sourceName || "已配置连接"]);
    return rows;
  }, [remote, video]);

  const saveOverride = async () => {
    if (mode === "manual" && !code.trim()) {
      setActionError("请输入手动番号。");
      return;
    }
    setSaving(true);
    setActionError(null);
    try {
      await setVideoJavOverride(video.id, mode, mode === "manual" ? code.trim() : "");
      // The write is the record: let the form follow it again.
      setFormTouched(false);
      jav.reload();
      onVideoChanged();
    } catch (e) {
      setActionError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const scrape = async () => {
    if (mode === "skip") return;
    setScraping(true);
    setActionError(null);
    try {
      await scrapeVideoJav(video.id, true);
      // The scrape itself reports back through the backend's event; this read
      // just makes the first screen update land immediately.
      jav.reload();
    } catch (e) {
      setActionError(String(e));
    } finally {
      setScraping(false);
    }
  };

  return (
    <section className="mx-auto flex min-h-full w-full max-w-[980px] flex-col gap-5 pb-8">
      <Toolbar
        className="border-b border-[var(--border)] px-0 pb-4"
        endContent={
          <div className="flex shrink-0 flex-wrap items-center gap-2">
          {onPlay && (
            <Button
              variant="primary"
              onClick={onPlay}
              disabled={video.status === "missing" || video.volumeOffline || sourceOffline}
              title={
                sourceOffline
                  ? `网盘「${video.sourceName || ""}」当前离线，恢复后即可播放`
                  : video.volumeOffline
                    ? `磁盘 ${video.volumeRoot || ""} 未接入，插回后即可播放`
                    : video.status === "missing"
                      ? "文件缺失，无法播放"
                      : remote
                        ? "在 OpenList 中打开播放"
                        : "用系统默认播放器播放"
              }
            >
              ▶ 播放
            </Button>
          )}
          <Button onClick={() => void scrape()} disabled={scraping || saving || mode === "skip"}>
            {scraping ? "重新刮削中…" : "重新刮削"}
          </Button>
          </div>
        }
        label="视频详情"
        startContent={
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <h2 className="min-w-0 break-words text-xl font-semibold">{videoDisplayName(video)}</h2>
            {video.mosaicRestored && <Badge label="马赛克破解" variant="purple" />}
            {video.backedUp && <Badge label="已备份" variant="success" />}
          </div>
        }
      />

      {error ? (
        <Banner
          endContent={
            <Button onClick={() => { setActionError(null); jav.reload(); }} size="sm">
              重试
            </Button>
          }
          status="error"
          title={error}
        />
      ) : null}

      {jav.loading ? (
        <div className="flex min-h-[320px] items-center justify-center">
          <Spinner label="加载详情中…" size="lg" />
        </div>
      ) : (
        <>
          <div className="grid gap-5 lg:grid-cols-[280px_minmax(0,1fr)]">
            <div>
              <ClickableCard
                className="flex min-h-[260px] w-full items-center justify-center bg-[#111114] p-2"
                label="查看大图"
                onClick={() => setCoverPreviewOpen(true)}
                padding={0}
              >
                <img className="max-h-[420px] max-w-full object-contain" src={videoCoverURL(video)} alt="当前封面" />
              </ClickableCard>
              <p className="mt-2 text-xs text-[var(--muted)]">
                当前封面：{video.coverSource === "scraped" ? "JavBus 刮削" : video.coverSource === "custom_frame" ? "用户自定义" : "自动帧"}
              </p>
            </div>

            <div className="min-w-0">
              <h3 className="mb-3 text-sm font-semibold">JavBus 元数据</h3>
              {detail ? (
                <MetadataList columns={2} label={{ position: "top" }}>
                  {([
                    ["番号", valueOrUnknown(detail.code)],
                    ["标题", valueOrUnknown(detail.title)],
                    ["制作商", valueOrUnknown(detail.studio)],
                    ["系列", valueOrUnknown(detail.series)],
                    ["发行日期", formatReleaseDate(detail.releaseUnix)],
                    ["时长", detail.durationMin ? `${detail.durationMin} 分钟` : "未提供"],
                    ["数据来源", valueOrUnknown(detail.provider)],
                    ["抓取时间", formatDateTime(detail.fetchedAt)],
                    ["是否无码", detail.isUncensored === undefined ? "未提供" : detail.isUncensored ? "是" : "否"],
                  ] as [string, string][]).map(([label, value]) => (
                    <MetadataListItem key={label} label={label}>{value}</MetadataListItem>
                  ))}
                </MetadataList>
              ) : (
                <p className="text-sm text-[var(--muted)]">尚未建立刮削记录。</p>
              )}
            </div>
          </div>

          {detail && (
            <div className="grid gap-5 lg:grid-cols-2">
              <section>
                <h3 className="mb-3 text-sm font-semibold">演员</h3>
                <div className="flex flex-wrap gap-2">
                  {actors.length > 0 ? actors.map((actor) =>
                    <Button
                      className="border-[var(--border)]"
                      key={actor}
                      onClick={onSelectActor ? () => onSelectActor(actor) : undefined}
                      size="sm"
                      title={`查看 ${actor} 出演的全部影片`}
                      variant="default"
                    >
                      {actor}
                    </Button>,
                  ) : <span className="text-sm text-[var(--muted)]">未提供</span>}
                </div>
                {onSelectActor && actors.length > 0 && (
                  <p className="mt-2 text-xs text-[var(--muted)]">点击演员可查看其出演的全部影片</p>
                )}
              </section>
              <section>
                <h3 className="mb-3 text-sm font-semibold">标签</h3>
                <div className="flex flex-wrap gap-2">
                  {tags.length > 0 ? tags.map((tag) => (
                    <Badge key={tag} label={tag} variant="neutral" />
                  )) : <span className="text-sm text-[var(--muted)]">未提供</span>}
                </div>
              </section>
            </div>
          )}

          <section className="border-t border-[var(--border)] pt-5">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold">刮削状态</h3>
                <p className="mt-1 text-sm text-[var(--muted)]">{statusText(effectiveStatus)}</p>
              </div>
              {task && <span className="text-xs text-[var(--muted)]">尝试 {task.attempts}/{task.maxAttempts}</span>}
            </div>
            {currentError && <p className="mt-3 text-sm text-[var(--danger)]">{currentError}</p>}
            {detail?.nextRetryAt && <p className="mt-2 text-xs text-[var(--muted)]">下次重试：{formatDateTime(detail.nextRetryAt)}</p>}
            {detail?.errorCode === "provider_auth" && (
              <Button size="sm" className="mt-3" onClick={onOpenSettings}>前往 JavBus 设置</Button>
            )}
          </section>

          <section className="border-t border-[var(--border)] pt-5">
            <h3 className="mb-3 text-sm font-semibold">番号与刮削模式</h3>
            <RadioList
              label="刮削模式"
              onChange={(value) => {
                setMode(value as OverrideMode);
                setFormTouched(true);
              }}
              orientation="horizontal"
              value={mode}
            >
              <RadioListItem label="自动识别" value="auto" />
              <RadioListItem label="手动番号" value="manual" />
              <RadioListItem label="跳过刮削" value="skip" />
            </RadioList>
            {mode === "manual" && (
              <Input
                aria-label="番号"
                className="mt-3 max-w-[300px]"
                value={code}
                onChange={(e) => { setCode(e.target.value); setFormTouched(true); }}
                placeholder="例如：SMD-133"
                autoComplete="off"
                spellCheck={false}
              />
            )}
            <div className="mt-4 flex flex-wrap items-center gap-3">
              <Button variant="primary" onClick={() => void saveOverride()} disabled={saving || scraping}>
                {saving ? "保存并执行中…" : "保存设置"}
              </Button>
              {mode === "skip" && <span className="text-xs text-[var(--muted)]">跳过后不会再自动执行刮削。</span>}
            </div>
          </section>

          <section className="border-t border-[var(--border)] pt-5">
            <h3 className="mb-3 text-sm font-semibold">文件信息</h3>
            <MetadataList columns={2} label={{ position: "top" }}>
              <MetadataListItem label={remote ? "网盘路径" : "路径"}>
                {onRevealPath ? (
                  <Button
                    className="h-auto justify-start px-0 text-left"
                    onClick={onRevealPath}
                    size="sm"
                    title={remote ? "在 OpenList 网页中打开" : "在文件管理器中定位该文件"}
                    variant="ghost"
                  >
                    {remote ? video.remotePath : video.path}
                  </Button>
                ) : (
                  remote ? video.remotePath : video.path
                )}
              </MetadataListItem>
              {metadataRows.map(([label, value]) => (
                <MetadataListItem key={label} label={label}>{value}</MetadataListItem>
              ))}
            </MetadataList>
            {onRevealPath && (
              <p className="mt-2 text-xs text-[var(--muted)]">
                {remote ? "点击路径可在 OpenList 网页中打开" : "点击路径可在文件管理器中定位该文件"}
              </p>
            )}
          </section>
        </>
      )}

      <Dialog
        open={coverPreviewOpen}
        width="min(900px, calc(100vw - 48px))"
        label="封面大图"
        onOpenChange={setCoverPreviewOpen}
      >
        <DialogContent className="p-4">
          <DialogCloseButton />
          <div className="flex max-h-[calc(100vh-96px)] min-h-[240px] items-center justify-center overflow-auto rounded-[var(--radius)] bg-black/30 p-2">
            <img
              className="max-h-[calc(100vh-128px)] max-w-full object-contain"
              src={videoCoverURL(video)}
              alt="当前封面大图"
            />
          </div>
        </DialogContent>
      </Dialog>
    </section>
  );
}
