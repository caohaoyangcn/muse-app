import { useEffect, useRef, useState, type ReactNode } from "react";

import {
  addVideoHighlight,
  addVideoHighlightFromPreview,
  discardVideoHighlightPreview,
  previewVideoHighlight,
} from "../services/desktop";
import { invalidate } from "../services/live";
import {
  EMPTY_HIGHLIGHT_TIME_PARTS,
  HIGHLIGHT_DURATION_PRESETS,
  canUseHighlightPreset,
  formatHighlightTime,
  isHighlightTimePartsEmpty,
  msToTimeParts,
  previewMatchesRequest,
  timePartsToMs,
  validateHighlightParts,
  validateHighlightTimes,
  type HighlightTimeParts,
} from "../utils/highlightTime";
import { formatFileSize, highlightPreviewURL } from "../utils/format";
import type { HighlightPreview } from "../types";
import { Banner, Spinner } from "../ui/astryx";
import { Button } from "../ui/button";
import { LoopingClip } from "../ui/clip";
import { Dialog, DialogContent, DialogFooter, DialogHeader } from "../ui/dialog";
import { Input } from "../ui/input";

/** 时/分/秒三格，按这个顺序排；`key` 同时是 HighlightTimeParts 的字段名。 */
const UNITS: { key: keyof HighlightTimeParts; label: string }[] = [
  { key: "h", label: "时" },
  { key: "m", label: "分" },
  { key: "s", label: "秒" },
];

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

/**
 * 一行时间输入：三个数字框 + 可选的尾部控件（「清空结束时间」）。
 *
 * 不做自动跳格：填分钟时被强行移走焦点，比多按一次 Tab 更烦人（spec §3.1）。
 */
function TimeRow({
  name,
  label,
  parts,
  onChange,
  endContent,
}: {
  name: string;
  label: string;
  parts: HighlightTimeParts;
  onChange: (key: keyof HighlightTimeParts, value: string) => void;
  endContent?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="w-[104px] shrink-0 text-[13px]">{label}</span>
      {UNITS.map((unit, index) => (
        <span className="flex items-center gap-1.5" key={unit.key}>
          {index > 0 && (
            <span aria-hidden="true" className="text-[var(--muted)]">
              :
            </span>
          )}
          <Input
            aria-label={`${name} ${unit.label}`}
            autoComplete="off"
            className="w-14 bg-[var(--bg)] text-center"
            inputMode="numeric"
            onChange={(e) => onChange(unit.key, e.target.value)}
            placeholder={unit.label}
            value={parts[unit.key]}
          />
        </span>
      ))}
      {endContent}
    </div>
  );
}

/** 预览下方的元信息：多长、什么帧率、多大。 */
function previewDescription(preview: HighlightPreview): string {
  const range =
    preview.endMs > 0
      ? `时间段 ${formatHighlightTime(preview.startMs)}–${formatHighlightTime(preview.endMs)}` +
        ` · ${Math.round((preview.endMs - preview.startMs) / 1000)} 秒`
      : `时刻 ${formatHighlightTime(preview.startMs)}`;
  const parts = [range];
  if (preview.fps > 0) parts.push(`${preview.fps} fps`);
  if (preview.bytes > 0) parts.push(formatFileSize(preview.bytes));
  return parts.join(" · ");
}

/**
 * 新增精彩时刻（桌面端弹窗）。
 *
 * 输入器是两行结构化时间而不是一个自由文本框：时刻/时间段的区别就是"结束那行
 * 有没有值"，这样它自己就能说清楚，不需要用户记住语法。校验在每次输入时算，
 * 保存按钮跟着错误一起禁用；提交时后端还会再校验一次（spec §3.3），前端这层
 * 只是让错误立刻可见，不是安全边界。
 *
 * 弹窗里可以先生成一次**真实预览**：跑的是正式保存时的同一条 ffmpeg 流程
 * （同样的帧率、同样的 8 MB 回退），所以看到的就是会存下来的东西。预览不是记录，
 * 不会出现在详情页，也不进任务看板；保存时直接采纳这张图，不再编码第二遍。
 * 用户中途改时间，旧预览立刻作废——"确认效果后保存"不能变成"确认 A、保存 B"。
 *
 * 没预览也能保存：那条路仍是后台队列生成（预览文件可能已被清理，或者用户就是
 * 知道自己要什么）。
 */
export default function HighlightMomentDialog({
  open,
  videoId,
  durationMs,
  onClose,
  sourceUnavailableReason,
}: {
  open: boolean;
  videoId: number;
  /** 影片时长，0 表示还不知道；此时不判"超出视频时长"。 */
  durationMs: number;
  onClose: () => void;
  /** 源文件读不到的原因（缺失 / 磁盘未接入 / 网盘离线）。有值时禁止预览与保存。 */
  sourceUnavailableReason?: string;
}) {
  const [start, setStart] = useState<HighlightTimeParts>(EMPTY_HIGHLIGHT_TIME_PARTS);
  const [end, setEnd] = useState<HighlightTimeParts>(EMPTY_HIGHLIGHT_TIME_PARTS);
  const [saving, setSaving] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const [preview, setPreview] = useState<HighlightPreview | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  // 每次输入变化都会让它 +1，所以一次"生成中"的预览回来时，如果用户已经又改了
  // 时间，这张图就会被丢弃而不是当成当前预览。
  const previewToken = useRef(0);

  // 每次打开都从空白开始：上一次的时间多半不是这一次想要的，悄悄沿用旧值会让人
  // 在没看清的情况下保存一条错的时刻。
  useEffect(() => {
    if (!open) return;
    previewToken.current += 1;
    setStart(EMPTY_HIGHLIGHT_TIME_PARTS);
    setEnd(EMPTY_HIGHLIGHT_TIME_PARTS);
    setActionError(null);
    setPreviewError(null);
    setPreview(null);
  }, [open]);

  const endEmpty = isHighlightTimePartsEmpty(end);
  const startMs = timePartsToMs(start);
  const endMs = endEmpty ? 0 : (timePartsToMs(end) ?? 0);

  const partsError = validateHighlightParts(start, end);
  // 输入框层面已经出错时不再算时间：NaN 传下去只会换来第二条互相矛盾的红字。
  const timesError =
    partsError === null && startMs !== null && !Number.isNaN(startMs)
      ? validateHighlightTimes(startMs, endMs, durationMs)
      : null;
  const error = partsError ?? timesError;

  const blocked = sourceUnavailableReason !== undefined;
  const timesKnown = startMs !== null && !Number.isNaN(startMs);
  const canSubmit = error === null && timesKnown;
  // 预览只在"这张图正是当前输入描述的"时候才算数。
  const freshPreview =
    preview && timesKnown && previewMatchesRequest(preview, startMs, endMs) ? preview : null;

  let previewText: string | null = null;
  if (canSubmit && startMs !== null) {
    previewText = endEmpty
      ? `将生成截图：${formatHighlightTime(startMs)}`
      : `将生成循环视频：${formatHighlightTime(startMs)}–${formatHighlightTime(endMs)}（${Math.round(
          (endMs - startMs) / 1000,
        )} 秒）`;
  }

  /** 旧预览作废：删掉文件，别让它留在磁盘上等下一次启动清理。 */
  const dropPreview = (target: HighlightPreview | null) => {
    setPreview(null);
    if (target) void discardVideoHighlightPreview(target.path).catch(() => undefined);
  };

  /** 输入变了：旧预览作废，还在飞的生成结果也一并作废。 */
  const invalidatePreview = () => {
    previewToken.current += 1;
    setPreviewError(null);
  };

  /**
   * 只留数字，不做别的改写：不夹取、不补零、不自动跳格。用户打错的 0:75 会由校验
   * 指出来，而不是被悄悄变成 1:15。改时间等于换了问题，旧预览当场失效。
   */
  const digits = (raw: string) => raw.replace(/\D/g, "");
  const setStartField = (key: keyof HighlightTimeParts, raw: string) => {
    invalidatePreview();
    dropPreview(preview);
    setStart((prev) => ({ ...prev, [key]: digits(raw) }));
  };
  const setEndField = (key: keyof HighlightTimeParts, raw: string) => {
    invalidatePreview();
    dropPreview(preview);
    setEnd((prev) => ({ ...prev, [key]: digits(raw) }));
  };
  const clearEnd = () => {
    invalidatePreview();
    dropPreview(preview);
    setEnd(EMPTY_HIGHLIGHT_TIME_PARTS);
  };

  /**
   * 常用时长：把结束时间设成"开始 + N 秒"。
   *
   * 只改结束那三格，开始时间与预览一并作废重来——和手填结束时间的路径完全一样，
   * 所以它不会绕开校验或留下一个对不上的预览。
   */
  const applyPreset = (seconds: number) => {
    if (startMs === null) return;
    invalidatePreview();
    dropPreview(preview);
    setEnd(msToTimeParts(startMs + seconds * 1000));
  };

  const render = async () => {
    if (error !== null || blocked || previewing || saving || startMs === null) return;
    const mine = ++previewToken.current;
    setPreviewing(true);
    setPreviewError(null);
    // 换一张就是换一个问题：旧图先清掉，避免生成失败时还留着上一张。
    dropPreview(preview);
    try {
      const next = await previewVideoHighlight(videoId, startMs, endMs);
      if (mine !== previewToken.current) {
        // 生成期间用户改了时间（或关了弹窗）：这张图描述的是旧问题。
        void discardVideoHighlightPreview(next.path).catch(() => undefined);
        return;
      }
      setPreview(next);
    } catch (e) {
      if (mine === previewToken.current) setPreviewError(errorText(e));
    } finally {
      if (mine === previewToken.current) setPreviewing(false);
    }
  };

  const save = async () => {
    if (error !== null || blocked || saving || previewing || startMs === null) return;
    setSaving(true);
    setActionError(null);
    try {
      if (freshPreview) {
        try {
          // 采纳预览：文件直接成为精彩时刻的图，不再编码一次。
          await addVideoHighlightFromPreview(videoId, startMs, endMs, freshPreview.path);
          setPreview(null); // 文件已被移走，不再需要作废
          invalidate();
          onClose();
          return;
        } catch {
          // 预览文件可能已被清理（启动清理、另一个窗口）。保存不能因此失败：
          // 清掉这张图，退回后台队列重新生成一次。
          const abandoned = freshPreview;
          setPreview(null);
          void discardVideoHighlightPreview(abandoned.path).catch(() => undefined);
        }
      }
      await addVideoHighlight(videoId, startMs, endMs);
      invalidate();
      onClose();
    } catch (e) {
      // API 层的错误（数据库、媒体路径不可读）留在弹窗里，输入原样保留：改完
      // 可以直接重试，不必重新填一遍时间。
      setActionError(errorText(e));
    } finally {
      setSaving(false);
    }
  };

  /** 关闭（取消、Esc、遮罩）都算放弃：没保存的预览不该留在磁盘上。 */
  const close = () => {
    previewToken.current += 1;
    dropPreview(preview);
    onClose();
  };

  return (
    <Dialog
      open={open}
      width={560}
      onOpenChange={(next) => !next && close()}
      purpose="form"
      // Astryx caps the dialog at 75dvh by default and clips what does not fit;
      // the content below is capped a little tighter than this so the header and
      // the footer always sit inside the box.
      maxHeight="calc(100vh - 40px)"
    >
      {/*
        高度上限 + 中间滚动：预览出现后内容会明显变长，而「保存」必须始终留在
        视口里——它是这个弹窗唯一不能滚走的东西。所以头尾固定，只有中间这一段滚动。
      */}
      <DialogContent className="flex max-h-[calc(100vh-140px)] flex-col">
        <DialogHeader title="新增精彩时刻" />
        <div className="min-h-0 flex-1 overflow-y-auto pr-1">
          <p className="mt-1 text-xs text-[var(--muted)]">
            填一个时刻生成截图；再填结束时间则生成该时间段的循环视频（1–20 秒）。
            可以先预览，确认效果后再保存。
          </p>

          <div className="mt-4 flex flex-col gap-2">
            <TimeRow
              label="开始时间（必填）"
              name="开始时间"
              onChange={setStartField}
              parts={start}
            />
            <TimeRow
              endContent={
                <Button
                  onClick={clearEnd}
                  size="sm"
                  title="清空结束时间，回到时刻截图"
                  variant="ghost"
                >
                  清空结束时间
                </Button>
              }
              label="结束时间（可选）"
              name="结束时间"
              onChange={setEndField}
              parts={end}
            />
            {timesKnown && startMs !== null && (
              <div className="flex flex-wrap items-center gap-1.5 pl-[112px]">
                <span className="text-xs text-[var(--muted)]">常用时长</span>
                {HIGHLIGHT_DURATION_PRESETS.map((seconds) => {
                  const usable = canUseHighlightPreset(startMs, seconds, durationMs);
                  return (
                    <Button
                      key={seconds}
                      disabled={!usable || saving || previewing}
                      onClick={() => applyPreset(seconds)}
                      size="sm"
                      title={
                        usable
                          ? `结束时间设为 ${formatHighlightTime(startMs + seconds * 1000)}`
                          : `从 ${formatHighlightTime(startMs)} 起取不到 ${seconds} 秒：超过 20 秒上限或视频结尾`
                      }
                      variant="default"
                    >
                      +{seconds} 秒
                    </Button>
                  );
                })}
              </div>
            )}
          </div>

          <div className="mt-4 min-h-[24px]">
            {previewText && !previewError && <p className="text-sm">{previewText}</p>}
            {error && <p className="mt-1 text-sm text-[var(--danger)]">{error}</p>}
            {sourceUnavailableReason && (
              <p className="mt-1 text-sm text-[var(--danger)]">{sourceUnavailableReason}</p>
            )}
          </div>

          <div className="mt-3 flex flex-wrap items-center gap-2">
            <Button
              disabled={!canSubmit || blocked || previewing || saving}
              onClick={() => void render()}
              size="sm"
              title="用正式保存时的同一条指令生成一次，看看效果"
              variant="default"
            >
              {previewing ? "生成预览中…" : freshPreview ? "重新预览" : "预览"}
            </Button>
            {previewing && <Spinner label="生成预览中…" size="sm" />}
            {!freshPreview && !previewing && canSubmit && (
              <span className="text-xs text-[var(--muted)]">
                直接保存也可以，生成会在后台队列里进行。
              </span>
            )}
            {freshPreview && (
              <span className="text-xs text-[var(--muted)]">预览即所得：保存后用的就是这张图。</span>
            )}
          </div>

          {previewError && (
            <p className="mt-2 text-sm text-[var(--danger)]" role="alert">
              {previewError}
            </p>
          )}

          {freshPreview && (
            <figure className="mt-3">
              {freshPreview.kind === "range" ? (
                <LoopingClip
                  aspect={null}
                  className="h-[clamp(150px,30vh,280px)] rounded-[var(--radius)] border border-[var(--border)]"
                  label="精彩时刻预览"
                  src={highlightPreviewURL(freshPreview)}
                />
              ) : (
                <div className="flex min-h-[150px] items-center justify-center overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[#111114] p-2">
                  <img
                    alt="精彩时刻预览"
                    className="max-h-[30vh] max-w-full object-contain"
                    src={highlightPreviewURL(freshPreview)}
                  />
                </div>
              )}
              <figcaption className="mt-2 text-xs text-[var(--muted)]">
                预览：{previewDescription(freshPreview)}
              </figcaption>
            </figure>
          )}

          {actionError ? (
            <div className="mt-3">
              <Banner status="error" title={actionError} />
            </div>
          ) : null}
        </div>

        <DialogFooter className="mt-4 shrink-0 border-t border-[var(--border)] pt-3">
          <Button disabled={saving} onClick={close}>
            取消
          </Button>
          <Button
            disabled={!canSubmit || blocked || saving || previewing}
            onClick={() => void save()}
            variant="primary"
          >
            {saving ? "保存中…" : "保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
