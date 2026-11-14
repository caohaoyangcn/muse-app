import { Toast, ToastViewport } from "@astryxdesign/core/Toast";

import { cn } from "../lib/utils";

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
 * Transient messages, hosted by Astryx's toast layer.
 *
 * Astryx owns the stacking, the live region and the enter/exit motion; the card
 * itself is ours (`renderContent`), because the two clients want different
 * cards: the desktop one carries a details list and a colour-coded left edge,
 * the phone one is a single line above the home indicator.
 *
 * The list stays controlled (`toasts` in, `onDismiss` out) — that is what lets
 * the app keep one stack per shell and dismiss a toast by tapping it.
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
    // The stack is anchored here, not by Astryx: `ToastViewport` lays the cards
    // out but does not take them out of flow, and the two shells dock it in
    // different corners (desktop top-right, phone above the home indicator).
    <div
      className={cn(
        "pointer-events-none z-[400]",
        bottom
          ? "fixed inset-x-3 bottom-[calc(var(--safe-bottom)+16px)]"
          : "fixed right-4 top-4 w-[340px]",
      )}
    >
      <ToastViewport position={bottom ? "bottomStart" : "topEnd"}>
      {toasts.map((toast) => (
        <Toast
          key={toast.id}
          type={toast.kind === "error" ? "error" : "info"}
          body={toast.message}
          // The app decides when a toast goes away; a failure list should stay
          // until it has been read.
          isAutoHide={false}
          autoHideDuration={0}
          onDismiss={() => onDismiss?.(toast.id)}
          renderContent={({ body }) => (
            <div
              onClick={onDismiss ? () => onDismiss(toast.id) : undefined}
              className={cn(
                "pointer-events-auto",
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
              <div className={bottom ? undefined : "font-medium"}>{body}</div>
              {!bottom && toast.details && toast.details.length > 0 && (
                <ul className="mt-2 list-disc pl-[18px] text-xs leading-relaxed break-words text-[var(--muted)]">
                  {toast.details.map((detail, i) => (
                    <li key={i}>{detail}</li>
                  ))}
                </ul>
              )}
            </div>
          )}
        />
      ))}
      </ToastViewport>
    </div>
  );
}
