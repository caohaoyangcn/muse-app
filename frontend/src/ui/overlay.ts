import { useEffect, useRef } from "react";

/**
 * Escape closes a transient surface — on touch the back gesture is the usual
 * exit, and the shell maps it to the same close action.
 */
export function useEscape(open: boolean, onClose: () => void) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);
}

const FOCUSABLE = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  '[tabindex]:not([tabindex="-1"])',
].join(",");

/**
 * Keeps focus inside an open surface.
 *
 * The panel itself takes the initial focus, not its first field: most sheets
 * hold a text input, and focusing that on open would pop the soft keyboard over
 * the surface the user just opened to read. Tab still walks into the contents,
 * and focus that escapes (a browser-driven focus move, not a Tab) is pulled
 * back.
 *
 * Focus returns to whatever opened the surface when it closes, so a keyboard or
 * screen-reader user is not dropped at the top of the page.
 */
export function useFocusTrap(open: boolean) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const panel = ref.current;
    if (!panel) return;
    const previous = document.activeElement as HTMLElement | null;
    const focusables = () =>
      Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
        (el) => el.getClientRects().length > 0,
      );

    panel.focus({ preventScroll: true });

    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Tab") return;
      const list = focusables();
      if (list.length === 0) {
        e.preventDefault();
        return;
      }
      const index = list.indexOf(document.activeElement as HTMLElement);
      const next = e.shiftKey
        ? index <= 0
          ? list.length - 1
          : index - 1
        : index === -1 || index === list.length - 1
          ? 0
          : index + 1;
      e.preventDefault();
      list[next]?.focus({ preventScroll: true });
    };

    const onFocusIn = (e: FocusEvent) => {
      if (panel.contains(e.target as Node)) return;
      const list = focusables();
      (list[0] ?? panel).focus({ preventScroll: true });
    };

    document.addEventListener("keydown", onKey, true);
    document.addEventListener("focusin", onFocusIn);
    return () => {
      document.removeEventListener("keydown", onKey, true);
      document.removeEventListener("focusin", onFocusIn);
      if (previous?.isConnected) previous.focus({ preventScroll: true });
    };
  }, [open]);

  return ref;
}
