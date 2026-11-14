import { useCallback, useEffect, useMemo, useState } from "react";
import type { GroupSuggestion } from "../types";
import { Button } from "../ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";

/** One file's suggestion entry, decorated with the backend video id so group
 * membership can be applied after the user confirms. */
export interface SuggestFile {
  videoId: number;
  path: string;
  fileName: string;
  suggested: GroupSuggestion[];
}

interface GroupSuggestDialogProps {
  files: SuggestFile[];
  /** Applies the chosen groups to one video. Called only for files the user
   * confirms (skipped files call nothing). */
  onApplyGroups: (videoId: number, groupIds: number[]) => Promise<void>;
  onClose: () => void;
}

/**
 * Post-add group suggestion carousel. Shows one file at a time with its
 * auto-inferred group suggestions (pre-checked). The user can toggle which
 * suggested groups to join, skip a file, or skip the whole batch. Switching
 * between files uses the ‹ › buttons or the Left/Right arrow keys; confirming
 * or skipping a file auto-advances to the next one.
 */
export default function GroupSuggestDialog({
  files,
  onApplyGroups,
  onClose,
}: GroupSuggestDialogProps) {
  const [index, setIndex] = useState(0);
  const [checked, setChecked] = useState<Record<number, Set<number>>>({});
  const [busy, setBusy] = useState(false);

  // Pre-check all suggested groups for every file up front.
  useEffect(() => {
    const initial: Record<number, Set<number>> = {};
    for (const f of files) {
      initial[f.videoId] = new Set(f.suggested.map((g) => g.groupId));
    }
    setChecked(initial);
  }, [files]);

  const current = files[index];

  const goto = useCallback(
    (n: number) => {
      if (n < 0 || n >= files.length) return;
      setIndex(n);
    },
    [files.length],
  );

  const next = useCallback(() => {
    setIndex((i) => Math.min(i + 1, files.length - 1));
  }, [files.length]);

  // Apply + advance. When the last file is processed the dialog closes.
  const handleConfirm = async () => {
    if (!current || busy) return;
    setBusy(true);
    try {
      await onApplyGroups(current.videoId, Array.from(checked[current.videoId] ?? new Set()));
    } finally {
      setBusy(false);
      if (index >= files.length - 1) onClose();
      else next();
    }
  };

  const handleSkip = () => {
    if (busy) return;
    if (index >= files.length - 1) onClose();
    else next();
  };

  // Arrow keys navigate the carousel; Dialog handles Escape.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "ArrowLeft") {
        goto(index - 1);
      } else if (e.key === "ArrowRight") {
        goto(index + 1);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [goto, index, onClose]);

  const matchCount = useMemo(
    () => files.reduce((n, f) => n + f.suggested.length, 0),
    [files],
  );

  return (
    <Dialog open width={520} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <div className="flex items-baseline justify-between gap-3">
          <DialogHeader>
            <DialogTitle>加入分组建议</DialogTitle>
            <DialogDescription className="sr-only">选择要加入的视频分组</DialogDescription>
          </DialogHeader>
          <div className="text-xs text-[var(--muted)]">
            {index + 1} / {files.length} · {matchCount} 个建议
          </div>
        </div>

        <div className="mt-3 flex items-center gap-2">
          {files.length > 1 && (
            <Button
              size="sm"
              variant="ghost"
              className="h-8 w-8 shrink-0 rounded-lg p-0 text-lg"
              onClick={() => goto(index - 1)}
              disabled={index === 0}
              aria-label="上一个"
            >
              ‹
            </Button>
          )}

          <div className="min-w-0 flex-1 overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)]">
            <div className="flex transition-transform duration-200 ease-in-out" style={{ transform: `translateX(${-index * 100}%)` }}>
              {files.map((f, i) => (
                <div className="min-w-0 basis-full shrink-0 p-3.5" key={f.videoId}>
                  <div className="mb-2.5 truncate text-sm font-semibold" title={f.fileName}>
                    {f.fileName}
                  </div>
                  {f.suggested.length === 0 ? (
                    <div className="py-2 text-[13px] text-[var(--muted)]">此文件没有匹配的分组建议</div>
                  ) : (
                    <div className="flex flex-col gap-1.5">
                      {f.suggested.map((g) => (
                        <div
                          key={g.groupId}
                          className={`flex cursor-pointer items-center gap-2.5 rounded-lg border p-2 px-3 ${
                            (checked[f.videoId] ?? new Set()).has(g.groupId)
                              ? "border-[var(--accent)] bg-[var(--panel)]"
                              : "border-[var(--border)] bg-[var(--panel-2)]"
                          }`}
                          onClick={() => {
                            const next = new Set(checked[f.videoId] ?? new Set());
                            if (next.has(g.groupId)) next.delete(g.groupId);
                            else next.add(g.groupId);
                            setChecked((prev) => ({ ...prev, [f.videoId]: next }));
                          }}
                        >
                          <input
                            type="checkbox"
                            checked={(checked[f.videoId] ?? new Set()).has(g.groupId)}
                            onChange={() => {}}
                          />
                          <span className="min-w-0 flex-1 text-[var(--text)]">{g.name}</span>
                        </div>
                      ))}
                    </div>
                  )}
                  {i === index && (
                    <div className="mt-2.5 text-[11px] text-[var(--muted)]">
                      勾选要加入的分组，或跳过 — 左右键切换文件
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>

          {files.length > 1 && (
            <Button
              size="sm"
              variant="ghost"
              className="h-8 w-8 shrink-0 rounded-lg p-0 text-lg"
              onClick={() => goto(index + 1)}
              disabled={index === files.length - 1}
              aria-label="下一个"
            >
              ›
            </Button>
          )}
        </div>

        <DialogFooter>
          <Button onClick={onClose} disabled={busy}>
            跳过本批
          </Button>
          <Button onClick={() => void handleSkip()} disabled={busy}>
            跳过此文件
          </Button>
          <Button
            variant="primary"
            onClick={() => void handleConfirm()}
            disabled={busy}
          >
            加入分组并继续
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
