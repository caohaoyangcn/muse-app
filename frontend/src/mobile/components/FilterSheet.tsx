import type { ReactNode } from "react";

import type { Group, SortDir, SortKey } from "../../types";
import type { DiskOption } from "../../utils/diskFilter";
import { cn } from "../../lib/utils";
import { Button } from "../../ui/button";
import { NavIcon } from "../../ui/NavIcon";
import { BottomSheet } from "../../ui/sheet";
import {
  diskOptionLabel,
  filterCount,
  toggleFilterValue,
  type ActorOption,
  type LibraryFilters,
} from "../libraryFilters";

const SORT_LABELS: Record<SortKey, string> = {
  createdAt: "添加时间",
  lastOpenedAt: "最近播放",
  fileSize: "文件大小",
  fileName: "文件名",
  likeCount: "点赞数",
};

const SORT_KEYS = Object.keys(SORT_LABELS) as SortKey[];

/**
 * The library's filter sheet.
 *
 * Every tap edits the live selection in the parent, not a private draft. That
 * is deliberate: closing the sheet with the drag handle, the backdrop or the
 * Android back gesture can therefore never discard choices — there is no second
 * copy to fall out of sync. "应用" only closes the sheet; the home screen has
 * already been showing the filtered rows and the chips behind it.
 */
export function FilterSheet({
  open,
  onClose,
  groups,
  actors,
  disks,
  filters,
  onChange,
  onReset,
  sortKey,
  sortDir,
  onSortKey,
  onSortDir,
}: {
  open: boolean;
  onClose: () => void;
  groups: Group[];
  actors: ActorOption[];
  disks: DiskOption[];
  filters: LibraryFilters;
  onChange: (next: LibraryFilters) => void;
  onReset: () => void;
  sortKey: SortKey;
  sortDir: SortDir;
  onSortKey: (key: SortKey) => void;
  onSortDir: (dir: SortDir) => void;
}) {
  const count = filterCount(filters);

  return (
    <BottomSheet
      headerEnd={
        <Button
          aria-label="关闭筛选"
          className="size-8 p-0 text-[var(--muted)]"
          onClick={onClose}
          size="sm"
          variant="ghost"
        >
          <NavIcon className="size-4" name="x" />
        </Button>
      }
      height="auto"
      onClose={onClose}
      open={open}
      title="筛选与排序"
    >
      <div className="px-1">
        <p className="px-2 pb-1 pt-0.5 text-[11px] leading-relaxed text-[var(--muted)]">
          {count === 0
            ? "未选择条件，显示全部收藏。同一栏内可多选。"
            : `已选 ${count} 个条件；同一栏内取并集，不同栏之间取交集。`}
        </p>

        <Section hint={`${groups.length} 个分组`} title="分组">
          {groups.length === 0 ? (
            <EmptyHint text="还没有分组" />
          ) : (
            <ChipWrap>
              {groups.map((group) => (
                <OptionChip
                  count={group.videoCount}
                  key={group.id}
                  label={group.name}
                  onClick={() =>
                    onChange({
                      ...filters,
                      groupIds: toggleFilterValue(filters.groupIds, group.id),
                    })
                  }
                  selected={filters.groupIds.includes(group.id)}
                />
              ))}
            </ChipWrap>
          )}
        </Section>

        <Section hint={`${actors.length} 位演员`} title="演员">
          {actors.length === 0 ? (
            <EmptyHint text="当前列表里没有演员信息" />
          ) : (
            <ChipWrap>
              {actors.map((actor) => (
                <OptionChip
                  count={actor.count}
                  key={actor.name}
                  label={actor.name}
                  onClick={() =>
                    onChange({
                      ...filters,
                      actors: toggleFilterValue(filters.actors, actor.name),
                    })
                  }
                  selected={filters.actors.includes(actor.name)}
                />
              ))}
            </ChipWrap>
          )}
        </Section>

        <Section hint={`${disks.length} 块硬盘`} title="硬盘">
          {disks.length === 0 ? (
            <EmptyHint text="当前列表没有可识别的硬盘" />
          ) : (
            <ChipWrap>
              {disks.map((disk) => (
                <OptionChip
                  count={disk.count}
                  key={disk.key}
                  label={`${diskOptionLabel(disk)}${disk.offline ? "（离线）" : ""}`}
                  onClick={() =>
                    onChange({
                      ...filters,
                      disks: toggleFilterValue(filters.disks, disk.key),
                    })
                  }
                  selected={filters.disks.includes(disk.key)}
                />
              ))}
            </ChipWrap>
          )}
        </Section>

        <Section hint="只影响排序" title="排序">
          <ChipWrap>
            {SORT_KEYS.map((key) => (
              <OptionChip
                key={key}
                label={SORT_LABELS[key]}
                onClick={() => onSortKey(key)}
                selected={key === sortKey}
              />
            ))}
          </ChipWrap>
          <div className="mt-2 flex gap-1.5">
            {(["desc", "asc"] as SortDir[]).map((dir) => (
              <Button
                aria-pressed={sortDir === dir}
                className={cn(
                  "h-8 rounded-full px-3 text-xs",
                  sortDir === dir
                    ? "border-white/15 bg-white/[0.09] text-[var(--text)]"
                    : "border-white/[0.08] bg-white/[0.035] text-[var(--muted)]",
                )}
                key={dir}
                onClick={() => onSortDir(dir)}
                size="sm"
                variant="ghost"
              >
                <span className="inline-flex items-center gap-1">
                  {dir === "desc" ? "降序" : "升序"}
                  <NavIcon
                    className={cn("size-3", dir === "asc" && "rotate-180")}
                    name="chevron-down"
                  />
                </span>
              </Button>
            ))}
          </div>
        </Section>

        {/* Live selection, sticky action bar. Closing any other way keeps the
            selection because onChange already committed it. */}
        <div className="sticky bottom-0 z-10 -mx-1 mt-1 flex items-center gap-2 border-t border-[var(--border)] bg-[var(--panel)] px-3 pb-3 pt-3">
          <Button
            className="h-10 shrink-0 px-4 text-sm"
            disabled={count === 0}
            onClick={onReset}
            variant="ghost"
          >
            重置
          </Button>
          <Button className="h-10 flex-1 text-sm" onClick={onClose} variant="primary">
            {count === 0 ? "应用" : `应用 · ${count} 项`}
          </Button>
        </div>
      </div>
    </BottomSheet>
  );
}

function Section({
  title,
  hint,
  children,
}: {
  title: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <section className="border-b border-white/[0.05] px-2 py-3.5">
      <div className="mb-2.5 flex items-baseline justify-between gap-2">
        <h3 className="text-[13px] font-semibold text-[var(--text)]">{title}</h3>
        {hint ? <span className="text-[11px] text-[var(--muted)]">{hint}</span> : null}
      </div>
      {children}
    </section>
  );
}

function ChipWrap({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap gap-1.5">{children}</div>;
}

function EmptyHint({ text }: { text: string }) {
  return <p className="py-1 text-xs text-[var(--muted)]">{text}</p>;
}

function OptionChip({
  label,
  count,
  selected,
  onClick,
}: {
  label: string;
  count?: number;
  selected: boolean;
  onClick: () => void;
}) {
  return (
    <Button
      aria-pressed={selected}
      className={cn(
        "h-8 min-w-0 max-w-full gap-1.5 rounded-full px-3 text-xs",
        selected
          ? "border-transparent bg-[var(--accent)] text-white"
          : "border-white/[0.09] bg-white/[0.055] text-[var(--text)]",
      )}
      onClick={onClick}
      size="sm"
      variant="ghost"
    >
      <span className="min-w-0 truncate">{label}</span>
      {typeof count === "number" ? (
        <span
          className={cn(
            "shrink-0 text-[10px]",
            selected ? "text-white/75" : "text-[var(--muted)]",
          )}
        >
          {count}
        </span>
      ) : null}
      {selected ? <NavIcon className="size-3 shrink-0" name="check" /> : null}
    </Button>
  );
}
