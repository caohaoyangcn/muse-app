import type { Video } from "../types";
import { diskKey, diskLabel, type DiskOption } from "./diskFilter";

/**
 * The library's filter state, shared by the desktop app and the LAN web client.
 *
 * Every field is a multi-select: values inside one field are a union (any of
 * these tags), and fields combine with AND (a video must match the chosen tag,
 * actor and disk sets). Tags are the scraped genre labels projected onto
 * `Video.tags`; they replaced the old user-defined groups. Everything is
 * derived from the rows already loaded, so no transport call is involved.
 */
export interface LibraryFilters {
  tags: string[];
  actors: string[];
  /** Volume identity keys (`diskKey`), not display labels. */
  disks: string[];
}

export const NO_FILTERS: LibraryFilters = { tags: [], actors: [], disks: [] };

export function filterCount(filters: LibraryFilters): number {
  return filters.tags.length + filters.actors.length + filters.disks.length;
}

export function hasFilters(filters: LibraryFilters): boolean {
  return filterCount(filters) > 0;
}

/** Add or remove one value, preserving the order the user picked them in. */
export function toggleFilterValue<T>(values: readonly T[], value: T): T[] {
  return values.includes(value) ? values.filter((item) => item !== value) : [...values, value];
}

/**
 * The local half of the filter. An empty field means "all"; a video must match
 * every non-empty field. Tags and actors use a set so a duplicate label in the
 * scrape data cannot count twice.
 */
export function filterVideos(videos: Video[], filters: LibraryFilters): Video[] {
  if (!hasFilters(filters)) return videos;
  const tags = new Set(filters.tags);
  const actors = new Set(filters.actors);
  const disks = new Set(filters.disks);
  return videos.filter((video) => {
    if (tags.size > 0 && !video.tags?.some((tag) => tags.has(tag))) return false;
    if (actors.size > 0 && !video.actors?.some((name) => actors.has(name))) return false;
    if (disks.size > 0 && !disks.has(diskKey(video.volumeRoot))) return false;
    return true;
  });
}

export interface ValueOption {
  name: string;
  /** Videos in the current (search/actor narrowed) list that carry this value. */
  count: number;
}

/** How many option chips a filter surface shows before offering 展示更多. */
export const OPTION_PAGE_SIZE = 20;

/** One page of an option section: what to render, and what is still hidden. */
export interface ValuePage {
  visible: ValueOption[];
  /** Options matching the keyword, before the page limit. */
  matched: number;
  /** Matches the page limit is still hiding. */
  hidden: number;
}

const collator = new Intl.Collator("zh-CN", { numeric: true, sensitivity: "base" });

/**
 * Counts distinct videos per value and orders by count then name. A video names
 * each value once (a set per row), so listing a tag twice is still one film.
 */
function collectValueOptions(
  videos: readonly Video[],
  valuesOf: (video: Video) => readonly string[] | undefined,
): ValueOption[] {
  const counts = new Map<string, number>();
  for (const video of videos) {
    for (const name of new Set(valuesOf(video) ?? [])) {
      if (!name) continue;
      counts.set(name, (counts.get(name) ?? 0) + 1);
    }
  }
  return [...counts.entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count || collator.compare(a.name, b.name));
}

/** Tag choices for the filter surface, derived from the rows currently loaded. */
export function collectTagOptions(videos: readonly Video[]): ValueOption[] {
  return collectValueOptions(videos, (video) => video.tags);
}

/** Actor choices for the filter surface, derived from the rows currently loaded. */
export function collectActorOptions(videos: readonly Video[]): ValueOption[] {
  return collectValueOptions(videos, (video) => video.actors);
}

/**
 * Keeps selected values visible when the rows have moved on (a search or a tag
 * change may leave none of their films on screen). Dropping them silently would
 * leave an invisible condition filtering the list, so the option stays with a
 * zero count and a visible place in the sheet.
 */
export function withSelectedValues(
  options: readonly ValueOption[],
  selected: readonly string[],
): ValueOption[] {
  const known = new Set(options.map((option) => option.name));
  const missing = selected
    .filter((name) => !known.has(name))
    .map((name) => ({ name, count: 0 }));
  return missing.length === 0 ? [...options] : [...options, ...missing];
}

/** Tag-specific spelling of withSelectedValues. */
export function withSelectedTags(
  options: readonly ValueOption[],
  selected: readonly string[],
): ValueOption[] {
  return withSelectedValues(options, selected);
}

/** Actor-specific spelling of withSelectedValues. */
export function withSelectedActors(
  options: readonly ValueOption[],
  selected: readonly string[],
): ValueOption[] {
  return withSelectedValues(options, selected);
}

/**
 * The value chips a surface renders: the first `limit` matches of the keyword,
 * plus every selected value. A selection made earlier (or one the keyword no
 * longer matches) must stay visible while it still narrows the list.
 */
export function pageValueOptions(
  options: readonly ValueOption[],
  selected: readonly string[],
  keyword: string,
  limit: number = OPTION_PAGE_SIZE,
): ValuePage {
  const needle = keyword.trim().toLowerCase();
  const matched = needle
    ? options.filter((option) => option.name.toLowerCase().includes(needle))
    : [...options];
  const head = matched.slice(0, Math.max(limit, 0));
  const shown = new Set(head.map((option) => option.name));
  const pinned = options.filter(
    (option) => selected.includes(option.name) && !shown.has(option.name),
  );
  return {
    visible: [...head, ...pinned],
    matched: matched.length,
    hidden: Math.max(0, matched.length - head.length),
  };
}

/** A disk's display name. The Unix root is too cryptic on a phone. */
export function diskOptionLabel(option: DiskOption): string {
  const label = option.label.trim();
  return label === "" || label === "/" ? "本机磁盘" : label;
}

/** Selected disks missing from the loaded rows, labelled but counted as zero. */
export function withSelectedDisks(
  options: readonly DiskOption[],
  selected: readonly string[],
): DiskOption[] {
  const known = new Set(options.map((option) => option.key));
  const missing = selected
    .filter((key) => !known.has(key))
    .map((key) => ({ key, root: key, label: diskLabel(key), count: 0, offline: false }));
  return [...options, ...missing].map((option) => ({
    ...option,
    label: diskOptionLabel(option),
  }));
}

export type FilterChipKind = "tag" | "actor" | "disk";

/** One removable chip on the library home. */
export interface FilterChip {
  /** Category + value, stable across a re-scrape. */
  id: string;
  kind: FilterChipKind;
  value: string;
  label: string;
}

const KIND_LABELS: Record<FilterChipKind, string> = {
  tag: "标签",
  actor: "演员",
  disk: "硬盘",
};

export function chipKindLabel(kind: FilterChipKind): string {
  return KIND_LABELS[kind];
}

/**
 * The applied conditions as chips, in the same order as the filter surface's
 * sections. Chips are built from the selection, not from the current options,
 * so a selection survives a list that no longer contains it and can still be
 * removed.
 */
export function buildFilterChips(
  filters: LibraryFilters,
  disks: readonly DiskOption[],
): FilterChip[] {
  const diskLabels = new Map(disks.map((option) => [option.key, diskOptionLabel(option)]));
  return [
    ...filters.tags.map((name) => ({ id: `tag:${name}`, kind: "tag" as const, value: name, label: name })),
    ...filters.actors.map((name) => ({
      id: `actor:${name}`,
      kind: "actor" as const,
      value: name,
      label: name,
    })),
    ...filters.disks.map((key) => ({
      id: `disk:${key}`,
      kind: "disk" as const,
      value: key,
      label: diskLabels.get(key) ?? diskLabel(key),
    })),
  ];
}

/** Removes one chip's condition without disturbing the rest of the selection. */
export function removeFilterChip(filters: LibraryFilters, chip: FilterChip): LibraryFilters {
  switch (chip.kind) {
    case "tag":
      return { ...filters, tags: filters.tags.filter((name) => name !== chip.value) };
    case "actor":
      return { ...filters, actors: filters.actors.filter((name) => name !== chip.value) };
    case "disk":
      return { ...filters, disks: filters.disks.filter((key) => key !== chip.value) };
  }
}
