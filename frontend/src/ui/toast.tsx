import { cn } from "../lib/utils";
import "./motion.css";

export type ToastKind = "success" | "info" | "error";

export interface ToastItem {
  id: number;
  kind: ToastKind;
  message: string;
  /** Extra lines shown under the message. The phone has no room for them. */
  details?: string[];
}

const ACCENT: Record<ToastKind, string> = {
  success: "border-l-[#3fb26f]",
  error: "border-l-[var(--danger)]",
  info: "border-l-[var(--accent)]",
};

/**
 * Transient messages. One stack for both clients, two placements:
 *
 * - `top-right` — the desktop card, with the details list (a toast there reports
 *   what failed in a list form, e.g. which files were skipped).
 * - `bottom` — the phone banner above the tab bar. No details: it disappears in
 *   under three seconds and the first line is all anyone reads.
 *
 * `aria-live="polite"` is on the container, so a screen reader announces each
 * arrival without stealing focus from whatever the user is doing.
 */
export function ToastStack({
  toasts,
  onDismiss,
  placement = "top-right",
}: {
  toasts: ToastItem[];
  onDismiss?: (id: number) => void;
  placement?: "top-right" | "bottom";
}) {
  if (toasts.length === 0) return null;
  const bottom = placement === "bottom";
  return (
    <div
      aria-live="polite"
      className={cn(
        "flex flex-col gap-2",
        bottom
          ? "pointer-events-none fixed inset-x-3 bottom-[calc(var(--safe-bottom)+72px)] z-30"
          : "fixed right-4 top-4 z-[400] w-[340px] gap-2.5",
      )}
    >
      {toasts.map((toast) => (
        <div
          key={toast.id}
          onClick={onDismiss ? () => onDismiss(toast.id) : undefined}
          className={cn(
            bottom
              ? "fade-enter rounded-[var(--radius)] border px-3 py-2 text-sm shadow-lg"
              : "cursor-pointer rounded-lg border border-[var(--border)] border-l-[3px] bg-[var(--panel-2)] px-3.5 py-2.5 shadow-[0_8px_24px_rgba(0,0,0,0.45)]",
            bottom && toast.kind === "error"
              ? "border-[var(--danger)] bg-[var(--danger)] text-white"
              : bottom
                ? "border-[var(--border)] bg-[var(--panel-2)] text-[var(--text)]"
                : ACCENT[toast.kind],
          )}
        >
          <div className={bottom ? undefined : "font-medium"}>{toast.message}</div>
          {!bottom && toast.details && toast.details.length > 0 && (
            <ul className="mt-2 list-disc pl-[18px] text-xs leading-relaxed text-[var(--muted)] break-words">
              {toast.details.map((detail, i) => (
                <li key={i}>{detail}</li>
              ))}
            </ul>
          )}
        </div>
      ))}
    </div>
  );
}
