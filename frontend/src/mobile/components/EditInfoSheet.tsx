import { useEffect, useState } from "react";

import type { Video } from "../../types";
import { TextArea } from "../../ui/astryx";
import { Button } from "../../ui/button";
import { Input } from "../../ui/input";
import { BottomSheet } from "../../ui/sheet";

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
    <BottomSheet height="tall" onClose={onClose} open={open} title="编辑信息">
      <div className="flex flex-col gap-4 px-3 pb-2 pt-1">
        <label className="flex flex-col gap-1.5">
          <span className="text-xs text-[var(--muted)]">
            标题（留空则显示刮削番号，未刮削时显示文件名）
          </span>
          <Input
            aria-label="标题"
            onChange={(e) => setTitle(e.target.value)}
            placeholder={video.javCode?.trim() || video.fileName}
            value={title}
          />
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-xs text-[var(--muted)]">备注</span>
          <TextArea
            label="备注"
            isLabelHidden
            onChange={(value) => setNotes(value)}
            placeholder="添加备注…"
            rows={4}
            value={notes}
          />
        </label>
        <Button className="py-3" disabled={busy} onClick={() => onSave(title, notes)} variant="primary">
          {busy ? "保存中…" : "保存"}
        </Button>
      </div>
    </BottomSheet>
  );
}
