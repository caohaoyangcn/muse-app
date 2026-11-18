import { useEffect, useMemo, useState, type ReactNode } from "react";

import {
  Divider,
  Section,
  ToggleButton,
  ToggleButtonGroup,
  toggleChipClass,
} from "../../ui/astryx";
import { Button } from "../../ui/button";
import { Icon } from "../../ui/icon";
import { Input } from "../../ui/input";
import type { DiskOption } from "../../utils/diskFilter";
import {
  OPTION_PAGE_SIZE,
  type ValueOption,
  diskOptionLabel,
  pageValueOptions,
  type LibraryFilters,
} from "../../utils/libraryFilter";

/**
 * The three condition sections shared by the phone's filter sheet and the
 * desktop app's filter dialog. It renders only controls; the platform wrapper
 * owns the surface (bottom sheet vs dialog), the footer and when to close.
 *
 * Tags and actors can both be huge (a real library carries hundreds), so each
 * gets a search box and a 20-item page. Selected values stay pinned regardless
 * of the page or keyword — a condition the surface cannot show would be a
 * condition the user cannot undo.
 */
export function FilterSections({
  open,
  tags,
  actors,
  disks,
  filters,
  hiddenTags = [],
  onChange,
  onUnhideTag,
  onClearHiddenTags,
}: {
  /** Reopening the surface resets the two search boxes and their pages. */
  open: boolean;
  tags: ValueOption[];
  actors: ValueOption[];
  disks: DiskOption[];
  filters: LibraryFilters;
  /** Tags hidden from the sidebar shortcut; still selectable here. */
  hiddenTags?: readonly string[];
  onChange: (next: LibraryFilters) => void;
  /** Restore one hidden quick tag. Omit on a surface without quick tags. */
  onUnhideTag?: (tag: string) => void;
  onClearHiddenTags?: () => void;
}) {
  const [tagKeyword, setTagKeyword] = useState("");
  const [tagLimit, setTagLimit] = useState(OPTION_PAGE_SIZE);
  const [actorKeyword, setActorKeyword] = useState("");
  const [actorLimit, setActorLimit] = useState(OPTION_PAGE_SIZE);

  useEffect(() => {
    if (!open) return;
    setTagKeyword("");
    setTagLimit(OPTION_PAGE_SIZE);
    setActorKeyword("");
    setActorLimit(OPTION_PAGE_SIZE);
  }, [open]);

  const tagPage = useMemo(
    () => pageValueOptions(tags, filters.tags, tagKeyword, tagLimit),
    [tags, filters.tags, tagKeyword, tagLimit],
  );
  const actorPage = useMemo(
    () => pageValueOptions(actors, filters.actors, actorKeyword, actorLimit),
    [actors, filters.actors, actorKeyword, actorLimit],
  );

  return (
    <div className="px-1">
      <Section className="px-2 py-3.5" padding={0} variant="transparent">
        <SectionHeading
          action={
            tagPage.hidden > 0 ? (
              <Button
                className="h-7 shrink-0 gap-0.5 rounded-full px-1.5 text-[11px] text-[var(--accent)]"
                icon={<Icon className="size-3" name="chevron-down" />}
                onClick={() => setTagLimit((current) => current + OPTION_PAGE_SIZE)}
                size="sm"
                variant="ghost"
              >
                展示更多 {tagPage.hidden}
              </Button>
            ) : undefined
          }
          hint={`${tags.length} 个标签`}
          title="标签"
        />
        {tags.length === 0 ? (
          <p className="py-1 text-xs text-[var(--muted)]">还没有标签——刮削成功后会出现在这里</p>
        ) : (
          <>
            <Input
              aria-label="搜索标签"
              onChange={(event) => {
                setTagKeyword(event.target.value);
                setTagLimit(OPTION_PAGE_SIZE);
              }}
              placeholder="搜索标签"
              type="search"
              value={tagKeyword}
            />
            {tagPage.matched === 0 ? (
              <p className="px-1 py-2 text-xs text-[var(--muted)]">
                没有匹配「{tagKeyword.trim()}」的标签
              </p>
            ) : (
              <ToggleButtonGroup
                className="mt-2 flex flex-wrap gap-1.5"
                label="标签"
                onChange={(values) => onChange({ ...filters, tags: values })}
                size="sm"
                type="multiple"
                value={filters.tags}
              >
                {tagPage.visible.map((tag) => (
                  <ToggleButton className={toggleChipClass} key={tag.name} label={tag.name} value={tag.name}>
                    <span className="flex min-w-0 items-center gap-1.5">
                      <span className="truncate">{tag.name}</span>
                      <span className="text-[10px] opacity-60">{tag.count}</span>
                    </span>
                  </ToggleButton>
                ))}
              </ToggleButtonGroup>
            )}
          </>
        )}
        {hiddenTags.length > 0 ? (
          <div className="mt-2 border-t border-[var(--border)] px-1 pt-2">
            <div className="flex items-center justify-between gap-2">
              <span className="text-[11px] text-[var(--muted)]">
                已从快捷入口隐藏 · {hiddenTags.length}
              </span>
              {onClearHiddenTags ? (
                <Button
                  className="h-7 shrink-0 rounded-full px-2 text-[11px] text-[var(--accent)]"
                  onClick={onClearHiddenTags}
                  size="sm"
                  variant="ghost"
                >
                  全部恢复
                </Button>
              ) : null}
            </div>
            <div className="mt-1.5 flex flex-wrap gap-1.5">
              {hiddenTags.map((tag) => (
                <Button
                  className="h-7 gap-1 rounded-full border border-[var(--border)] bg-[var(--panel-2)] px-2 text-xs text-[var(--muted)]"
                  icon={<Icon className="size-3" name="plus" />}
                  key={tag}
                  onClick={() => onUnhideTag?.(tag)}
                  size="sm"
                  title="恢复到侧栏快捷入口"
                  variant="ghost"
                >
                  <span className="max-w-[9rem] truncate">{tag}</span>
                </Button>
              ))}
            </div>
          </div>
        ) : null}
      </Section>

      <Divider />

      <Section className="px-2 py-3.5" padding={0} variant="transparent">
        <SectionHeading
          action={
            actorPage.hidden > 0 ? (
              <Button
                className="h-7 shrink-0 gap-0.5 rounded-full px-1.5 text-[11px] text-[var(--accent)]"
                icon={<Icon className="size-3" name="chevron-down" />}
                onClick={() => setActorLimit((current) => current + OPTION_PAGE_SIZE)}
                size="sm"
                variant="ghost"
              >
                展示更多 {actorPage.hidden}
              </Button>
            ) : undefined
          }
          hint={
            [
              actorKeyword.trim() ? `${actorPage.matched} 位匹配` : `${actors.length} 位演员`,
              filters.actors.length > 0 ? `已选 ${filters.actors.length}` : "",
            ]
              .filter(Boolean)
              .join(" · ")
          }
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
                setActorLimit(OPTION_PAGE_SIZE);
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
        <SectionHeading hint={`${disks.length} 块硬盘`} title="硬盘" />
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
    </div>
  );
}

function SectionHeading({
  title,
  hint,
  action,
}: {
  title: string;
  hint: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex items-center justify-between gap-2 px-1 pb-1.5">
      <div className="flex min-w-0 items-baseline gap-2">
        <span className="text-[13px] font-semibold text-[var(--text)]">{title}</span>
        <span className="truncate text-[11px] text-[var(--muted)]">{hint}</span>
      </div>
      {action}
    </div>
  );
}
