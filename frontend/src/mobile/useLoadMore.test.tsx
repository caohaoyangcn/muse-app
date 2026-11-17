import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from "vitest";

import { PAGE_SIZE, useLoadMore, type LoadMoreState } from "./useLoadMore";

/**
 * The phone's paging contract.
 *
 * The phone appends the library a page at a time instead of windowing it (see
 * useLoadMore for why). What has to stay true is a small set of promises the
 * views rely on: the first page is bounded, the list never renders more than the
 * library holds, the button always advances exactly one page, the next page loads
 * when the footer comes into view, and the depth of each list survives opening a
 * video and coming back — without leaking between different lists.
 */

const TOTAL = 1217;

let latest: LoadMoreState | null = null;
let root: Root | null = null;
let host: HTMLElement | null = null;
let keySeq = 0;

interface ObserverRecord {
  callback: IntersectionObserverCallback;
  targets: Element[];
  disconnected: boolean;
}
let observers: ObserverRecord[] = [];

/** A fresh list identity per test: the page memory is module state by design. */
function freshKey(label: string): string {
  keySeq += 1;
  return `${label}:${keySeq}`;
}

function state(): LoadMoreState {
  if (!latest) throw new Error("the paged view has not rendered yet");
  return latest;
}

beforeAll(() => {
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

  // jsdom has no IntersectionObserver. The stub records every observer so a test
  // can make the footer "come into view" the way scrolling would.
  class IntersectionObserverShim {
    private readonly record: ObserverRecord;
    constructor(callback: IntersectionObserverCallback) {
      this.record = { callback, targets: [], disconnected: false };
      observers.push(this.record);
    }
    observe(target: Element) {
      this.record.targets.push(target);
    }
    unobserve() {}
    disconnect() {
      this.record.disconnected = true;
    }
    takeRecords(): IntersectionObserverEntry[] {
      return [];
    }
  }
  globalThis.IntersectionObserver = IntersectionObserverShim as unknown as typeof IntersectionObserver;
});

afterAll(() => {
  delete (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT;
});

beforeEach(() => {
  observers = [];
});

afterEach(async () => {
  latest = null;
  await act(async () => {
    root?.unmount();
  });
  root = null;
  host?.remove();
  host = null;
});

function Paged({ total, memoryKey }: { total: number; memoryKey: string }) {
  latest = useLoadMore({ total, memoryKey });
  return <div ref={latest.sentinelRef} />;
}

async function mount(total: number, memoryKey: string): Promise<void> {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  await act(async () => {
    root?.render(<Paged total={total} memoryKey={memoryKey} />);
  });
}

async function rerender(total: number, memoryKey: string): Promise<void> {
  await act(async () => {
    root?.render(<Paged total={total} memoryKey={memoryKey} />);
  });
}

/** The observer watching the current footer, if the footer is mounted. */
function footerObserver(): ObserverRecord {
  const active = observers.filter((observer) => !observer.disconnected);
  const last = active[active.length - 1];
  if (!last) throw new Error("no footer observer is active");
  return last;
}

describe("useLoadMore", () => {
  it("renders one page and reports the rest as still behind the button", async () => {
    await mount(TOTAL, freshKey("first"));

    expect(state().visibleCount).toBe(PAGE_SIZE);
    expect(state().hasMore).toBe(true);
  });

  it("advances one page per request and never past the end", async () => {
    await mount(TOTAL, freshKey("advance"));

    act(() => {
      state().loadMore();
    });
    expect(state().visibleCount).toBe(PAGE_SIZE * 2);

    act(() => {
      for (let i = 0; i < 40; i += 1) state().loadMore();
    });
    expect(state().visibleCount).toBe(TOTAL);
    expect(state().hasMore).toBe(false);
  });

  it("loads the next page when the footer comes into view", async () => {
    await mount(TOTAL, freshKey("observer"));
    expect(state().visibleCount).toBe(PAGE_SIZE);

    act(() => {
      const entry = { isIntersecting: true } as IntersectionObserverEntry;
      footerObserver().callback([entry], {} as IntersectionObserver);
    });

    expect(state().visibleCount).toBe(PAGE_SIZE * 2);
  });

  it("keeps each list's depth and restores it when the user comes back", async () => {
    const photoWall = freshKey("wall");
    const otherList = freshKey("other");

    await mount(TOTAL, photoWall);
    act(() => {
      state().loadMore();
      state().loadMore();
    });
    expect(state().visibleCount).toBe(PAGE_SIZE * 3);

    // A different filter (or sort, or keyword) is a different list: it starts at
    // its own first page instead of inheriting the depth above.
    await rerender(TOTAL, otherList);
    expect(state().visibleCount).toBe(PAGE_SIZE);

    // Returning to the wall — a detail screen goes back to the same list —
    // restores the place the user had reached.
    await rerender(TOTAL, photoWall);
    expect(state().visibleCount).toBe(PAGE_SIZE * 3);
  });

  it("clamps the depth when the list shrank under it", async () => {
    const shrinking = freshKey("shrink");

    await mount(500, shrinking);
    act(() => {
      state().loadMore();
      state().loadMore();
    });
    expect(state().visibleCount).toBe(PAGE_SIZE * 3);

    await rerender(100, shrinking);
    expect(state().visibleCount).toBe(100);
    expect(state().hasMore).toBe(false);
    act(() => {
      state().loadMore();
    });
    expect(state().visibleCount).toBe(100);
  });

  it("has nothing to load for a library that fits in one page", async () => {
    await mount(PAGE_SIZE - 1, freshKey("short"));

    expect(state().visibleCount).toBe(PAGE_SIZE - 1);
    expect(state().hasMore).toBe(false);
    act(() => {
      state().loadMore();
    });
    expect(state().visibleCount).toBe(PAGE_SIZE - 1);
  });
});
