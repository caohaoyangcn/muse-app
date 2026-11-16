import { useMemo, useState } from "react";

import type { Group } from "../../types";
import type { DiskOption } from "../../utils/diskFilter";
import {
  Divider,
  Section,
  IconButton,
  ToggleButton,
  ToggleButtonGroup,
  toggleChipClass,
} from "../../ui/astryx";
import { Button } from "../../ui/button";
import { Icon } from "../../ui/icon";
import { Input } from "../../ui/input";
import { BottomSheet } from "../../ui/sheet";
import {
  ACTOR_PAGE_SIZE,
  diskOptionLabel,
  filterCount,
  pageActorOptions,
  type ActorOption,
  type LibraryFilters,
} from "../libraryFilters";
import { SheetHeading } from "./SheetHeading";

/**
 * The library's filter sheet — conditions only. Ordering lives in SortSheet:
 * a filter decides *which* videos the phone has, a sort only their order, and
 * one sheet for both made the sort read like another condition.
 *
 * Every tap edits the live selection in the parent, not a private draft. That
 * is deliberate: closing the sheet with the drag handle, the backdrop or the
 * Android back gesture can therefore never discard choices — there is no second
 * copy to fall out of sync. "应用" only closes the sheet; the home screen has
 * already been showing the filtered rows and the chips behind it.
 *
 * The actor list is the one section that can be huge (a real library carries
 * hundreds), so it gets its own search box and shows 20 at a time. Selected
 * actors stay pinned regardless of the page or the keyword — a condition the
 * sheet cannot show would be a condition the user cannot undo.
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
}: {
  open: boolean;
  onClose: () => void;
  groups: Group[];
  actors: ActorOption[];
  disks: DiskOption[];
  filters: LibraryFilters;
  onChange: (next: LibraryFilters) => void;
  onReset: () => void;
}) {
  const count = filterCount(filters);
  const [actorKeyword, setActorKeyword] = useState("");
  const [actorLimit, setActorLimit] = useState(ACTOR_PAGE_SIZE);

  const actorPage = useMemo(
    () => pageActorOptions(actors, filters.actors, actorKeyword, actorLimit),
    [actors, filters.actors, actorKeyword, actorLimit],
  );
  const searching = actorKeyword.trim() !== "";
  const actorHint = [
    searching ? `${actorPage.matched} 位匹配` : `${actors.length} 位演员`,
    filters.actors.length > 0 ? `已选 ${filters.actors.length}` : "",
  ]
    .filter(Boolean)
    .join(" · ");

  // Reopening the sheet starts from the full list: a stale keyword would look
  // like the library lost actors.
  const close = () => {
    setActorKeyword("");
    setActorLimit(ACTOR_PAGE_SIZE);
    onClose();
  };

  return (
    <BottomSheet
      headerEnd={
        <IconButton
          className="size-8 p-0 text-[var(--muted)]"
          icon={<Icon className="size-4" name="x" />}
          label="关闭筛选"
          onClick={close}
          size="sm"
          tooltip="关闭筛选"
          variant="ghost"
        />
      }
      height="hug"
      onClose={close}
      open={open}
      title="筛选"
    >
      <div className="px-1">
        <p className="px-2 pb-1 pt-0.5 text-[11px] leading-relaxed text-[var(--muted)]">
          {count === 0
            ? "未选择条件，显示全部收藏。同一栏内可多选。"
            : `已选 ${count} 个条件；同一栏内取并集，不同栏之间取交集。`}
        </p>

        <Section className="px-2 py-3.5" padding={0} variant="transparent">
          <SheetHeading hint={`${groups.length} 个分组`} title="分组" />
          {groups.length === 0 ? (
            <p className="py-1 text-xs text-[var(--muted)]">还没有分组</p>
          ) : (
            <ToggleButtonGroup
              className="flex flex-wrap gap-1.5"
              label="分组"
              onChange={(values) =>
                onChange({ ...filters, groupIds: values.map((value) => Number(value)) })
              }
              size="sm"
              type="multiple"
              value={filters.groupIds.map(String)}
            >
              {groups.map((group) => (
                <ToggleButton className={toggleChipClass} key={group.id} label={group.name} value={String(group.id)}>
                  <span className="flex min-w-0 items-center gap-1.5">
                    <span className="truncate">{group.name}</span>
                    <span className="text-[10px] opacity-60">{group.videoCount}</span>
                  </span>
                </ToggleButton>
              ))}
            </ToggleButtonGroup>
          )}
        </Section>

        <Divider />

        <Section className="px-2 py-3.5" padding={0} variant="transparent">
          <SheetHeading
            action={
              actorPage.hidden > 0 ? (
                <Button
                  className="h-7 shrink-0 gap-0.5 rounded-full px-1.5 text-[11px] text-[var(--accent)]"
                  icon={<Icon className="size-3" name="chevron-down" />}
                  onClick={() => setActorLimit((current) => current + ACTOR_PAGE_SIZE)}
                  size="sm"
                  variant="ghost"
                >
                  展示更多 {actorPage.hidden}
                </Button>
              ) : undefined
            }
            hint={actorHint}
            title="演员"
          />
          {actors.length === 0 ? (
            <p className="py-1 text-xs text-[var(--muted)]">当前列表里没有演员信息</p>
          ) : (
            <>
              <Input
                aria-label="搜索演员"
                onChange={(event) => {
                  setActorKeyword(event.target.value);
                  setActorLimit(ACTOR_PAGE_SIZE);
                }}
                placeholder="搜索演员"
                type="search"
                value={actorKeyword}
              />
              {actorPage.matched === 0 ? (
                <p className="px-1 py-2 text-xs text-[var(--muted)]">
                  没有匹配「{actorKeyword.trim()}」的演员
                </p>
              ) : (
                <ToggleButtonGroup
                  className="mt-2 flex flex-wrap gap-1.5"
                  label="演员"
                  onChange={(values) => onChange({ ...filters, actors: values })}
                  size="sm"
                  type="multiple"
                  value={filters.actors}
                >
                  {actorPage.visible.map((actor) => (
                    <ToggleButton className={toggleChipClass} key={actor.name} label={actor.name} value={actor.name}>
                      <span className="flex min-w-0 items-center gap-1.5">
                        <span className="truncate">{actor.name}</span>
                        <span className="text-[10px] opacity-60">{actor.count}</span>
                      </span>
                    </ToggleButton>
                  ))}
                </ToggleButtonGroup>
              )}
            </>
          )}
        </Section>

        <Divider />

        <Section className="px-2 py-3.5" padding={0} variant="transparent">
          <SheetHeading hint={`${disks.length} 块硬盘`} title="硬盘" />
          {disks.length === 0 ? (
            <p className="py-1 text-xs text-[var(--muted)]">当前列表没有可识别的硬盘</p>
          ) : (
            <ToggleButtonGroup
              className="flex flex-wrap gap-1.5"
              label="硬盘"
              onChange={(values) => onChange({ ...filters, disks: values })}
              size="sm"
              type="multiple"
              value={filters.disks}
            >
              {disks.map((disk) => (
                <ToggleButton className={toggleChipClass} key={disk.key} label={diskOptionLabel(disk)} value={disk.key}>
                  <span className="flex min-w-0 items-center gap-1.5">
                    <span className="truncate">{diskOptionLabel(disk)}</span>
                    <span className="text-[10px] opacity-60">{disk.count}</span>
                    {disk.offline ? <Icon className="size-3 text-[var(--danger)]" name="alert" /> : null}
                  </span>
                </ToggleButton>
              ))}
            </ToggleButtonGroup>
          )}
        </Section>

        {/* Live selection: Astryx owns the controls; closing the sheet keeps
            them because onChange has already committed the state. */}
        <div className="sticky bottom-0 z-10 -mx-1 mt-1 flex items-center gap-2 border-t border-[var(--border)] bg-[var(--panel)] px-3 pb-3 pt-3">
          <Button
            className="h-10 shrink-0 px-4 text-sm"
            disabled={count === 0}
            onClick={onReset}
            variant="ghost"
          >
            重置
          </Button>
          <Button className="h-10 flex-1 text-sm" onClick={close} variant="primary">
            {count === 0 ? "应用" : `应用 · ${count} 项`}
          </Button>
        </div>
      </div>
    </BottomSheet>
  );
}
