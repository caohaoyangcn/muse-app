import { act, type ReactElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";

import { useVirtualRows } from "./useVirtualRows";

/**
 * Regression tests for the row windowing that keeps a 1000+ favorite library
 * cheap.
 *
 * The failure they pin down is not exotic: any future change to how a card is
 * wrapped can break the measurement, and a broken measurement does not throw —
 * the list simply stops windowing and renders its starter rows, which on the
 * desktop meant "全部收藏 shows the first 24 videos and does not scroll". Astryx's
 * Tooltip wrapping each card in a zero-height (display:contents) span caused
 * exactly that, so the wrapped-card case is asserted first.
 *
 * jsdom has no layout engine, so the components below declare their own boxes
 * through data attributes and the three accessors the hook uses are patched for
 * the duration of the file.
 */

/** The card width the wall asks for, and the box every card reports. */
const CARD_PX = 200;
const CARD_H = 300;
/** Viewport height, and the offset of the wall below the toolbar. */
const VIEWPORT_H = 600;
const GRID_TOP = 120;
const GRID_W = 1000;
/** A library big enough that windowing must engage (the threshold is 100). */
const LONG_LIST = 1217;

interface Reading {
  columns: number;
  endIndex: number;
  startIndex: number;
  totalHeight: number;
  windowed: boolean;
}

let probe: Reading | null = null;
let root: Root | null = null;
let host: HTMLElement | null = null;
/** How far the scroller has been scrolled; boxes inside it move up by this. */
let scrollOffset = 0;

const originalRect = HTMLElement.prototype.getBoundingClientRect;

function num(element: Element, attribute: string): number {
  return Number(element.getAttribute(attribute) ?? 0);
}

function reading(): Reading {
  if (!probe) throw new Error("the wall has not rendered yet");
  return probe;
}

beforeAll(() => {
  // React's act() needs to be told this is a test environment.
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

  // Boxes come from data attributes; anything without them is a zero box, the
  // same answer jsdom gives for everything.
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect(this: HTMLElement) {
    const height = num(this, "data-rect-h");
    // Content inside the scroller moves up with it; the scroller itself stays.
    const scrolled = this.hasAttribute("data-scrolled") ? scrollOffset : 0;
    const top = num(this, "data-rect-top") - scrolled;
    const width = num(this, "data-rect-w");
    return {
      bottom: top + height,
      height,
      left: 0,
      right: width,
      top,
      width,
      x: 0,
      y: top,
      toJSON: () => ({}),
    } as DOMRect;
  };
  Object.defineProperty(HTMLElement.prototype, "clientWidth", {
    configurable: true,
    get(this: HTMLElement) {
      return num(this, "data-client-w");
    },
  });
  Object.defineProperty(HTMLElement.prototype, "clientHeight", {
    configurable: true,
    get(this: HTMLElement) {
      return num(this, "data-client-h");
    },
  });

  // The hook observes its scroller; jsdom has no ResizeObserver. The delivery
  // that follows observe() is what produces the window: the first measurement
  // runs before the scroller is known.
  class ResizeObserverShim {
    constructor(private readonly callback: () => void) {}
    observe() {
      queueMicrotask(() => this.callback());
    }
    unobserve() {}
    disconnect() {}
  }
  globalThis.ResizeObserver = ResizeObserverShim as unknown as typeof ResizeObserver;

  // rAF drives the scroll handler; run it on a timer so the test can wait.
  globalThis.requestAnimationFrame = ((callback: FrameRequestCallback) =>
    globalThis.setTimeout(() => callback(performance.now()), 0)) as unknown as typeof requestAnimationFrame;
  globalThis.cancelAnimationFrame = ((handle: number) =>
    globalThis.clearTimeout(handle)) as unknown as typeof cancelAnimationFrame;
});

afterAll(() => {
  HTMLElement.prototype.getBoundingClientRect = originalRect;
  delete (HTMLElement.prototype as unknown as Record<string, unknown>).clientWidth;
  delete (HTMLElement.prototype as unknown as Record<string, unknown>).clientHeight;
  delete (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT;
});

afterEach(async () => {
  probe = null;
  await act(async () => {
    root?.unmount();
  });
  root = null;
  host?.remove();
  host = null;
});

/**
 * The wall under test. `wrapped` puts each card behind a zero-height span, which
 * is the shape Astryx's Tooltip produces.
 */
function Wall({ count, wrapped }: { count: number; wrapped: boolean }) {
  const virtual = useVirtualRows({ count, minColumnWidth: CARD_PX, maxColumns: 4 });
  probe = {
    columns: virtual.columns,
    endIndex: virtual.endIndex,
    startIndex: virtual.startIndex,
    totalHeight: virtual.totalHeight,
    windowed: virtual.windowed,
  };

  const items: ReactElement[] = [];
  for (let index = virtual.startIndex; index < virtual.endIndex; index += 1) {
    const card = <div data-card="" data-rect-h={String(CARD_H)} key={index} />;
    items.push(
      wrapped ? (
        <span data-rect-h="0" data-wrapper="" key={index}>
          {card}
        </span>
      ) : (
        card
      ),
    );
  }

  return (
    <div
      data-client-w={String(GRID_W)}
      data-rect-h={String(virtual.totalHeight)}
      data-rect-top={String(GRID_TOP)}
      data-scrolled=""
      ref={virtual.containerRef}
    >
      <div
        data-client-w={String(GRID_W)}
        data-grid=""
        data-rect-h="0"
        data-rect-top={String(GRID_TOP)}
        data-scrolled=""
        ref={(element) => {
          virtual.windowRef.current = element;
        }}
      >
        {items}
      </div>
    </div>
  );
}

/** Renders the wall inside the scroll container the hook expects to find. */
async function mount(count: number, wrapped: boolean): Promise<HTMLElement> {
  scrollOffset = 0;
  host = document.createElement("div");
  document.body.appendChild(host);
  // The wall must scroll inside an ancestor whose overflow is a scroll, exactly
  // like the desktop's <main>.
  const scroller = document.createElement("div");
  scroller.style.overflowY = "auto";
  scroller.setAttribute("data-client-h", String(VIEWPORT_H));
  scroller.setAttribute("data-client-w", String(GRID_W));
  scroller.setAttribute("data-rect-h", String(VIEWPORT_H));
  scroller.setAttribute("data-rect-top", "0");
  // jsdom clamps a real scrollTop to the (unlaid-out) scrollable area, so the
  // scroll offset is held by the test instead: the hook only reads it, and the
  // content inside the scroller is reported as moved up by the same amount.
  Object.defineProperty(scroller, "scrollTop", {
    configurable: true,
    get: () => scrollOffset,
    set: (value: number) => {
      scrollOffset = value;
    },
  });
  host.appendChild(scroller);
  const mountPoint = document.createElement("div");
  scroller.appendChild(mountPoint);

  root = createRoot(mountPoint);
  await act(async () => {
    root?.render(<Wall count={count} wrapped={wrapped} />);
  });
  // Let the ResizeObserver delivery and the retry frames commit the window.
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 20));
  });
  return scroller;
}

async function rerender(count: number, wrapped: boolean): Promise<void> {
  await act(async () => {
    root?.render(<Wall count={count} wrapped={wrapped} />);
  });
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 20));
  });
}

describe("useVirtualRows", () => {
  it("windows a long library whose cards sit behind a zero-height wrapper", async () => {
    await mount(LONG_LIST, true);

    const current = reading();
    expect(current.windowed).toBe(true);
    // A reserved height is what makes the pane scrollable to the end at all.
    expect(current.totalHeight).toBeGreaterThan(0);
    expect(current.columns).toBe(4);
    // Only a window is mounted, not the whole library.
    expect(current.endIndex).toBeGreaterThan(current.startIndex);
    expect(current.endIndex).toBeLessThan(100);
  });

  it("windows a long library whose cards are direct children", async () => {
    await mount(LONG_LIST, false);

    const current = reading();
    expect(current.windowed).toBe(true);
    expect(current.totalHeight).toBeGreaterThan(0);
    expect(current.endIndex).toBeLessThan(100);
  });

  it("renders a short list in full and reserves nothing", async () => {
    await mount(40, true);

    const current = reading();
    expect(current.windowed).toBe(false);
    expect(current.startIndex).toBe(0);
    expect(current.endIndex).toBe(40);
    expect(current.totalHeight).toBe(0);
  });

  it("clamps the window when the list shrinks under it", async () => {
    await mount(LONG_LIST, true);
    expect(reading().windowed).toBe(true);

    await rerender(6, true);
    const current = reading();
    expect(current.windowed).toBe(false);
    expect(current.startIndex).toBe(0);
    expect(current.endIndex).toBe(6);
  });

  it("moves the window with the scroller", async () => {
    const scroller = await mount(LONG_LIST, true);
    expect(reading().startIndex).toBe(0);

    await act(async () => {
      scroller.scrollTop = 6000;
      scroller.dispatchEvent(new Event("scroll"));
      await new Promise((resolve) => setTimeout(resolve, 30));
    });

    const current = reading();
    expect(current.startIndex).toBeGreaterThan(0);
    expect(current.windowed).toBe(true);
  });
});
