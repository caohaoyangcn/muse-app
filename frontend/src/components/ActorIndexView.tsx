import { useEffect, useRef, useState } from "react";
import { actorFillStatus, mergeActors, startActorFill, unmergeActor } from "../services/desktop";
import { invalidate } from "../services/live";
import { useLiveQuery } from "../hooks/useLiveQuery";
import { usePagedList } from "../hooks/usePagedList";
import type { ActorFillStatus, ActorSummary } from "../types";
import { MIME_ACTOR_DRAG } from "../types";
import { actorAvatarURL } from "../utils/format";
import { Avatar, Badge, DropdownMenuItem, EmptyState, Spinner } from "../ui/astryx";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Pagination } from "../ui/pagination";
import { cn } from "../lib/utils";
import { MenuSurface } from "../ui/menu";
import { useActorIndex } from "../views/actors/useActorIndex";

/** One page of actor chips; the index can carry several hundred. */
const PAGE_SIZE = 50;

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
  const listKey = `desktop:actors:${keyword}`;
  const paging = usePagedList({ total: filtered.length, listKey, pageSize: PAGE_SIZE });
  const listTopRef = useRef<HTMLDivElement | null>(null);
  const firstListKey = useRef(true);
  useEffect(() => {
    if (firstListKey.current) {
      firstListKey.current = false;
      return;
    }
    listTopRef.current?.scrollIntoView({ block: "start" });
  }, [listKey]);
  const pageActors = filtered.slice(paging.startIndex, paging.endIndex);
  const turnPage = (next: number) => {
    paging.goToPage(next);
    listTopRef.current?.scrollIntoView({ block: "start" });
  };
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
      <div className="scroll-mt-4" ref={listTopRef} />
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <h2 className="text-sm font-semibold">演员</h2>
        <span className="text-xs text-[var(--muted)]">
          {actors.length} 位演员
          {filtered.length !== actors.length ? ` · 匹配 ${filtered.length} 位` : ""}
        </span>
        <span className="text-xs text-[var(--muted)]">拖动一个演员到另一个上可合并同一人的不同艺名</span>
        <Button
          disabled={startingFill || (fill?.active ?? false)}
          onClick={handleStartFill}
          size="sm"
          title="为还没有头像与档案的演员逐个查询数据源"
          variant="ghost"
        >
          {fill?.active ? "补全中…" : "补全演员信息"}
        </Button>
        <div className="ml-auto w-[240px] max-w-full">
          <Input
            aria-label="筛选演员"
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
        <div className="flex min-h-[320px] items-center justify-center">
          <Spinner label="加载中…" size="lg" />
        </div>
      ) : error ? (
        <EmptyState description={error} title="加载演员失败" />
      ) : actors.length === 0 ? (
        <EmptyState description="刮削完成后，演员会在这里聚合展示" title="暂无演员" />
      ) : filtered.length === 0 ? (
        <EmptyState description={`没有匹配“${keyword.trim()}”的演员`} title="无结果" />
      ) : (
        <>
          <div className="flex flex-wrap gap-2">
            {pageActors.map((a) => {
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
                <Button
                  className="h-auto px-0 text-sm text-[var(--text)] hover:text-[var(--accent)]"
                  onClick={() => onOpenActor(a.name)}
                  size="sm"
                  variant="ghost"
                >
                  {a.name}
                </Button>
                {a.aliases && a.aliases.length > 0 && (
                  <span className="text-xs text-[var(--muted)]">
                    +{a.aliases.join("、")}
                  </span>
                )}
                <Badge label={String(a.count)} variant="neutral" />
              </div>
            );
          })}
          </div>
          <Pagination
            page={paging.page}
            pageCount={paging.pageCount}
            total={filtered.length}
            unit="位"
            onPageChange={turnPage}
          />
        </>
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

/** One actor's avatar, falling back to initials. */
function ActorAvatar({ actor }: { actor: ActorSummary }) {
  return <Avatar name={actor.name} size="sm" src={actorAvatarURL(actor.profile) || undefined} />;
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
  const aliases = actor.aliases ?? [];
  return (
    <MenuSurface label={`${actor.name} 操作`} onClose={onClose} x={x} y={y}>
      {aliases.length === 0 ? (
        <DropdownMenuItem isDisabled label="没有已合并的艺名" />
      ) : (
        aliases.map((alias) => (
          <DropdownMenuItem
            key={alias}
            label={`解除合并「${alias}」`}
            onClick={() => {
              onUnmerge(alias);
              onClose();
            }}
          />
        ))
      )}
    </MenuSurface>
  );
}
