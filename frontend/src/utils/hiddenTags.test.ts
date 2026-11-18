import { describe, expect, it } from "vitest";

import type { ValueOption } from "./libraryFilter";
import { toggleHiddenTag, withoutHiddenTags } from "./hiddenTags";

/**
 * The sidebar's quick list is a shortcut, so hiding a tag is a display-only
 * choice. These tests pin the two behaviours that are easy to break silently:
 * hidden tags disappear from the shortcut, and toggling one does not disturb
 * the others.
 */

const options: ValueOption[] = [
  { name: "剧情", count: 9 },
  { name: "巨乳", count: 7 },
  { name: "单体作品", count: 4 },
];

describe("toggleHiddenTag", () => {
  it("adds and removes one tag without disturbing the rest", () => {
    const hidden = toggleHiddenTag(["剧情"], "巨乳");
    expect(hidden).toEqual(["剧情", "巨乳"]);
    expect(toggleHiddenTag(hidden, "剧情")).toEqual(["巨乳"]);
  });
});

describe("withoutHiddenTags", () => {
  it("drops hidden tags but leaves every other option in order", () => {
    expect(withoutHiddenTags(options, ["巨乳"]).map((option) => option.name)).toEqual([
      "剧情",
      "单体作品",
    ]);
  });

  it("returns the options unchanged when nothing is hidden", () => {
    expect(withoutHiddenTags(options, [])).toEqual(options);
  });
});
