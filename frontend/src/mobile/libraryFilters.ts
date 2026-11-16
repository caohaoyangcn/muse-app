import type { Group, Video } from "../types";
import { diskKey, diskLabel, type DiskOption } from "../utils/diskFilter";

/**
 * The library's filter state. Every field is a multi-select: values inside one
 * field are a union (any of these actors), and fields combine with AND (a video
 * must match the chosen group, actor and disk sets).
 *
 * Groups are the one field the HTTP layer can answer directly — membership
 * lives in the database, not on the Video read model — so the chosen group ids
 * take part in the fetch (see LibraryView). Actors and disks are derived from
 * the loaded rows and applied locally.
 */
export interface LibraryFilters {
  groupIds: number[];
  actors: string[];
  /** Volume identity keys (`diskKey`), not display labels. */
  disks: string[];
}

export const NO_FILTERS: LibraryFilters = { groupIds: [], actors: [], disks: [] };

export function filterCount(filters: LibraryFilters): number {
  return filters.groupIds.length + filters.actors.length + filters.disks.length;
}

export function hasFilters(filters: LibraryFilters): boolean {
  return filterCount(filters) > 0;
}

/** Add or remove one value, preserving the order the user picked them in. */
export function toggleFilterValue<T>(values: readonly T[], value: T): T[] {
  return values.includes(value) ? values.filter((item) => item !== value) : [...values, value];
}

/**
 * The local half of the filter. Groups are already resolved by the fetch, so
 * only the actor and disk sets are checked here. An empty set means "all".
 */
export function filterVideos(videos: Video[], filters: LibraryFilters): Video[] {
  if (filters.actors.length === 0 && filters.disks.length === 0) return videos;
  const actors = new Set(filters.actors);
  const disks = new Set(filters.disks);
  return videos.filter((video) => {
    if (actors.size > 0 && !video.actors?.some((name) => actors.has(name))) return false;
    if (disks.size > 0 && !disks.has(diskKey(video.volumeRoot))) return false;
    return true;
  });
}

export interface ActorOption {
  name: string;
  /** Videos in the current (group/keyword narrowed) list that name this actor. */
  count: number;
}

const collator = new Intl.Collator("zh-CN", { numeric: true, sensitivity: "base" });

/**
 * Actor choices for the sheet, derived from the rows currently loaded. A video
 * names each actor once, so counting distinct videos is just a set per row.
 */
export function collectActorOptions(videos: readonly Video[]): ActorOption[] {
  const counts = new Map<string, number>();
  for (const video of videos) {
    for (const name of new Set(video.actors ?? [])) {
      if (!name) continue;
      counts.set(name, (counts.get(name) ?? 0) + 1);
    }
  }
  return [...counts.entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count || collator.compare(a.name, b.name));
}

/**
 * Keeps a selected actor visible when the rows have moved on (a group change or
 * a search may leave none of their films on screen). Dropping it silently would
 * leave an invisible condition filtering the list, so the option stays with a
 * zero count and a visible place in the sheet.
 */
export function withSelectedActors(
  options: readonly ActorOption[],
  selected: readonly string[],
): ActorOption[] {
  const known = new Set(options.map((option) => option.name));
  const missing = selected
    .filter((name) => !known.has(name))
    .map((name) => ({ name, count: 0 }));
  return missing.length === 0 ? [...options] : [...options, ...missing];
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

export type FilterChipKind = "group" | "actor" | "disk";

/** One removable chip on the library home. */
export interface FilterChip {
  /** Category + value, stable across a label edit (a renamed group). */
  id: string;
  kind: FilterChipKind;
  value: number | string;
  label: string;
}

const KIND_LABELS: Record<FilterChipKind, string> = {
  group: "分组",
  actor: "演员",
  disk: "硬盘",
};

export function chipKindLabel(kind: FilterChipKind): string {
  return KIND_LABELS[kind];
}

/**
 * The applied conditions as chips, in the same order as the sheet's sections.
 * Chips are built from the selection, not from the current options, so a
 * selection survives a list that no longer contains it (or a group deletion)
 * and can still be removed.
 */
export function buildFilterChips(
  filters: LibraryFilters,
  groups: readonly Group[],
  disks: readonly DiskOption[],
): FilterChip[] {
  const groupNames = new Map(groups.map((group) => [group.id, group.name]));
  const diskLabels = new Map(disks.map((option) => [option.key, diskOptionLabel(option)]));
  return [
    ...filters.groupIds.map((id) => ({
      id: `group:${id}`,
      kind: "group" as const,
      value: id,
      label: groupNames.get(id) ?? `分组 ${id}`,
    })),
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
    case "group":
      return { ...filters, groupIds: filters.groupIds.filter((id) => id !== chip.value) };
    case "actor":
      return { ...filters, actors: filters.actors.filter((name) => name !== chip.value) };
    case "disk":
      return { ...filters, disks: filters.disks.filter((key) => key !== chip.value) };
  }
}
