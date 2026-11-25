import type { Highlight, HighlightPreview } from "../types";

/**
 * 精彩时刻的时间输入、校验与展示文案。
 *
 * 规则来自 docs/spec/highlight-moments.md §3，并且与后端 `video.AddHighlight` 是
 * 同一张表：整数秒、分/秒 0–59、结束晚于开始、时间段 1–10 秒、不超视频时长。
 * 两处用同一套中文文案，用户在前端看到的红字和后端拒绝时的理由才是同一句话。
 *
 * 这里只做"判断"，不夹取也不改写：越界要看得见，悄悄改掉用户输入会让人以为
 * 自己填的 0:75 变成了 1:15。
 */

/** 一条精彩时刻允许的最短/最长时间段，与后端常量保持一致。 */
export const MIN_HIGHLIGHT_RANGE_MS = 1000;
export const MAX_HIGHLIGHT_RANGE_MS = 10000;

/** 时 / 分 / 秒三个输入框的原始文本；空串表示这一格没填。 */
export type HighlightTimeParts = { h: string; m: string; s: string };

/** 两行输入器的初始值（也用于「清空结束时间」）。 */
export const EMPTY_HIGHLIGHT_TIME_PARTS: HighlightTimeParts = { h: "", m: "", s: "" };

/** 三个框全空。结束行全空意味着"时刻截图"，而不是 0:00:00。 */
export function isHighlightTimePartsEmpty(parts: HighlightTimeParts): boolean {
  return !parts.h.trim() && !parts.m.trim() && !parts.s.trim();
}

/** 单个字段的数值：空串按 0 解释，非数字返回 NaN（见 timePartsToMs 的契约）。 */
function partValue(text: string): number {
  const trimmed = text.trim();
  // 结束时间填了「1 分」而秒留空，就是 1:00——空着的子字段按 0 解释。
  if (!trimmed) return 0;
  // 输入器已经滤掉非数字；这里返回 NaN 是为了让程序调用出错时也被挡住，而不是被
  // Number() 的宽松转换（"" → 0、"1e3" → 1000）悄悄当成别的数。
  return /^\d+$/.test(trimmed) ? Number(trimmed) : Number.NaN;
}

/**
 * 时/分/秒 → 毫秒。
 *
 * 三个框全空返回 `null`，因为"没有这个时间"和 0:00:00 是两件事：结束行全空表示
 * 时刻截图，而 0:00:00 是一个真实存在的时间点。任一格非数字返回 `NaN`——那是
 * "输入无效"，调用方必须与"没填"分开处理。
 */
export function timePartsToMs(parts: HighlightTimeParts): number | null {
  if (isHighlightTimePartsEmpty(parts)) return null;
  const h = partValue(parts.h);
  const m = partValue(parts.m);
  const s = partValue(parts.s);
  if (Number.isNaN(h) || Number.isNaN(m) || Number.isNaN(s)) return Number.NaN;
  return (h * 3600 + m * 60 + s) * 1000;
}

/** 分或秒超过 59。必须在换算成毫秒之前判断：0:75 换算后就是 1:15，长度看起来完全正常。 */
function minutesSecondsOutOfRange(parts: HighlightTimeParts): boolean {
  return [parts.m, parts.s].some((text) => {
    const value = partValue(text);
    return !Number.isNaN(value) && value > 59;
  });
}

/**
 * 输入框层面的校验：还没到"时间段长度"这一步。
 *
 * 只返回第一条错误。同时标出三条红字不会让人更快改对，反而把真正要改的那一格
 * 埋掉了。顺序按规格 §3.2 的表格：先看开始时间本身，再看结束行能不能构成区间；
 * 时长越界那一类留给 validateHighlightTimes。
 */
export function validateHighlightParts(
  start: HighlightTimeParts,
  end: HighlightTimeParts,
): string | null {
  if (isHighlightTimePartsEmpty(start)) return "请填写开始时间";
  if (minutesSecondsOutOfRange(start)) return "分/秒需在 0–59";
  const startMs = timePartsToMs(start);
  if (startMs === null || Number.isNaN(startMs)) return "开始时间无效";

  if (isHighlightTimePartsEmpty(end)) return null;
  if (minutesSecondsOutOfRange(end)) return "分/秒需在 0–59";
  const endMs = timePartsToMs(end);
  if (endMs === null || Number.isNaN(endMs)) return "结束时间无效";
  // 结束行填了值就是时间段，哪怕填的是 0:00:00。它不可能是合法的区间终点（开始
  // 时间不会为负），而毫秒层面的校验把 endMs === 0 当成"时刻"跳过（与后端一致），
  // 所以这里必须拦住，否则保存会安静地变成一张截图。
  if (endMs === 0) return "结束时间必须晚于开始时间";
  return null;
}

/**
 * 时间层面的校验：顺序、长度与视频时长。
 *
 * `endMs === 0` 表示"时刻"，不参与时间段规则；`durationMs <= 0` 表示时长还未知
 * （ffprobe 没读到），此时跳过越界判断——不知道多长就不能说用户超了。
 */
export function validateHighlightTimes(
  startMs: number,
  endMs: number,
  durationMs: number,
): string | null {
  if (!Number.isFinite(startMs) || startMs < 0) return "开始时间无效";

  if (endMs > 0) {
    if (!Number.isFinite(endMs)) return "结束时间无效";
    if (endMs <= startMs) return "结束时间必须晚于开始时间";
    const range = endMs - startMs;
    if (range < MIN_HIGHLIGHT_RANGE_MS) return "时间段至少 1 秒";
    // 报出当前秒数：用户才知道该往哪边收，而不是只被告知"太长了"。
    if (range > MAX_HIGHLIGHT_RANGE_MS) {
      return `时间段最长 10 秒（当前 ${Math.round(range / 1000)} 秒）`;
    }
  }

  if (durationMs > 0 && startMs >= durationMs) return "开始时间超出视频时长";
  if (durationMs > 0 && endMs > durationMs) return "结束时间超出视频时长";
  return null;
}

/** 毫秒 → `0:00` / `1:30` / `1:02:03`。小时只在需要时出现，秒始终两位。 */
export function formatHighlightTime(ms: number): string {
  const total = Math.max(0, Math.round(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const ss = String(s).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
}

/**
 * 卡片上的时间标签：`时刻 1:30` / `时间段 1:30–1:35 · 5 秒`。
 *
 * 用 `kind` 与 `endMs` 一起判断：`kind` 是后端写的权威字段，`endMs === 0` 是
 * "时刻"的数据含义，两者不一致的记录（不该出现）按时刻展示，至少不会画出
 * `1:30–0:00` 这种读不懂的区间。
 */
export function highlightLabel(h: Highlight): string {
  const start = formatHighlightTime(h.startMs);
  if (h.kind !== "range" || h.endMs <= 0) return `时刻 ${start}`;
  const seconds = Math.round((h.endMs - h.startMs) / 1000);
  return `时间段 ${start}–${formatHighlightTime(h.endMs)} · ${seconds} 秒`;
}

/** 生成还没结束：排队或正在跑。桌面端据此显示占位卡，手机端据此决定要不要轮询。 */
export function isHighlightPending(h: Highlight): boolean {
  return h.status === "queued" || h.status === "running";
}

/** 生成完成，`mediaPath` 指向的文件已经可以显示。 */
export function isHighlightReady(h: Highlight): boolean {
  return h.status === "ready";
}

/**
 * 手机端可见的条目：隐藏失败行。
 *
 * 移动端不提供重试与删除，一张点不动的失败卡只会让人以为手机上能处理；失败计数
 * 仍会出现在桌面端的任务看板里（那里才有重试入口）。
 */
export function mobileVisibleHighlights(list: Highlight[]): Highlight[] {
  return list.filter((h) => h.status !== "failed");
}

/**
 * 这张预览是否就是当前输入所描述的那一张。
 *
 * 预览带着生成时的请求（`startMs` / `endMs`），所以"用户改了时间"不需要另一个
 * 状态位去记：比对一下就知道。过期的预览必须当作不存在——否则"确认效果后保存"
 * 会变成"确认 1:30–1:35，保存下来的是 2:00–2:05"。
 *
 * 毫秒为 0 的结束时间表示时刻截图，同样是可比对的值。
 */
export function previewMatchesRequest(
  preview: HighlightPreview,
  startMs: number,
  endMs: number,
): boolean {
  return preview.startMs === startMs && preview.endMs === endMs;
}
