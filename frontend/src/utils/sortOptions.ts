import type { SortDir, SortKey } from "../types";

/**
 * The library's sort fields, shared by the desktop SortBar and the phone's
 * SortSheet. `defaultDir` is the field's natural first direction when the user
 * picks it (newest/largest first, A→Z for names). Keeping this list in one
 * place is what stopped the two clients from disagreeing about 文件名.
 */
export interface SortField {
  key: SortKey;
  label: string;
  defaultDir: SortDir;
}

export const SORT_FIELDS: readonly SortField[] = [
  { key: "createdAt", label: "添加时间", defaultDir: "desc" },
  { key: "lastOpenedAt", label: "最近播放", defaultDir: "desc" },
  { key: "fileSize", label: "文件大小", defaultDir: "desc" },
  { key: "fileName", label: "文件名", defaultDir: "asc" },
  { key: "likeCount", label: "点赞数", defaultDir: "desc" },
];

export const SORT_LABELS = Object.fromEntries(
  SORT_FIELDS.map((field) => [field.key, field.label]),
) as Record<SortKey, string>;

export const SORT_KEYS = SORT_FIELDS.map((field) => field.key);

export function defaultSortDir(key: SortKey): SortDir {
  return SORT_FIELDS.find((field) => field.key === key)?.defaultDir ?? "desc";
}
