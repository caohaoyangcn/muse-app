import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";

import { usePagedList, type PagedListOptions, type PagedListState } from "./usePagedList";

/**
 * The paging contract shared by both shells.
 *
 * Every list renders one page and replaces it; what has to stay true is a small
 * set of promises the views rely on: the first page is bounded, the buttons
 * move exactly one page and never past the ends, each list remembers its own
 * depth, returning to it restores that depth, a shrinking list clamps, and a
 * keyboard selection that moves off the page is followed while a manual page
 * change is not.
 */

const TOTAL = 1217;

let latest: PagedListState | null = null;
let root: Root | null = null;
let host: HTMLElement | null = null;
let keySeq = 0;

/** A fresh list identity per test: the page memory is module state by design. */
function freshKey(label: string): string {
  keySeq += 1;
  return `${label}:${keySeq}`;
}

function state(): PagedListState {
  if (!latest) throw new Error("the paged view has not rendered yet");
  return latest;
}

beforeAll(() => {
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
});

afterAll(() => {
  delete (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT;
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

function Paged(props: PagedListOptions) {
  latest = usePagedList(props);
  return null;
}

async function mount(props: PagedListOptions): Promise<void> {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  await act(async () => {
    root?.render(<Paged {...props} />);
  });
}

async function rerender(props: PagedListOptions): Promise<void> {
  await act(async () => {
    root?.render(<Paged {...props} />);
  });
}

describe("usePagedList", () => {
  it("renders one page and counts the rest as further pages", async () => {
    await mount({ total: TOTAL, listKey: freshKey("first"), pageSize: 50 });

    expect(state().page).toBe(1);
    expect(state().pageCount).toBe(25);
    expect(state().startIndex).toBe(0);
    expect(state().endIndex).toBe(50);
    expect(state().hasPrev).toBe(false);
    expect(state().hasNext).toBe(true);
  });

  it("ends the last page at the total instead of padding it", async () => {
    await mount({ total: TOTAL, listKey: freshKey("last"), pageSize: 50 });

    act(() => {
      state().goToPage(state().pageCount);
    });

    expect(state().page).toBe(25);
    expect(state().startIndex).toBe(1200);
    expect(state().endIndex).toBe(TOTAL);
    expect(state().hasNext).toBe(false);
  });

  it("moves exactly one page per request and clamps at both ends", async () => {
    await mount({ total: TOTAL, listKey: freshKey("step"), pageSize: 60 });

    act(() => {
      state().nextPage();
    });
    expect(state().page).toBe(2);
    expect(state().startIndex).toBe(60);

    act(() => {
      state().prevPage();
    });
    expect(state().page).toBe(1);

    act(() => {
      state().prevPage();
    });
    expect(state().page).toBe(1);

    act(() => {
      for (let i = 0; i < 40; i += 1) state().nextPage();
    });
    expect(state().page).toBe(state().pageCount);
    expect(state().endIndex).toBe(TOTAL);
  });

  it("keeps each list's depth and restores it when the user comes back", async () => {
    const photoWall = freshKey("wall");
    const otherList = freshKey("other");

    await mount({ total: TOTAL, listKey: photoWall, pageSize: 60 });
    act(() => {
      state().nextPage();
      state().nextPage();
    });
    expect(state().page).toBe(3);

    // A different filter (or sort, or keyword) is a different list: it starts at
    // its own first page instead of inheriting the depth above.
    await rerender({ total: TOTAL, listKey: otherList, pageSize: 60 });
    expect(state().page).toBe(1);

    // Returning to the wall — a detail screen goes back to the same list —
    // restores the page the user had reached.
    await rerender({ total: TOTAL, listKey: photoWall, pageSize: 60 });
    expect(state().page).toBe(3);
  });

  it("does not erase the remembered page while the first read is still empty", async () => {
    const list = freshKey("loading");

    await mount({ total: TOTAL, listKey: list, pageSize: 50 });
    act(() => {
      state().nextPage();
    });
    expect(state().page).toBe(2);
    await act(async () => {
      root?.unmount();
    });
    root = null;
    host?.remove();
    host = null;

    // The screen remounts before its data arrives; the empty list clamps to
    // page 1, but that clamp is not an explicit page change and must not be
    // filed as the depth.
    await mount({ total: 0, listKey: list, pageSize: 50 });
    expect(state().page).toBe(1);

    await rerender({ total: TOTAL, listKey: list, pageSize: 50 });
    expect(state().page).toBe(2);
  });

  it("clamps the page when the list shrank under it", async () => {
    const shrinking = freshKey("shrink");

    await mount({ total: 500, listKey: shrinking, pageSize: 50 });
    act(() => {
      state().goToPage(3);
    });
    expect(state().page).toBe(3);

    await rerender({ total: 100, listKey: shrinking, pageSize: 50 });
    expect(state().page).toBe(2);
    expect(state().endIndex).toBe(100);

    act(() => {
      state().nextPage();
    });
    expect(state().page).toBe(2);

    act(() => {
      state().prevPage();
    });
    expect(state().page).toBe(1);
  });

  it("follows a keyboard selection only when it moves off the current page", async () => {
    const list = freshKey("reveal");

    await mount({ total: TOTAL, listKey: list, pageSize: 50, revealIndex: 0 });
    expect(state().page).toBe(1);

    await rerender({ total: TOTAL, listKey: list, pageSize: 50, revealIndex: 120 });
    expect(state().page).toBe(3);
    expect(state().startIndex).toBe(100);

    // Moving within the page does not turn it.
    await rerender({ total: TOTAL, listKey: list, pageSize: 50, revealIndex: 130 });
    expect(state().page).toBe(3);

    // A manual page change is not undone by the selection that is still on the
    // old page.
    act(() => {
      state().prevPage();
    });
    expect(state().page).toBe(2);
    await rerender({ total: TOTAL, listKey: list, pageSize: 50, revealIndex: 130 });
    expect(state().page).toBe(2);

    // A new selection on another page is followed again.
    await rerender({ total: TOTAL, listKey: list, pageSize: 50, revealIndex: 350 });
    expect(state().page).toBe(8);
  });

  it("does not override a restored page with a selection present at mount", async () => {
    const list = freshKey("mount-reveal");

    await mount({ total: TOTAL, listKey: list, pageSize: 50 });
    act(() => {
      state().nextPage();
    });
    expect(state().page).toBe(2);
    await act(async () => {
      root?.unmount();
    });
    root = null;
    host?.remove();
    host = null;

    await mount({ total: TOTAL, listKey: list, pageSize: 50, revealIndex: 300 });
    expect(state().page).toBe(2);
  });

  it("has one page and nothing to load for a short list", async () => {
    await mount({ total: 10, listKey: freshKey("short"), pageSize: 50 });

    expect(state().pageCount).toBe(1);
    expect(state().page).toBe(1);
    expect(state().startIndex).toBe(0);
    expect(state().endIndex).toBe(10);

    act(() => {
      state().nextPage();
      state().prevPage();
    });
    expect(state().page).toBe(1);
  });
});
