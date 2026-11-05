/** A centered spinner used while a view's first load is in flight. */
export function Loading({ label }: { label?: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-20 text-[var(--muted)]">
      <div className="size-6 animate-spin rounded-full border-[3px] border-[var(--border)] border-t-[var(--accent)]" />
      {label ? <span className="text-sm">{label}</span> : null}
    </div>
  );
}

/** The "nothing here" state, with an optional hint line. */
export function EmptyState({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="flex flex-col items-center gap-2 px-6 py-20 text-center">
      <div className="text-[var(--muted)]">{title}</div>
      {hint ? <div className="text-xs text-[var(--muted)] opacity-80">{hint}</div> : null}
    </div>
  );
}

/** A load failure with a retry affordance. */
export function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="flex flex-col items-center gap-3 px-6 py-20 text-center">
      <div className="text-sm text-[var(--danger)]">{message}</div>
      <button
        className="rounded-[var(--radius)] border border-[var(--border)] px-4 py-2 text-sm active:bg-[var(--panel-2)]"
        onClick={onRetry}
        type="button"
      >
        重试
      </button>
    </div>
  );
}
