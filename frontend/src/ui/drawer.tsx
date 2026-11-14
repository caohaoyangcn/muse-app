import type { ReactNode } from "react";
import { Drawer as AstryxDrawer } from "@astryxdesign/core/Drawer";

import { cn } from "../lib/utils";
import { Button } from "./button";

/**
 * A panel that slides in from an edge — the touch counterpart of the desktop
 * sidebar. Astryx's Drawer supplies the surface: the native dialog, the scrim,
 * the focus trap, Escape and the slide motion, all of which used to be hand
 * written in `overlay.ts`.
 *
 * The panel still does not own its content. A shell supplies a header (brand or
 * title plus a close control), the navigation rows and an optional footer,
 * because a drawer on a phone holds navigation while a drawer on a tablet may
 * hold filters — the surface is the shared part.
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
  return (
    <AstryxDrawer
      isOpen={open}
      onOpenChange={(next) => !next && onClose()}
      side={side === "left" ? "start" : "end"}
      label={title ?? "导航"}
      // The shells bring their own safe-area padding and edge-to-edge rows.
      padding={0}
      width="min(82vw, 320px)"
      className={className}
    >
      {children}
    </AstryxDrawer>
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
    <Button
      aria-current={active ? "page" : undefined}
      variant="ghost"
      className={cn(
        "relative flex h-auto w-full shrink-0 items-center justify-start gap-3 rounded-[var(--radius)] px-3 py-2.5 text-left text-[15px]",
        active ? "bg-[rgba(124,92,255,0.12)] text-[var(--text)]" : "text-[var(--muted)]",
      )}
      onClick={onClick}
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
    </Button>
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
