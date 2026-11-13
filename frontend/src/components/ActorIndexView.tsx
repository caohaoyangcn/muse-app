import { useEffect, useRef, useState } from "react";
import { actorFillStatus, mergeActors, startActorFill, unmergeActor } from "../services/desktop";
import { invalidate } from "../services/live";
import { useLiveQuery } from "../hooks/useLiveQuery";
import type { ActorFillStatus, ActorSummary } from "../types";
import { MIME_ACTOR_DRAG } from "../types";
import { actorAvatarURL } from "../utils/format";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { cn } from "../lib/utils";
import { useActorIndex } from "../views/actors/useActorIndex";

/**
 * Aggregated actor index (演员): one chip per actor, with the number of
 * favorites they appear in. Clicking a chip opens that actor's film list as its
 * own workspace tab. The counts come from a live query, so a scrape that adds a
 * favorite, or a removal, updates them without leaving the view.
 *
 * Two stage names can be merged (演员合并) to model one performer scraped under
 * two names: drag one chip onto another. Merged names show as small aliases on
 * the chip, and right-clicking offers to split them apart again (解除合并).
 */
export default function ActorIndexView({
  onOpenActor,
  onMerged,
  onNotify,
}: {
  onOpenActor: (name: string) => void;
  /** Called after a merge/unmerge so the caller can refresh open actor tabs. */
  onMerged: () => void;
  onNotify: (message: string, kind: "success" | "error") => void;
}) {
  const { actors, filtered, keyword, setKeyword, loading, error } = useActorIndex();
  const [dragName, setDragName] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<string | null>(null);
  const [menu, setMenu] = useState<{ x: number; y: number; actor: ActorSummary } | null>(null);
  const dragCounter = useRef(0);
  // 演员档案补全的进度：后台逐个查询，队列状态反过来驱动这里的按钮与说明。
  const { data: fill } = useLiveQuery(actorFillStatus, "actorFill");
  const [startingFill, setStartingFill] = useState(false);

  const handleStartFill = () => {
    setStartingFill(true);
    startActorFill()
      .then((queued) => {
        onNotify(
          queued > 0
            ? `已加入 ${queued} 位演员的补全队列，完成后头像与档案会自动出现`
            : "所有演员都已有档案或已查询过",
          "success",
        );
        invalidate();
      })
      .catch((e) => onNotify(`补全失败：${String(e)}`, "error"))
      .finally(() => setStartingFill(false));
  };

  // Drag one chip onto another to merge the dragged actor into the target. The
  // dragged name rides MIME_ACTOR_DRAG so a video drag can never trigger this.
  const handleDragStart = (e: React.DragEvent, actor: ActorSummary) => {
    e.dataTransfer.effectAllowed = "link";
    e.dataTransfer.setData(MIME_ACTOR_DRAG, actor.name);
    e.dataTransfer.setData("text/plain", actor.name);
    setDragName(actor.name);
  };

  const handleDragEnd = () => {
    setDragName(null);
    setDropTarget(null);
    dragCounter.current = 0;
  };

  const readDraggedName = (e: React.DragEvent): string | null => {
    if (!Array.from(e.dataTransfer.types).includes(MIME_ACTOR_DRAG)) return null;
    const name = e.dataTransfer.getData(MIME_ACTOR_DRAG);
    return name && name !== "" ? name : dragName;
  };

  const handleDragOver = (e: React.DragEvent, target: ActorSummary) => {
    if (!Array.from(e.dataTransfer.types).includes(MIME_ACTOR_DRAG)) return;
    if (dragName === target.name) return; // no self-drop
    e.preventDefault();
    e.dataTransfer.dropEffect = "link";
    setDropTarget(target.name);
  };

  const handleDrop = (e: React.DragEvent, target: ActorSummary) => {
    const source = readDraggedName(e);
    setDropTarget(null);
    setDragName(null);
    dragCounter.current = 0;
    if (!source || source === target.name) return;
    e.preventDefault();
    mergeActors(source, target.name)
      .then(() => {
        onNotify(`已将「${source}」合并到「${target.name}」`, "success");
        onMerged();
        invalidate();
      })
      .catch((err) => onNotify(`合并失败：${String(err)}`, "error"));
  };

  const handleUnmerge = (actor: ActorSummary, alias: string) => {
    unmergeActor(alias)
      .then(() => {
        onNotify(`已解除「${alias}」与「${actor.name}」的合并`, "success");
        onMerged();
        invalidate();
      })
      .catch((err) => onNotify(`解除合并失败：${String(err)}`, "error"));
  };

  return (
    <div className="pt-4">
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <h2 className="text-sm font-semibold">演员</h2>
        <span className="text-xs text-[var(--muted)]">
          {actors.length} 位演员
          {filtered.length !== actors.length ? ` · 匹配 ${filtered.length} 位` : ""}
        </span>
        <span className="text-xs text-[var(--muted)]">拖动一个演员到另一个上可合并同一人的不同艺名</span>
        <button
          type="button"
          onClick={handleStartFill}
          disabled={startingFill || (fill?.active ?? false)}
          title="为还没有头像与档案的演员逐个查询数据源"
          className="rounded-[var(--radius)] border border-[var(--border)] px-2.5 py-1 text-xs hover:border-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-50"
        >
          {fill?.active ? "补全中…" : "补全演员信息"}
        </button>
        <div className="ml-auto w-[240px] max-w-full">
          <Input
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            placeholder="筛选演员"
            autoComplete="off"
            spellCheck={false}
          />
        </div>
      </div>

      {fill && fill.total > 0 && (
        <p className="-mt-2 mb-3 text-xs text-[var(--muted)]">{fillSummary(fill)}</p>
      )}

      {loading ? (
        <div className="flex min-h-[320px] flex-col items-center justify-center text-[var(--muted)]">
          加载中…
        </div>
      ) : error ? (
        <div className="flex min-h-[320px] flex-col items-center justify-center text-[var(--danger)]">
          加载演员失败：{error}
        </div>
      ) : actors.length === 0 ? (
        <div className="flex min-h-[320px] flex-col items-center justify-center text-center text-[var(--muted)]">
          <div className="mb-1.5 text-xl text-[var(--text)]">暂无演员</div>
          <div>刮削完成后，演员会在这里聚合展示</div>
        </div>
      ) : filtered.length === 0 ? (
        <div className="flex min-h-[320px] flex-col items-center justify-center text-center text-[var(--muted)]">
          <div className="mb-1.5 text-xl text-[var(--text)]">无结果</div>
          <div>没有匹配“{keyword.trim()}”的演员</div>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2">
          {filtered.map((a) => {
            const dragging = dragName === a.name;
            const over = dropTarget === a.name && dragName !== a.name;
            return (
              <div
                key={a.name}
                draggable
                onDragStart={(e) => handleDragStart(e, a)}
                onDragEnd={handleDragEnd}
                onDragOver={(e) => handleDragOver(e, a)}
                onDragLeave={() => setDropTarget((cur) => (cur === a.name ? null : cur))}
                onDrop={(e) => handleDrop(e, a)}
                onContextMenu={(e) => {
                  e.preventDefault();
                  setMenu({ x: e.clientX, y: e.clientY, actor: a });
                }}
                className={cn(
                  "flex cursor-grab items-center gap-2 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)] px-2.5 py-1 text-sm active:cursor-grabbing",
                  "hover:border-[var(--accent)]",
                  dragging && "opacity-50",
                  over && "border-dashed border-[var(--accent)] bg-[rgba(124,92,255,0.18)] shadow-[inset_0_0_0_2px_var(--accent)]",
                )}
                title={`查看 ${a.name} 出演的影片${a.aliases?.length ? `（已合并：${a.aliases.join("、")}）` : ""}`}
              >
                <ActorAvatar actor={a} />
                <button
                  type="button"
                  className="cursor-pointer hover:text-[var(--accent)]"
                  onClick={() => onOpenActor(a.name)}
                >
                  {a.name}
                </button>
                {a.aliases && a.aliases.length > 0 && (
                  <span className="text-xs text-[var(--muted)]">
                    +{a.aliases.join("、")}
                  </span>
                )}
                <span className="rounded-[10px] bg-[var(--border)] px-2 py-px text-xs text-[var(--muted)]">
                  {a.count}
                </span>
              </div>
            );
          })}
        </div>
      )}

      {menu && (
        <ActorMergeMenu
          x={menu.x}
          y={menu.y}
          actor={menu.actor}
          onUnmerge={(alias) => handleUnmerge(menu.actor, alias)}
          onClose={() => setMenu(null)}
        />
      )}
    </div>
  );
}

/** One actor's avatar, falling back to the first character of the name. */
function ActorAvatar({ actor }: { actor: ActorSummary }) {
  const src = actorAvatarURL(actor.profile);
  return (
    <span className="h-7 w-7 shrink-0 overflow-hidden rounded-full border border-[var(--border)] bg-[var(--panel-2)]">
      {src ? (
        <img src={src} alt="" className="h-full w-full object-cover" loading="lazy" />
      ) : (
        <span className="flex h-full w-full items-center justify-center text-[11px] text-[var(--muted)]">
          {actor.name.slice(0, 1)}
        </span>
      )}
    </span>
  );
}

/** The 补全演员信息 progress line shown under the actor index header. */
function fillSummary(fill: ActorFillStatus): string {
  const parts = [`演员档案：已补全 ${fill.succeeded} 位`];
  if (fill.notFound > 0) parts.push(`数据源未收录 ${fill.notFound} 位`);
  if (fill.failed > 0) parts.push(`失败 ${fill.failed} 位`);
  if (fill.queued > 0) parts.push(`队列 ${fill.queued} 位`);
  if (fill.current) parts.push(`正在查询「${fill.current}」`);
  return parts.join(" · ");
}

/** Right-click menu for an actor chip: split one merged alias back out. */
function ActorMergeMenu({
  x,
  y,
  actor,
  onUnmerge,
  onClose,
}: {
  x: number;
  y: number;
  actor: ActorSummary;
  onUnmerge: (alias: string) => void;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const left = Math.min(x, window.innerWidth - 240);
  const top = Math.min(y, window.innerHeight - 200);

  useEffect(() => {
    const close = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", onKey);
    window.addEventListener("blur", onClose);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("blur", onClose);
    };
  }, [onClose]);

  const aliases = actor.aliases ?? [];
  return (
    <div
      ref={ref}
      className="fixed z-[200] min-w-[230px] rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel-2)] p-1.5 shadow-[0_8px_24px_rgba(0,0,0,0.45)]"
      style={{ left, top }}
    >
      <div className="px-3 py-1.5 text-xs text-[var(--muted)]">{actor.name}</div>
      {aliases.length === 0 ? (
        <div className="px-3 py-2 text-sm text-[var(--muted)]">没有已合并的艺名</div>
      ) : (
        aliases.map((alias) => (
          <Button
            variant="ghost"
            key={alias}
            className="h-auto w-full justify-start rounded-md px-3 py-2 text-left text-[var(--text)]"
            onClick={() => {
              onUnmerge(alias);
              onClose();
            }}
          >
            解除合并「{alias}」
          </Button>
        ))
      )}
    </div>
  );
}
