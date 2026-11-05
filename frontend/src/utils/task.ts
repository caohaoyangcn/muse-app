export function formatDateTime(unixMillis?: number): string {
  if (!unixMillis) return "未提供";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(unixMillis));
}
