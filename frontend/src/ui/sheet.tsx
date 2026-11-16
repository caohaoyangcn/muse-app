import type { ReactNode } from "react";
import { BottomSheet as AstryxBottomSheet } from "@astryxdesign/core/BottomSheet";

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
  height = "85dvh",
  headerEnd,
}: {
  open: boolean;
  title?: string;
  onClose: () => void;
  children: ReactNode;
  /** A sheet that hugs its content passes "auto"; task sheets keep the default
   *  tall surface so their rows stay in one reachable scroll area. */
  height?: string;
  /** Replaces the default 完成 control. A sheet with its own footer action can
   *  use a compact close icon instead of saying "done" twice. */
  headerEnd?: ReactNode;
}) {
  return (
    <AstryxBottomSheet
      isOpen={open}
      onOpenChange={(next) => !next && onClose()}
      label={title ?? "操作"}
      // Sheets are edge-to-edge on purpose; each row brings its own padding.
      padding={0}
      height={height}
    >
      <div className="flex items-center justify-between px-4 pb-2 pt-3">
        <div className="text-base font-semibold">{title}</div>
        {headerEnd ?? (
          <Button
            className="text-[13px] text-[var(--muted)]"
            variant="ghost"
            size="sm"
            onClick={onClose}
          >
            完成
          </Button>
        )}
      </div>
      <div className="scroll-area max-h-[70dvh] px-2 pb-[var(--safe-bottom)]">{children}</div>
    </AstryxBottomSheet>
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
