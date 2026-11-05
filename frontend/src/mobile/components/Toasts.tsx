import { cn } from "../../lib/utils";

export type Toast = {
  id: number;
  message: string;
  tone: "info" | "error";
};

/** Transient messages pinned above the tab bar. */
export function Toasts({ toasts }: { toasts: Toast[] }) {
  if (toasts.length === 0) return null;
  return (
    <div className="pointer-events-none fixed inset-x-3 bottom-[calc(var(--safe-bottom)+72px)] z-30 flex flex-col gap-2">
      {toasts.map((t) => (
        <div
          className={cn(
            "fade-enter rounded-[var(--radius)] border px-3 py-2 text-sm shadow-lg",
            t.tone === "error"
              ? "border-[var(--danger)] bg-[var(--danger)] text-white"
              : "border-[var(--border)] bg-[var(--panel-2)] text-[var(--text)]",
          )}
          key={t.id}
        >
          {t.message}
        </div>
      ))}
    </div>
  );
}
