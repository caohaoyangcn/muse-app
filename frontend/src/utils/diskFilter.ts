import type { Video } from "../types";

/**
 * The library grid can narrow itself to a set of disks. A disk is the volume a
 * favorite lives on: the backend stamps every read with `volumeRoot` ("D:\",
 * "\\server\share\", "/"), which is also what the detached-drive warning is
 * about. The filter therefore runs over the already-loaded list and needs no
 * query of its own.
 */
export interface DiskOption {
  /** Canonical volume identity (trimmed, lower-cased). */
  key: string;
  /** Root as the read model spelled it, e.g. "D:\". */
  root: string;
  /** Short label for the chip, e.g. "D:" or "\\server\share". */
  label: string;
  /** Favorites on this volume in the list currently loaded. */
  count: number;
  /** At least one of those favorites is on a volume that is not attached. */
  offline: boolean;
}

// Drive letters and share names sort the way a file manager lists them.
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });

/** Volume names are case-insensitive on the platforms that have them. */
export function diskKey(root: string): string {
  return root.trim().toLowerCase();
}

/**
 * "D:\" -> "D:", "\\server\share\" -> "\\server\share", "/" -> "/". The chip
 * already names the disk, so the trailing separator is only noise. A drive
 * letter is upper-cased because Windows paths may spell it either way and the
 * chips should not look like two different disks.
 */
export function diskLabel(root: string): string {
  const trimmed = root.trim();
  const withoutSeparator = trimmed.replace(/[\\/]+$/, "");
  const label = withoutSeparator === "" ? trimmed : withoutSeparator;
  return /^[a-zA-Z]:$/.test(label) ? label.toUpperCase() : label;
}

/**
 * One option per volume the loaded favorites reference. Favorites with no
 * stamped root (a write response the service did not decorate, for example)
 * are left out: there is no disk to attribute them to, and inventing one would
 * put them behind a chip the user never chose.
 */
export function collectDiskOptions(videos: readonly Video[]): DiskOption[] {
  const byKey = new Map<string, DiskOption>();
  for (const video of videos) {
    const root = video.volumeRoot?.trim() ?? "";
    if (!root) continue;
    const key = diskKey(root);
    const existing = byKey.get(key);
    if (existing) {
      existing.count += 1;
      existing.offline = existing.offline || video.volumeOffline === true;
      continue;
    }
    byKey.set(key, {
      key,
      root,
      label: diskLabel(root),
      count: 1,
      offline: video.volumeOffline === true,
    });
  }
  return [...byKey.values()].sort(byLabel);
}

/**
 * Adds the disks the user selected that the current list no longer contains.
 * Switching groups or searching changes the available disks, but silently
 * dropping the selection would leave the list unfiltered without saying so.
 * Keeping the chip (with a zero count) makes the empty result explainable and
 * clearable.
 */
export function withSelectedDisks(
  options: readonly DiskOption[],
  selected: ReadonlyMap<string, string>,
): DiskOption[] {
  const known = new Set(options.map((option) => option.key));
  const out = [...options];
  for (const [key, root] of selected) {
    if (known.has(key)) continue;
    out.push({ key, root, label: diskLabel(root), count: 0, offline: false });
  }
  return out.sort(byLabel);
}

/**
 * Keeps only the favorites whose volume is in the selection. An empty
 * selection means "all disks" and returns the original list untouched.
 */
export function filterVideosByDisk(
  videos: Video[],
  selected: ReadonlySet<string>,
): Video[] {
  if (selected.size === 0) return videos;
  return videos.filter((video) => selected.has(diskKey(video.volumeRoot)));
}

function byLabel(a: DiskOption, b: DiskOption): number {
  return collator.compare(a.label, b.label);
}
