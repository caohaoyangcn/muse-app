import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { ReactNode } from "react";

import { cn } from "../lib/utils";
import { Button } from "./button";

/** One row of a menu. Rows that carry a flyout open it on hover. */
export type MenuItemSpec = {
  key: string;
  label: ReactNode;
  /** Secondary text pinned to the right edge. */
  hint?: ReactNode;
  danger?: boolean;
  disabled?: boolean;
  title?: string;
  /** Plain rows: run this, then close the menu. */
  onSelect?: () => void;
  /** Rows with a flyout: the row's rect is the anchor, hovering opens it. */
  renderFlyout?: (anchor: DOMRect) => ReactNode;
};

const MARGIN = 8;
/** Grace period before a hover-away closes the flyout: crossing a corner on the
 *  way to it must not flicker the submenu shut. */
const LEAVE_GRACE_MS = 100;

/**
 * The one right-click / command menu surface.
 *
 * Three menus in this app used to carry their own copy of the same plumbing:
 * measure-then-clamp against the viewport, close on outside press / Escape /
 * window blur, and hover-intent for a flyout anchored to its opener. That is
 * all here; a menu now only describes its rows.
 *
 * Positioning: the surface renders once at the click point but invisible, so it
 * can be measured. Clamping against a hardcoded height used to clip the last
 * rows whenever the click landed low in the window — routine for cards near the
 * bottom of the grid.
 *
 * The flyout is rendered *inside* the surface (see FlyoutShell), so hovering it
 * never trips the hover-away close.
 */
export function MenuSurface({
  x,
  y,
  items,
  label,
  onClose,
}: {
  x: number;
  y: number;
  items: MenuItemSpec[];
  /** Accessible name, since a context menu has no visible title. */
  label?: string;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const rows = useRef(new Map<string, HTMLButtonElement | null>());
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);
  const [openKey, setOpenKey] = useState<string | null>(null);
  const leaveTimer = useRef<number | null>(null);
  // Where focus was before the menu opened; returned there on close so a
  // keyboard user is not dropped at the top of the document.
  const returnFocus = useRef<HTMLElement | null>(null);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const { width, height } = el.getBoundingClientRect();
    setPos({
      left: Math.max(MARGIN, Math.min(x, window.innerWidth - width - MARGIN)),
      top: Math.max(MARGIN, Math.min(y, window.innerHeight - height - MARGIN)),
    });
  }, [x, y]);

  // A context menu takes focus (rows are reachable with the arrow keys) and
  // gives it back on close. preventScroll: opening a menu must not scroll the
  // grid behind it.
  useLayoutEffect(() => {
    returnFocus.current = document.activeElement as HTMLElement | null;
    const first = ref.current?.querySelector<HTMLElement>('[role="menuitem"]:not([disabled])');
    first?.focus({ preventScroll: true });
    return () => {
      const target = returnFocus.current;
      if (target && target.isConnected) target.focus({ preventScroll: true });
    };
  }, []);

  const enabledRows = (): HTMLElement[] =>
    Array.from(ref.current?.querySelectorAll<HTMLElement>('[role="menuitem"]:not([disabled])') ?? []);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    // Handled keys stop here: the library grid listens for arrows on window, and
    // a menu on top of it must own the keyboard while it is open.
    if (e.key === "Tab") {
      e.preventDefault();
      e.stopPropagation();
      onClose();
      return;
    }
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) return;
    e.preventDefault();
    e.stopPropagation();
    const rows = enabledRows();
    if (rows.length === 0) return;
    const current = rows.indexOf(document.activeElement as HTMLElement);
    const next =
      e.key === "Home"
        ? 0
        : e.key === "End"
          ? rows.length - 1
          : e.key === "ArrowDown"
            ? (current + 1) % rows.length
            : (current <= 0 ? rows.length : current) - 1;
    rows[next]?.focus({ preventScroll: true });
  };

  useEffect(() => {
    const close = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", onKey);
    window.addEventListener("blur", onClose);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("blur", onClose);
      if (leaveTimer.current != null) window.clearTimeout(leaveTimer.current);
    };
  }, [onClose]);

  const cancelLeave = () => {
    if (leaveTimer.current != null) {
      window.clearTimeout(leaveTimer.current);
      leaveTimer.current = null;
    }
  };

  // Delegated hover: only rows decide. A flyout's own rows are not marked with
  // data-menu-key, so hovering the flyout never closes it.
  const handleMouseOver = (e: React.MouseEvent<HTMLDivElement>) => {
    const row = (e.target as HTMLElement).closest<HTMLElement>("[data-menu-key]");
    if (!row) return;
    const key = row.dataset.menuKey ?? null;
    if (key === openKey) return;
    const spec = items.find((item) => item.key === key);
    setOpenKey(spec?.renderFlyout && !spec.disabled ? key : null);
  };

  const handleMouseLeave = () => {
    cancelLeave();
    leaveTimer.current = window.setTimeout(() => setOpenKey(null), LEAVE_GRACE_MS);
  };

  const openItem = openKey ? items.find((item) => item.key === openKey) : undefined;
  const openAnchor = openKey ? rows.current.get(openKey)?.getBoundingClientRect() : undefined;

  return (
    <div
      ref={ref}
      role="menu"
      aria-label={label}
      aria-orientation="vertical"
      className="fixed z-[200] min-w-[210px] rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel-2)] p-1.5 shadow-[0_8px_24px_rgba(0,0,0,0.45)]"
      style={pos ? { left: pos.left, top: pos.top } : { left: x, top: y, visibility: "hidden" }}
      onKeyDown={handleKeyDown}
      onMouseEnter={cancelLeave}
      onMouseLeave={handleMouseLeave}
      onMouseOver={handleMouseOver}
    >
      {items.map((item) => (
        <Button
          key={item.key}
          ref={(el) => {
            rows.current.set(item.key, el);
          }}
          role="menuitem"
          variant="ghost"
          data-menu-key={item.key}
          aria-haspopup={item.renderFlyout ? "menu" : undefined}
          aria-expanded={item.renderFlyout ? openKey === item.key : undefined}
          title={item.title}
          disabled={item.disabled}
          className={cn(
            "h-auto w-full justify-start rounded-md px-3 py-2 text-left text-[var(--text)]",
            item.danger && "text-[var(--danger)]",
            openKey === item.key && "bg-[var(--border)]",
          )}
          onClick={() => {
            if (item.renderFlyout) {
              setOpenKey(item.key);
              return;
            }
            item.onSelect?.();
            onClose();
          }}
        >
          <span className="min-w-0 flex-1">{item.label}</span>
          {item.hint ? <span className="shrink-0 text-xs text-[var(--muted)]">{item.hint}</span> : null}
        </Button>
      ))}
      {openItem?.renderFlyout && openAnchor ? openItem.renderFlyout(openAnchor) : null}
    </div>
  );
}
