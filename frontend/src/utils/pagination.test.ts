import { describe, expect, it } from "vitest";

import { pageShortcuts } from "./pagination";

/**
 * The shortcut strip is a user-visible promise: page 10 of 21 must offer
 * 1, 5–15 and 21, with no duplicate when the first, current-5 or last coincide.
 */
describe("pageShortcuts", () => {
  it("surrounds the current page with five neighbours, plus the first and last", () => {
    expect(pageShortcuts(10, 21)).toEqual([1, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 21]);
  });

  it("clamps the window at the start without repeating the first page", () => {
    expect(pageShortcuts(1, 21)).toEqual([1, 2, 3, 4, 5, 6, 21]);
  });

  it("clamps the window at the end without repeating the last page", () => {
    expect(pageShortcuts(21, 21)).toEqual([1, 16, 17, 18, 19, 20, 21]);
  });

  it("lists every page of a short list once", () => {
    expect(pageShortcuts(2, 3)).toEqual([1, 2, 3]);
  });

  it("stays inside the list when the page is out of range", () => {
    expect(pageShortcuts(99, 4)).toEqual([1, 2, 3, 4]);
    expect(pageShortcuts(-3, 4)).toEqual([1, 2, 3, 4]);
  });
});
