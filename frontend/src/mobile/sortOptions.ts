import type { SortKey } from "../types";

/**
 * The library's sort fields, in the order the sheet lists them. Kept beside the
 * sort sheet rather than inside it so the toolbar button can name the current
 * field without importing a component.
 */
export const SORT_LABELS: Record<SortKey, string> = {
  createdAt: "添加时间",
  lastOpenedAt: "最近播放",
  fileSize: "文件大小",
  fileName: "文件名",
  likeCount: "点赞数",
};

export const SORT_KEYS = Object.keys(SORT_LABELS) as SortKey[];
