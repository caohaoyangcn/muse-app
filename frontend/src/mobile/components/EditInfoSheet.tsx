import { useEffect, useState } from "react";

import type { Video } from "../../types";
import { BottomSheet } from "./Sheet";

/** Sheet for editing a favorite's custom title and notes. */
export function EditInfoSheet({
  video,
  open,
  busy,
  onSave,
  onClose,
}: {
  video: Video;
  open: boolean;
  busy: boolean;
  onSave: (title: string, notes: string) => void;
  onClose: () => void;
}) {
  const [title, setTitle] = useState(video.title ?? "");
  const [notes, setNotes] = useState(video.notes ?? "");

  // Reset to the current values whenever the sheet opens for a video, so a
  // cancelled edit never leaks into the next open.
  useEffect(() => {
    if (open) {
      setTitle(video.title ?? "");
      setNotes(video.notes ?? "");
    }
  }, [open, video.id, video.title, video.notes]);

  return (
    <BottomSheet onClose={onClose} open={open} title="编辑信息">
      <div className="flex flex-col gap-4 px-3 pb-2 pt-1">
        <label className="flex flex-col gap-1.5">
          <span className="text-xs text-[var(--muted)]">
            标题（留空则显示刮削番号，未刮削时显示文件名）
          </span>
          <input
            className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)] px-3 py-2.5 outline-none focus:border-[var(--accent)]"
            onChange={(e) => setTitle(e.target.value)}
            placeholder={video.javCode?.trim() || video.fileName}
            value={title}
          />
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-xs text-[var(--muted)]">备注</span>
          <textarea
            className="min-h-28 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)] px-3 py-2.5 outline-none focus:border-[var(--accent)]"
            onChange={(e) => setNotes(e.target.value)}
            placeholder="添加备注…"
            value={notes}
          />
        </label>
        <button
          className="rounded-[var(--radius)] bg-[var(--accent)] py-3 font-medium text-white disabled:opacity-50"
          disabled={busy}
          onClick={() => onSave(title, notes)}
          type="button"
        >
          {busy ? "保存中…" : "保存"}
        </button>
      </div>
    </BottomSheet>
  );
}
