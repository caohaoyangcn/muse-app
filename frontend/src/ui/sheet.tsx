import type { ReactNode } from "react";
import { BottomSheet as AstryxBottomSheet } from "@astryxdesign/core/BottomSheet";

import { cn } from "../lib/utils";
import { Button } from "./button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "./dialog";

/**
 * A bottom sheet — the touch stand-in for the desktop app's context menus and
 * dialogs. Astryx supplies the surface (scrim, focus trap, Escape, the
 * slide-up motion and the drag handle), so this module only lays out what the
 * dashboards put in it.
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
  return (
    <AstryxBottomSheet
      isOpen={open}
      onOpenChange={(next) => !next && onClose()}
      label={title ?? "操作"}
      // Sheets are edge-to-edge on purpose; each row brings its own padding.
      padding={0}
      height="85dvh"
    >
      <div className="flex items-center justify-between px-4 pb-2 pt-3">
        <div className="text-base font-semibold">{title}</div>
        <Button variant="ghost" size="sm" onClick={onClose}>
          完成
        </Button>
      </div>
      <div className="scroll-area max-h-[70dvh] px-2 pb-[var(--safe-bottom)]">{children}</div>
    </AstryxBottomSheet>
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
    <Button
      variant="ghost"
      disabled={disabled}
      className={cn(
        "flex h-auto w-full items-center justify-start gap-3 rounded-[var(--radius)] px-3 py-3 text-left text-[15px]",
        danger ? "text-[var(--danger)]" : "text-[var(--text)]",
      )}
      onClick={onClick}
    >
      {icon ? <span className="w-5 text-center text-[var(--muted)]">{icon}</span> : null}
      <span className="flex-1">{label}</span>
      {hint ? <span className="text-xs text-[var(--muted)]">{hint}</span> : null}
    </Button>
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
  return (
    <Dialog open={open} width={380} onOpenChange={(next) => !next && onClose()}>
      <DialogContent label={title}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        {message ? (
          <p className="mt-2 text-sm leading-relaxed text-[var(--muted)]">{message}</p>
        ) : null}
        <DialogFooter className="mt-5">
          <Button onClick={onClose}>取消</Button>
          <Button variant={danger ? "danger" : "primary"} disabled={busy} onClick={onConfirm}>
            {busy ? "处理中…" : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
