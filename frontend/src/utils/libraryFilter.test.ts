import { describe, expect, it } from "vitest";

import { makeVideo } from "../test/fixtures";
import {
  buildFilterChips,
  chipKindLabel,
  collectActorOptions,
  collectTagOptions,
  diskOptionLabel,
  filterCount,
  filterVideos,
  NO_FILTERS,
  pageValueOptions,
  removeFilterChip,
  toggleFilterValue,
  withSelectedDisks,
  withSelectedValues,
  type LibraryFilters,
} from "./libraryFilter";

/**
 * The library's filter semantics, shared by the desktop app and the phone.
 *
 * These rules are the whole reason the filter surface needs no second draft:
 * values inside one field are a union, fields combine with AND, and an empty
 * field means "no restriction". They are easy to invert by accident and
 * impossible to notice from a screenshot, so they are pinned here.
 */

function filters(overrides: Partial<LibraryFilters> = {}): LibraryFilters {
  return { ...NO_FILTERS, ...overrides };
}

describe("filterVideos", () => {
  const library = [
    makeVideo({ id: 1, tags: ["剧情"], actors: ["新垣结衣"], volumeRoot: "D:\\" }),
    makeVideo({ id: 2, tags: ["剧情", "巨乳"], actors: ["新垣结衣", "石原里美"], volumeRoot: "E:\\" }),
    makeVideo({ id: 3, tags: ["巨乳"], actors: ["石原里美"], volumeRoot: "D:\\" }),
    makeVideo({ id: 4, tags: [], actors: [], volumeRoot: "\\\\nas\\video\\" }),
    makeVideo({ id: 5, tags: ["剧情"], actors: ["新垣结衣"], volumeRoot: "" }),
  ];

  it("returns the list untouched when nothing is selected", () => {
    expect(filterVideos(library, NO_FILTERS)).toBe(library);
  });

  it("unions the tags inside one field", () => {
    const kept = filterVideos(library, filters({ tags: ["剧情", "巨乳"] }));
    expect(kept.map((video) => video.id)).toEqual([1, 2, 3, 5]);
  });

  it("unions the actors inside one field", () => {
    const kept = filterVideos(library, filters({ actors: ["新垣结衣", "石原里美"] }));
    expect(kept.map((video) => video.id)).toEqual([1, 2, 3, 5]);
  });

  it("unions the disks inside one field", () => {
    const kept = filterVideos(library, filters({ disks: ["d:\\", "\\\\nas\\video\\"] }));
    expect(kept.map((video) => video.id)).toEqual([1, 3, 4]);
  });

  it("combines tags, actors and disks with AND", () => {
    const kept = filterVideos(
      library,
      filters({ tags: ["剧情"], actors: ["新垣结衣"], disks: ["e:\\"] }),
    );
    expect(kept.map((video) => video.id)).toEqual([2]);
  });

  it("counts the selected conditions", () => {
    expect(filterCount(filters({ tags: ["a", "b"], actors: ["c"], disks: ["d:\\"] }))).toBe(4);
  });
});

describe("collectTagOptions", () => {
  it("counts the films of each tag and orders by count then name", () => {
    const options = collectTagOptions([
      makeVideo({ id: 1, tags: ["B", "A"] }),
      makeVideo({ id: 2, tags: ["A"] }),
      makeVideo({ id: 3, tags: ["A", "A"] }),
      makeVideo({ id: 4, tags: ["C"] }),
      makeVideo({ id: 5, tags: [] }),
    ]);

    // A video names each tag once, so listing "A" twice is still one film.
    expect(options).toEqual([
      { name: "A", count: 3 },
      { name: "B", count: 1 },
      { name: "C", count: 1 },
    ]);
  });

  it("ignores blank tags", () => {
    expect(collectTagOptions([makeVideo({ tags: ["", "A"] })])).toEqual([{ name: "A", count: 1 }]);
  });
});

describe("collectActorOptions", () => {
  it("counts the films of each actor", () => {
    expect(
      collectActorOptions([makeVideo({ actors: ["A"] }), makeVideo({ actors: ["A", "B"] })]),
    ).toEqual([
      { name: "A", count: 2 },
      { name: "B", count: 1 },
    ]);
  });
});

describe("withSelectedValues", () => {
  it("keeps a selected value that the current list no longer contains", () => {
    const options = withSelectedValues([{ name: "A", count: 2 }], ["A", "gone"]);
    expect(options).toEqual([
      { name: "A", count: 2 },
      { name: "gone", count: 0 },
    ]);
  });
});

describe("pageValueOptions", () => {
  const options = [
    { name: "Alpha", count: 9 },
    { name: "Beta", count: 8 },
    { name: "Gamma", count: 7 },
    { name: "Delta", count: 6 },
  ];

  it("shows one page and counts what the page is still hiding", () => {
    const page = pageValueOptions(options, [], "", 2);
    expect(page.visible.map((option) => option.name)).toEqual(["Alpha", "Beta"]);
    expect(page.matched).toBe(4);
    expect(page.hidden).toBe(2);
  });

  it("filters by keyword, case-insensitively", () => {
    const page = pageValueOptions(options, [], "a", 10);
    expect(page.visible.map((option) => option.name)).toEqual(["Alpha", "Beta", "Gamma", "Delta"]);
    const gamma = pageValueOptions(options, [], "AMM", 10);
    expect(gamma.visible.map((option) => option.name)).toEqual(["Gamma"]);
    expect(gamma.hidden).toBe(0);
  });

  it("pins a selected value the page would otherwise hide", () => {
    const page = pageValueOptions(options, ["Delta"], "", 2);
    expect(page.visible.map((option) => option.name)).toEqual(["Alpha", "Beta", "Delta"]);
    // The pinned selection is not counted as hidden: it is on screen.
    expect(page.hidden).toBe(2);
  });
});

describe("buildFilterChips", () => {
  it("lists the conditions in the surface's order and names them", () => {
    const chips = buildFilterChips(filters({ tags: ["剧情"], actors: ["A"], disks: ["d:\\"] }), [
      { key: "d:\\", root: "D:\\", label: "D:", count: 3, offline: false },
    ]);

    expect(chips).toEqual([
      { id: "tag:剧情", kind: "tag", value: "剧情", label: "剧情" },
      { id: "actor:A", kind: "actor", value: "A", label: "A" },
      { id: "disk:d:\\", kind: "disk", value: "d:\\", label: "D:" },
    ]);
    expect(chipKindLabel("tag")).toBe("标签");
  });

  it("still labels a selected disk that is not in the current options", () => {
    const chips = buildFilterChips(filters({ disks: ["e:\\"] }), []);
    expect(chips).toEqual([{ id: "disk:e:\\", kind: "disk", value: "e:\\", label: "E:" }]);
  });
});

describe("removeFilterChip", () => {
  it("removes only the chip's own value", () => {
    const next = removeFilterChip(
      filters({ tags: ["剧情", "巨乳"], actors: ["A", "B"], disks: ["d:\\"] }),
      { id: "tag:剧情", kind: "tag", value: "剧情", label: "剧情" },
    );
    expect(next).toEqual(filters({ tags: ["巨乳"], actors: ["A", "B"], disks: ["d:\\"] }));
  });
});

describe("toggleFilterValue", () => {
  it("adds a value in the order it was picked and removes it again", () => {
    const picked = toggleFilterValue(toggleFilterValue([], "b"), "a");
    expect(picked).toEqual(["b", "a"]);
    expect(toggleFilterValue(picked, "b")).toEqual(["a"]);
  });
});

describe("diskOptionLabel", () => {
  it("names the Unix root instead of showing a bare slash", () => {
    expect(diskOptionLabel({ key: "/", root: "/", label: "/", count: 1, offline: false })).toBe("本机磁盘");
    expect(diskOptionLabel({ key: "/mnt/a", root: "/mnt/a", label: "/mnt/a", count: 1, offline: false })).toBe("/mnt/a");
  });
});

describe("withSelectedDisks", () => {
  it("keeps a selected disk that the current list no longer contains", () => {
    const options = withSelectedDisks(
      [{ key: "d:\\", root: "D:\\", label: "D:", count: 2, offline: false }],
      ["d:\\", "e:\\"],
    );
    expect(options.map((option) => [option.key, option.count, option.label])).toEqual([
      ["d:\\", 2, "D:"],
      ["e:\\", 0, "E:"],
    ]);
  });
});
