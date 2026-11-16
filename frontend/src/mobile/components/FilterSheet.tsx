import type { Group, SortDir, SortKey } from "../../types";
import type { DiskOption } from "../../utils/diskFilter";
import {
  Divider,
  Section,
  SegmentedControl,
  SegmentedControlItem,
  ToggleButton,
  ToggleButtonGroup,
  toggleChipClass,
} from "../../ui/astryx";
import { Button } from "../../ui/button";
import { Icon } from "../../ui/icon";
import { BottomSheet } from "../../ui/sheet";
import {
  diskOptionLabel,
  filterCount,
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
 *
 * The controls are Astryx section/toggle/segmented components; this file only
 * maps the library's groups, actors, disks and sort fields onto them.
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
          <Icon className="size-4" name="x" />
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

        <Section className="px-2 py-3.5" padding={0} variant="transparent">
          <GroupHeading hint={`${groups.length} 个分组`} title="分组" />
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
          <GroupHeading hint={`${actors.length} 位演员`} title="演员" />
          {actors.length === 0 ? (
            <p className="py-1 text-xs text-[var(--muted)]">当前列表里没有演员信息</p>
          ) : (
            <ToggleButtonGroup
              className="flex flex-wrap gap-1.5"
              label="演员"
              onChange={(values) => onChange({ ...filters, actors: values })}
              size="sm"
              type="multiple"
              value={filters.actors}
            >
              {actors.map((actor) => (
                <ToggleButton className={toggleChipClass} key={actor.name} label={actor.name} value={actor.name}>
                  <span className="flex min-w-0 items-center gap-1.5">
                    <span className="truncate">{actor.name}</span>
                    <span className="text-[10px] opacity-60">{actor.count}</span>
                  </span>
                </ToggleButton>
              ))}
            </ToggleButtonGroup>
          )}
        </Section>

        <Divider />

        <Section className="px-2 py-3.5" padding={0} variant="transparent">
          <GroupHeading hint={`${disks.length} 块硬盘`} title="硬盘" />
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

        <Divider />

        <Section className="px-2 py-3.5" padding={0} variant="transparent">
          <GroupHeading hint="只影响排序" title="排序" />
          <ToggleButtonGroup
            className="flex flex-wrap gap-1.5"
            label="排序字段"
            onChange={(value) => value && onSortKey(value as SortKey)}
            size="sm"
            type="single"
            value={sortKey}
          >
            {SORT_KEYS.map((key) => (
              <ToggleButton className={toggleChipClass} key={key} label={SORT_LABELS[key]} value={key} />
            ))}
          </ToggleButtonGroup>
          <SegmentedControl
            className="mt-2"
            label="排序方向"
            layout="hug"
            onChange={(value) => onSortDir(value as SortDir)}
            size="sm"
            value={sortDir}
          >
            <SegmentedControlItem
              icon={<Icon className="size-3" name="arrow-down" />}
              label="降序"
              value="desc"
            />
            <SegmentedControlItem
              icon={<Icon className="size-3" name="arrow-up" />}
              label="升序"
              value="asc"
            />
          </SegmentedControl>
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
          <Button className="h-10 flex-1 text-sm" onClick={onClose} variant="primary">
            {count === 0 ? "应用" : `应用 · ${count} 项`}
          </Button>
        </div>
      </div>
    </BottomSheet>
  );
}

function GroupHeading({ title, hint }: { title: string; hint: string }) {
  return (
    <div className="mb-2.5 flex items-baseline justify-between gap-2">
      <h3 className="text-[13px] font-semibold text-[var(--text)]">{title}</h3>
      <span className="text-[11px] text-[var(--muted)]">{hint}</span>
    </div>
  );
}
