import { Toast, ToastViewport } from "@astryxdesign/core/Toast";

export type ToastKind = "success" | "info" | "error";

export interface ToastItem {
  id: number;
  kind: ToastKind;
  message: string;
}

/**
 * Transient messages, entirely on Astryx's toast layer: the card, its close
 * button, stacking, live region and motion are the design system's. Longer
 * outcomes (scan summaries, error causes) belong in `ReportBanner`, not here.
 *
 * The list stays controlled (`toasts` in, `onDismiss` out) — that is what lets
 * the app keep one stack per shell and dismiss a toast by its own control.
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
    <ToastViewport position={bottom ? "bottomStart" : "topEnd"}>
      {toasts.map((toast) => (
        <Toast
          key={toast.id}
          type={toast.kind === "error" ? "error" : "info"}
          body={toast.message}
          isAutoHide={toast.kind !== "error"}
          autoHideDuration={4000}
          onDismiss={() => onDismiss?.(toast.id)}
        />
      ))}
    </ToastViewport>
  );
}
