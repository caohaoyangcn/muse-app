import { useEffect, useState, type ReactNode } from "react";

import { addVideoHighlight } from "../services/desktop";
import { invalidate } from "../services/live";
import {
  EMPTY_HIGHLIGHT_TIME_PARTS,
  formatHighlightTime,
  isHighlightTimePartsEmpty,
  timePartsToMs,
  validateHighlightParts,
  validateHighlightTimes,
  type HighlightTimeParts,
} from "../utils/highlightTime";
import { Banner } from "../ui/astryx";
import { Button } from "../ui/button";
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

/**
 * 新增精彩时刻（桌面端弹窗）。
 *
 * 输入器是两行结构化时间而不是一个自由文本框：时刻/时间段的区别就是"结束那行
 * 有没有值"，这样它自己就能说清楚，不需要用户记住语法。校验在每次输入时算，
 * 保存按钮跟着错误一起禁用；提交时后端还会再校验一次（spec §3.3），前端这层
 * 只是让错误立刻可见，不是安全边界。
 *
 * 保存成功后先 `invalidate()` 再关窗：生成本身在后台跑，弹窗没有理由继续挡着
 * 详情页；占位卡会自己出现在刚被失效重读的那一栏里。
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
  /** 源文件读不到的原因（缺失 / 磁盘未接入 / 网盘离线）。有值时禁止保存。 */
  sourceUnavailableReason?: string;
}) {
  const [start, setStart] = useState<HighlightTimeParts>(EMPTY_HIGHLIGHT_TIME_PARTS);
  const [end, setEnd] = useState<HighlightTimeParts>(EMPTY_HIGHLIGHT_TIME_PARTS);
  const [saving, setSaving] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  // 每次打开都从空白开始：上一次的时间多半不是这一次想要的，悄悄沿用旧值会让人
  // 在没看清的情况下保存一条错的时刻。
  useEffect(() => {
    if (!open) return;
    setStart(EMPTY_HIGHLIGHT_TIME_PARTS);
    setEnd(EMPTY_HIGHLIGHT_TIME_PARTS);
    setActionError(null);
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

  // 结果行只在"现在按保存会发生什么"确定时说：还在报错时先让用户把红字改掉。
  const canSubmit = error === null && startMs !== null && !Number.isNaN(startMs);
  let preview: string | null = null;
  if (canSubmit) {
    preview = endEmpty
      ? `将生成截图：${formatHighlightTime(startMs)}`
      : `将生成循环 GIF：${formatHighlightTime(startMs)}–${formatHighlightTime(endMs)}（${Math.round(
          (endMs - startMs) / 1000,
        )} 秒）`;
  }

  const blocked = sourceUnavailableReason !== undefined;

  /**
   * 只留数字，不做别的改写：不夹取、不补零、不自动跳格。用户打错的 0:75 会由校验
   * 指出来，而不是被悄悄变成 1:15。
   */
  const digits = (raw: string) => raw.replace(/\D/g, "");
  const setStartField = (key: keyof HighlightTimeParts, raw: string) =>
    setStart((prev) => ({ ...prev, [key]: digits(raw) }));
  const setEndField = (key: keyof HighlightTimeParts, raw: string) =>
    setEnd((prev) => ({ ...prev, [key]: digits(raw) }));

  const save = async () => {
    if (error !== null || blocked || saving || startMs === null) return;
    setSaving(true);
    setActionError(null);
    try {
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

  return (
    <Dialog open={open} width={520} onOpenChange={(next) => !next && onClose()} purpose="form">
      <DialogContent className="flex flex-col">
        <DialogHeader title="新增精彩时刻" />
        <p className="mt-1 text-xs text-[var(--muted)]">
          填一个时刻生成截图；再填结束时间则生成该时间段的循环 GIF（1–10 秒）。
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
                onClick={() => setEnd(EMPTY_HIGHLIGHT_TIME_PARTS)}
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
        </div>

        <div className="mt-4 min-h-[44px]">
          {preview && <p className="text-sm">{preview}</p>}
          {error && <p className="mt-1 text-sm text-[var(--danger)]">{error}</p>}
          {sourceUnavailableReason && (
            <p className="mt-1 text-sm text-[var(--danger)]">{sourceUnavailableReason}</p>
          )}
        </div>

        {actionError ? (
          <div className="mt-2">
            <Banner status="error" title={actionError} />
          </div>
        ) : null}

        <DialogFooter className="mt-4">
          <Button disabled={saving} onClick={onClose}>
            取消
          </Button>
          <Button
            disabled={error !== null || blocked || saving}
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
