import type { ReactNode } from "react";

/**
 * A section heading inside a bottom sheet: the name on the left, an optional
 * counter or hint, and an optional compact action (the actor section's
 * 展示更多). Both the filter and the sort sheet use it so their sections read
 * as one system.
 */
export function SheetHeading({
  title,
  hint,
  action,
}: {
  title: string;
  hint?: string;
  action?: ReactNode;
}) {
  return (
    <div className="mb-2.5 flex items-baseline justify-between gap-2">
      <h3 className="text-[13px] font-semibold text-[var(--text)]">{title}</h3>
      <div className="flex min-w-0 items-center gap-2">
        {hint ? <span className="truncate text-[11px] text-[var(--muted)]">{hint}</span> : null}
        {action}
      </div>
    </div>
  );
}
