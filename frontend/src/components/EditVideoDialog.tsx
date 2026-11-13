import { useState } from "react";
import type { Video } from "../types";
import { client, resetVideoCover, setVideoCover, setVideoScrapedCoverAllowed } from "../services/desktop";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { videoCoverURL } from "../utils/format";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";

interface Props {
  video: Video;
  onClose: () => void;
  /** Called after a successful info/cover change so the library can refresh. */
  onSaved: (video: Video) => void;
}

const fmtTime = (t: number) => {
  if (!Number.isFinite(t) || t < 0) return "0:00";
  const m = Math.floor(t / 60);
  const s = Math.floor(t % 60);
  return `${m}:${String(s).padStart(2, "0")}`;
};

type CoverMode = "auto" | "custom";

/**
 * Edit a favorite's info (custom title/notes) and cover in one transactional
 * "Save". The cover choice is staged as a mode plus timestamp and is only
 * written (together with title/notes) when Save is pressed. The desktop app
 * deliberately does not decode video for a preview; ffmpeg extracts the chosen
 * frame directly from the source when the user saves.
 */
export default function EditVideoDialog({ video, onClose, onSaved }: Props) {
  const [title, setTitle] = useState(video.title ?? "");
  const [notes, setNotes] = useState(video.notes ?? "");
  const [mode, setMode] = useState<CoverMode>(video.customCoverAt > 0 ? "custom" : "auto");
  const [allowScrapedCover, setAllowScrapedCover] = useState(video.allowScrapedCover !== false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const durationSec = video.durationMs > 0 ? video.durationMs / 1000 : 0;
  const [atSec, setAtSec] = useState(() => (durationSec > 0 ? durationSec / 10 : 0));

  const clampAt = (value: number) => {
    if (!Number.isFinite(value)) return 0;
    return Math.min(Math.max(0, value), durationSec > 0 ? durationSec : Number.MAX_SAFE_INTEGER);
  };
  const setTimestamp = (value: number) => setAtSec(clampAt(value));

  // Clear the like counter immediately (no Save needed — it is a direct write,
  // unlike the staged cover/title/notes).
  const clearLikes = async () => {
    try {
      const updated = await client.clearVideoLikes(video.id);
      onSaved(updated);
    } catch (e) {
      setError(String(e));
    }
  };

  // Save commits the staged cover + the text fields together (transactional).
  const save = async () => {
    setError(null);
    setSaving(true);
    try {
      await setVideoScrapedCoverAllowed(video.id, allowScrapedCover);
      if (mode === "custom") {
        await setVideoCover(video.id, atSec);
      } else if (video.customCoverAt > 0) {
        await resetVideoCover(video.id);
      }
      const updated = await client.updateVideoInfo(video.id, title, notes);
      onSaved(updated);
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="flex w-[620px] flex-col">
        <DialogHeader>
          <DialogTitle>编辑影片信息</DialogTitle>
          <DialogDescription className="sr-only">编辑影片标题、备注和封面</DialogDescription>
        </DialogHeader>

        <div className="mt-3 flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto pr-1">
          <div className="mb-1 flex flex-col gap-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)] p-3">
            <div className="flex items-center gap-2.5">
              <span className="shrink-0 text-xs text-[var(--muted)]">当前封面</span>
              <img
                className="block max-h-[68px] max-w-[120px] rounded-md border border-[var(--border)] bg-[var(--bg)] object-cover"
                src={videoCoverURL(video)}
                alt="当前封面"
              />
            </div>
            <div className="flex gap-4">
              <label className="inline-flex cursor-pointer items-center gap-1.5 text-[13px]">
                <input type="radio" name="cover-mode" checked={mode === "auto"} onChange={() => setMode("auto")} />
                自动封面
              </label>
              <label className="inline-flex cursor-pointer items-center gap-1.5 text-[13px]">
                <input type="radio" name="cover-mode" checked={mode === "custom"} onChange={() => setMode("custom")} />
                指定时间抽帧
              </label>
            </div>
            {mode === "custom" ? (
              <div>
                <label className="mb-2 block text-[13px]" htmlFor="cover-time">
                  抽帧时间（秒）
                </label>
                <div className="flex flex-wrap items-center gap-2">
                  <Input
                    id="cover-time"
                    className="w-28 bg-[var(--panel)]"
                    min={0}
                    max={durationSec > 0 ? durationSec : undefined}
                    onChange={(e) => setTimestamp(Number(e.target.value))}
                    step={0.1}
                    type="number"
                    value={atSec}
                  />
                  <Button size="sm" onClick={() => setTimestamp(atSec - 1)}>
                    −1s
                  </Button>
                  <Button size="sm" onClick={() => setTimestamp(atSec - 0.1)}>
                    −0.1s
                  </Button>
                  <Button size="sm" onClick={() => setTimestamp(atSec + 0.1)}>
                    +0.1s
                  </Button>
                  <Button size="sm" onClick={() => setTimestamp(atSec + 1)}>
                    +1s
                  </Button>
                  <span className="text-xs tabular-nums text-[var(--muted)]">
                    {fmtTime(atSec)}
                    {durationSec > 0 ? ` / ${fmtTime(durationSec)}` : ""}
                  </span>
                </div>
                <p className="mt-2 text-xs leading-relaxed text-[var(--muted)]">
                  保存时由 ffmpeg 从源视频抽取该时间点画面，并直接替换当前封面。
                </p>
              </div>
            ) : (
              <p className="text-[13px] text-[var(--muted)]">
                封面跟随播放器默认自动生成。
                {video.customCoverAt > 0 && " 当前为自定义封面，保存后将恢复为自动封面。"}
              </p>
            )}
          </div>

          <label className="inline-flex cursor-pointer items-center gap-2 text-[13px]">
            <input
              type="checkbox"
              checked={allowScrapedCover}
              onChange={(e) => setAllowScrapedCover(e.target.checked)}
            />
            允许刮削封面替换当前封面
          </label>
          <p className="mt-1 text-[13px] text-[var(--muted)]">
            默认开启。关闭后，刮削封面仍会保存，但只作为备用封面，不会覆盖这部影片当前封面。
          </p>

          <label className="mb-2 block font-medium" htmlFor="edit-title">
            自定义标题
          </label>
          <Input
            id="edit-title"
            className="w-full bg-[var(--bg)]"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder={video.javCode?.trim() || video.fileName}
          />
          <label className="mb-2 block font-medium" htmlFor="edit-notes">
            备注
          </label>
          <textarea
            id="edit-notes"
            className="min-h-[60px] w-full resize-y rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg)] px-3 py-2 font-[inherit] text-sm text-[var(--text)] outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            rows={3}
          />
          <p className="mt-2.5 text-[13px] text-[var(--muted)]">标题留空时列表显示刮削番号；尚未刮削时显示文件名。</p>

          <label className="mb-2 block font-medium" htmlFor="edit-likes">点赞数</label>
          <div className="flex items-center gap-2">
            <span className="font-semibold text-[#ff6b81]">♥ {video.likeCount || 0}</span>
            {video.likeCount > 0 && (
              <Button size="sm" onClick={() => void clearLikes()}>
                清空点赞数
              </Button>
            )}
            {video.likeCount === 0 && (
              <span className="text-[13px] text-[var(--muted)]">还没有点赞。在列表点击 ♥ 即可点赞（可多次）。</span>
            )}
          </div>

          {error && <p className="text-[13px] text-[var(--danger)]">{error}</p>}
        </div>

        <DialogFooter>
          <Button onClick={onClose}>
            取消
          </Button>
          <Button variant="primary" onClick={() => void save()} disabled={saving}>
            {saving ? "保存中…" : "保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
