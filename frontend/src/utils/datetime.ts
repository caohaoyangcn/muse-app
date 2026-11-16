/**
 * Timestamp formatting for the two units this codebase stores.
 *
 * `*_at` columns (`fetchedAt`, `createdAt`, `lastOpenedAt`, `nextRetryAt`, …) are
 * Unix **milliseconds**; JAV release dates (`release_unix` / `last_release_unix`)
 * are Unix **seconds**. Feeding seconds to a millisecond formatter lands in
 * January 1970 instead of failing, so each unit gets its own function.
 */

/** Formats Unix **milliseconds** as a localized date + time. */
export function formatDateTime(unixMillis?: number): string {
  if (!unixMillis) return "未提供";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(unixMillis));
}

/** Formats Unix **seconds** (JAV release dates) as a localized date. */
export function formatReleaseDate(unixSeconds?: number): string {
  if (!unixSeconds) return "未提供";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium" }).format(new Date(unixSeconds * 1000));
}
