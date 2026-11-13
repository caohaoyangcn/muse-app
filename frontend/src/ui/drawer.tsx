import type { ReactNode } from "react";

import { cn } from "../lib/utils";
import { useEscape, useFocusTrap } from "./overlay";
import "./motion.css";
import "./drawer.css";

/**
 * A panel that slides in over the page from an edge — the touch counterpart of
 * the desktop sidebar. It is the same transient surface as `BottomSheet`, just
 * opened from the side: backdrop, focus trap, Escape, and focus returned to
 * whatever opened it.
 *
 * The panel does not own its content. A shell supplies a header (brand or title
 * plus a close control), the navigation rows, and an optional footer, because a
 * drawer on a phone holds navigation while a drawer on a tablet may hold
 * filters — the surface is the shared part, the contents are the shell's.
 *
 * Safe-area padding stays with the caller: the panel is `fixed`, so it is laid
 * out against the viewport and cannot inherit the shell's own insets. Pass
 * `pt-[var(--safe-top)] pb-[var(--safe-bottom)]` where those variables exist.
 */
export function Drawer({
  open,
  title,
  onClose,
  children,
  side = "left",
  className,
}: {
  open: boolean;
  title?: string;
  onClose: () => void;
  children: ReactNode;
  side?: "left" | "right";
  className?: string;
}) {
  useEscape(open, onClose);
  const panelRef = useFocusTrap(open);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-40 flex">
      <button
        aria-label="关闭"
        className="fade-enter absolute inset-0 bg-black/55"
        onClick={onClose}
        tabIndex={-1}
        type="button"
      />
      <div
        ref={panelRef}
        aria-label={title}
        aria-modal="true"
        className={cn(
          "relative flex h-full w-[min(82vw,320px)] flex-col border-[var(--border)] bg-[var(--panel)] outline-none",
          side === "left" ? "drawer-enter-left border-r" : "drawer-enter-right ml-auto border-l",
          className,
        )}
        role="dialog"
        tabIndex={-1}
      >
        {children}
      </div>
    </div>
  );
}

/**
 * One navigation row inside a drawer. The active row carries the same 2px
 * accent marker and soft accent fill as the desktop sidebar, so the two shells
 * agree on what "you are here" looks like even though one is a column and the
 * other a panel.
 */
export function DrawerRow({
  active,
  icon,
  label,
  hint,
  onClick,
}: {
  active?: boolean;
  icon?: ReactNode;
  label: string;
  hint?: ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      aria-current={active ? "page" : undefined}
      className={cn(
        "relative flex w-full shrink-0 items-center gap-3 rounded-[var(--radius)] px-3 py-2.5 text-left text-[15px] transition-colors active:bg-[var(--panel-2)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]",
        active
          ? "bg-[rgba(124,92,255,0.12)] text-[var(--text)]"
          : "text-[var(--muted)]",
      )}
      onClick={onClick}
      type="button"
    >
      {active ? (
        <span
          aria-hidden="true"
          className="absolute left-0 top-1/2 h-4 w-0.5 -translate-y-1/2 rounded-full bg-[var(--accent)]"
        />
      ) : null}
      {icon ? (
        <span
          className={cn(
            "flex w-5 shrink-0 items-center justify-center",
            active && "text-[var(--accent)]",
          )}
        >
          {icon}
        </span>
      ) : null}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {hint ? <span className="shrink-0 text-xs text-[var(--muted)]">{hint}</span> : null}
    </button>
  );
}

/** A section caption above a group of rows. */
export function DrawerSection({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "px-3 pb-1 pt-3 text-[11px] font-medium tracking-[0.02em] text-[var(--muted)]",
        className,
      )}
    >
      {children}
    </div>
  );
}
