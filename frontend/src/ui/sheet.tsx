import { useEffect, type ReactNode } from "react";

import { cn } from "../lib/utils";
import "./sheet.css";

/** Escape closes the surface — on touch the back gesture is the usual exit. */
function useEscape(open: boolean, onClose: () => void) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);
}

/**
 * A bottom sheet — the touch stand-in for the desktop app's context menus and
 * dialogs. Everything the desktop reaches through right-click or a modal lives
 * here on a phone, and it is the mobile form of `ui/ActionMenu`.
 */
export function BottomSheet({
  open,
  title,
  onClose,
  children,
}: {
  open: boolean;
  title?: string;
  onClose: () => void;
  children: ReactNode;
}) {
  useEscape(open, onClose);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-40 flex flex-col justify-end">
      <button
        aria-label="关闭"
        className="fade-enter absolute inset-0 bg-black/55"
        onClick={onClose}
        type="button"
      />
      <div
        className="sheet-enter relative max-h-[85dvh] rounded-t-2xl border-t border-[var(--border)] bg-[var(--panel)] pb-[var(--safe-bottom)]"
        aria-modal="true"
        role="dialog"
      >
        <div className="flex items-center justify-between px-4 pb-2 pt-4">
          <div className="text-base font-semibold">{title}</div>
          <button
            className="-mr-2 rounded-full px-3 py-1 text-sm text-[var(--muted)] active:bg-[var(--panel-2)]"
            onClick={onClose}
            type="button"
          >
            完成
          </button>
        </div>
        <div className="scroll-area max-h-[70dvh] px-2 pb-3">{children}</div>
      </div>
    </div>
  );
}

/** A tappable row inside a sheet. */
export function SheetAction({
  label,
  hint,
  icon,
  danger,
  disabled,
  onClick,
}: {
  label: string;
  hint?: string;
  icon?: ReactNode;
  danger?: boolean;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      className={cn(
        "flex w-full items-center gap-3 rounded-[var(--radius)] px-3 py-3 text-left text-[15px] active:bg-[var(--panel-2)] disabled:opacity-40",
        danger ? "text-[var(--danger)]" : "text-[var(--text)]",
      )}
      disabled={disabled}
      onClick={onClick}
      type="button"
    >
      {icon ? <span className="w-5 text-center text-[var(--muted)]">{icon}</span> : null}
      <span className="flex-1">{label}</span>
      {hint ? <span className="text-xs text-[var(--muted)]">{hint}</span> : null}
    </button>
  );
}

/** A small confirm sheet for destructive or irreversible actions. */
export function ConfirmSheet({
  open,
  title,
  message,
  confirmLabel = "确定",
  danger,
  busy,
  onConfirm,
  onClose,
}: {
  open: boolean;
  title: string;
  message?: string;
  confirmLabel?: string;
  danger?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  useEscape(open, onClose);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-6">
      <button
        aria-label="关闭"
        className="fade-enter absolute inset-0 bg-black/60"
        onClick={onClose}
        type="button"
      />
      <div
        className="fade-enter relative w-full max-w-sm rounded-2xl border border-[var(--border)] bg-[var(--panel)] p-5"
        aria-modal="true"
        role="alertdialog"
      >
        <div className="text-base font-semibold">{title}</div>
        {message ? (
          <p className="mt-2 text-sm leading-relaxed text-[var(--muted)]">{message}</p>
        ) : null}
        <div className="mt-5 flex gap-3">
          <button
            className="flex-1 rounded-[var(--radius)] border border-[var(--border)] py-2.5 text-sm active:bg-[var(--panel-2)]"
            onClick={onClose}
            type="button"
          >
            取消
          </button>
          <button
            className={cn(
              "flex-1 rounded-[var(--radius)] py-2.5 text-sm font-medium text-white disabled:opacity-50",
              danger ? "bg-[var(--danger)]" : "bg-[var(--accent)]",
            )}
            disabled={busy}
            onClick={onConfirm}
            type="button"
          >
            {busy ? "处理中…" : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
