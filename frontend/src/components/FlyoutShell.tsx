import { useLayoutEffect, useRef, useState } from "react";
import type { CSSProperties, ReactNode } from "react";

interface FlyoutShellProps {
  /** Bounding rect (viewport coords) of the sub-menu button that opened the flyout. */
  anchor: DOMRect;
  /** Preferred side; flips automatically when the flyout would overflow. */
  side: "right" | "left";
  children: ReactNode;
}

const GAP = 6;
const EDGE = 4;

/**
 * Positions a flyout against its anchor (the sub-menu button):
 *  - vertical: top edge aligns with the anchor's top edge; if the flyout
 *    would overflow the bottom, it flips to grow upward instead.
 *  - horizontal: sits at the requested side with a small gap; if it would
 *    overflow the viewport, it flips to the other side.
 *
 * The flyout is rendered *inside* the menu surface, so hovering it never trips
 * the menu's hover-away close. Because a flip can leave a gap between the
 * opener and the flyout (vertical up-flip, horizontal left-flip, or both), a
 * transparent bridge element covers the vertical strip between the opener's
 * inner edge and the flyout's inner edge (spanning the full vertical union of
 * both), so the mouse path from the opener into the flyout stays on an
 * interactive surface and the hover-away close never fires mid-travel.
 *
 * The flyout never exceeds the app window: it is capped to the viewport height
 * so a very tall list scrolls internally instead of being clipped (the shell is
 * a flex container, .group-flyout is a column flex, and the list is the
 * shrink/scroll region).
 *
 * The bridge is deliberately *only* that strip, not the whole bounding
 * rectangle: a full-bounding-rect bridge would sit (zIndex 214) on top of the
 * menu surface (zIndex 200) and swallow lower menu items — e.g. at the bottom
 * of the viewport the groups flyout's bridge covered the last menu item, so
 * hovering it never switched submenus.
 */
export default function FlyoutShell({ anchor, side, children }: FlyoutShellProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState(() => ({
    top: anchor.top,
    left: side === "right" ? anchor.left + anchor.width + GAP : anchor.left - GAP,
    // Start unconstrained so the first layout pass measures the flyout's real
    // natural height; the effect then caps it to the viewport.
    maxHeight: 1e9,
  }));
  const [bridge, setBridge] = useState<CSSProperties | null>(null);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const maxLeft = vw - rect.width - EDGE;

    // Vertical: align top with the anchor; if the flyout would overflow the
    // bottom, flip it up. The flyout is additionally capped to the viewport
    // height so a very tall list scrolls internally (via the shell's max-height
    // and the panel's flex layout) instead of being clipped by overflow:hidden.
    const natural = rect.height;
    let top = anchor.top;
    if (top + natural > vh - EDGE) top = Math.max(EDGE, vh - EDGE - natural);
    top = Math.max(EDGE, top);
    const maxHeight = Math.min(natural, Math.max(0, vh - EDGE - top));

    // Horizontal: prefer the requested side, flip when it overflows.
    let left: number;
    const rightPos = anchor.left + anchor.width + GAP;
    const leftPos = anchor.left - GAP - rect.width;
    if (side === "right") {
      left = rightPos <= maxLeft ? rightPos : leftPos;
    } else {
      left = leftPos >= EDGE ? leftPos : rightPos;
    }
    left = Math.min(maxLeft, Math.max(EDGE, left));

    setPos((prev) =>
      prev.top === top && prev.left === left && prev.maxHeight === maxHeight
        ? prev
        : { top, left, maxHeight },
    );

    // Bridge: one vertical strip between the anchor's inner edge and the
    // flyout's inner edge, covering the whole vertical union of the two. This
    // covers every cursor path between the opener and the flyout (whether or
    // not the flyout flipped) while never extending over the menu surface.
    // The flyout's rendered height may be capped (maxHeight), so the bridge
    // follows the actual height, never the natural one — otherwise it would
    // extend past the visible flyout and swallow menu items below it.
    const actualHeight = Math.min(natural, maxHeight);
    const anchorRight = anchor.left + anchor.width;
    const anchorBottom = anchor.top + anchor.height;
    const flyoutRight = left + rect.width;
    const flyoutBottom = top + actualHeight;
    const flyoutIsRight = left >= anchorRight;
    const gapLeft = flyoutIsRight ? anchorRight : flyoutRight;
    const gapRight = flyoutIsRight ? left : anchor.left;
    const bridgeStyle: CSSProperties = {
      position: "fixed",
      left: gapLeft,
      top: Math.min(anchor.top, top),
      width: Math.max(0, gapRight - gapLeft),
      height: Math.max(anchorBottom, flyoutBottom) - Math.min(anchor.top, top),
      zIndex: 214,
    };
    setBridge((prev) =>
      prev &&
      prev.left === bridgeStyle.left &&
      prev.top === bridgeStyle.top &&
      prev.width === bridgeStyle.width &&
      prev.height === bridgeStyle.height
        ? prev
        : bridgeStyle,
    );
  }, [anchor.height, anchor.left, anchor.top, anchor.width, side]);

  return (
    <>
      {bridge && <div className="flyout-bridge" style={bridge} />}
      <div
        ref={ref}
        className="flex"
        style={{
          position: "fixed",
          top: pos.top,
          left: pos.left,
          zIndex: 215,
          maxHeight: pos.maxHeight,
        }}
      >
        {children}
      </div>
    </>
  );
}
