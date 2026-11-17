import { describe, expect, it } from "vitest";

import { makeGroup, makeVideo } from "../test/fixtures";
import {
  buildFilterChips,
  chipKindLabel,
  collectActorOptions,
  diskOptionLabel,
  filterCount,
  filterVideos,
  NO_FILTERS,
  pageActorOptions,
  removeFilterChip,
  toggleFilterValue,
  withSelectedActors,
  withSelectedDisks,
  type LibraryFilters,
} from "./libraryFilters";

/**
 * The phone's filter semantics.
 *
 * These rules are the whole reason the sheet needs no second draft: values inside
 * one field are a union, fields combine with AND, and an empty field means "no
 * restriction". They are easy to invert by accident and impossible to notice from
 * a screenshot, so they are pinned here.
 */

function filters(overrides: Partial<LibraryFilters> = {}): LibraryFilters {
  return { ...NO_FILTERS, ...overrides };
}

describe("filterVideos", () => {
  const library = [
    makeVideo({ id: 1, actors: ["新垣结衣"], volumeRoot: "D:\\" }),
    makeVideo({ id: 2, actors: ["新垣结衣", "石原里美"], volumeRoot: "E:\\" }),
    makeVideo({ id: 3, actors: ["石原里美"], volumeRoot: "D:\\" }),
    makeVideo({ id: 4, actors: [], volumeRoot: "\\\\nas\\video\\" }),
    makeVideo({ id: 5, actors: ["新垣结衣"], volumeRoot: "" }),
  ];

  it("returns the list untouched when nothing is selected", () => {
    expect(filterVideos(library, NO_FILTERS)).toBe(library);
  });

  it("unions the actors inside one field", () => {
    const kept = filterVideos(library, filters({ actors: ["新垣结衣", "石原里美"] }));
    expect(kept.map((video) => video.id)).toEqual([1, 2, 3, 5]);
  });

  it("unions the disks inside one field", () => {
    const kept = filterVideos(library, filters({ disks: ["d:\\", "\\\\nas\\video\\"] }));
    expect(kept.map((video) => video.id)).toEqual([1, 3, 4]);
  });

  it("combines actors and disks with AND", () => {
    const kept = filterVideos(library, filters({ actors: ["新垣结衣"], disks: ["e:\\"] }));
    expect(kept.map((video) => video.id)).toEqual([2]);
  });

  it("counts the selected conditions", () => {
    expect(filterCount(filters({ groupIds: [3], actors: ["a", "b"], disks: ["d:\\"] }))).toBe(4);
  });
});

describe("collectActorOptions", () => {
  it("counts the films of each actor and orders by count then name", () => {
    const options = collectActorOptions([
      makeVideo({ id: 1, actors: ["B", "A"] }),
      makeVideo({ id: 2, actors: ["A"] }),
      makeVideo({ id: 3, actors: ["A", "A"] }),
      makeVideo({ id: 4, actors: ["C"] }),
      makeVideo({ id: 5, actors: [] }),
    ]);

    // A video names each actor once, so listing "A" twice is still one film.
    expect(options).toEqual([
      { name: "A", count: 3 },
      { name: "B", count: 1 },
      { name: "C", count: 1 },
    ]);
  });

  it("ignores blank actor names", () => {
    expect(collectActorOptions([makeVideo({ actors: ["", "A"] })])).toEqual([{ name: "A", count: 1 }]);
  });
});

describe("withSelectedActors", () => {
  it("keeps a selected actor that the current list no longer contains", () => {
    const options = withSelectedActors([{ name: "A", count: 2 }], ["A", "gone"]);
    expect(options).toEqual([
      { name: "A", count: 2 },
      { name: "gone", count: 0 },
    ]);
  });
});

describe("pageActorOptions", () => {
  const options = [
    { name: "Alpha", count: 9 },
    { name: "Beta", count: 8 },
    { name: "Gamma", count: 7 },
    { name: "Delta", count: 6 },
  ];

  it("shows one page and counts what the page is still hiding", () => {
    const page = pageActorOptions(options, [], "", 2);
    expect(page.visible.map((option) => option.name)).toEqual(["Alpha", "Beta"]);
    expect(page.matched).toBe(4);
    expect(page.hidden).toBe(2);
  });

  it("filters by keyword, case-insensitively", () => {
    const page = pageActorOptions(options, [], "a", 10);
    expect(page.visible.map((option) => option.name)).toEqual(["Alpha", "Beta", "Gamma", "Delta"]);
    const gamma = pageActorOptions(options, [], "AMM", 10);
    expect(gamma.visible.map((option) => option.name)).toEqual(["Gamma"]);
    expect(gamma.hidden).toBe(0);
  });

  it("pins a selected actor the page would otherwise hide", () => {
    const page = pageActorOptions(options, ["Delta"], "", 2);
    expect(page.visible.map((option) => option.name)).toEqual(["Alpha", "Beta", "Delta"]);
    // The pinned selection is not counted as hidden: it is on screen.
    expect(page.hidden).toBe(2);
  });
});

describe("buildFilterChips", () => {
  it("lists the conditions in the sheet's order and names them", () => {
    const chips = buildFilterChips(
      filters({ groupIds: [2, 9], actors: ["A"], disks: ["d:\\"] }),
      [makeGroup({ id: 2, name: "女仆" })],
      [{ key: "d:\\", root: "D:\\", label: "D:", count: 3, offline: false }],
    );

    expect(chips).toEqual([
      { id: "group:2", kind: "group", value: 2, label: "女仆" },
      // A group that is gone still produces a removable chip.
      { id: "group:9", kind: "group", value: 9, label: "分组 9" },
      { id: "actor:A", kind: "actor", value: "A", label: "A" },
      { id: "disk:d:\\", kind: "disk", value: "d:\\", label: "D:" },
    ]);
    expect(chipKindLabel("group")).toBe("分组");
  });
});

describe("removeFilterChip", () => {
  it("removes only the chip's own value", () => {
    const next = removeFilterChip(
      filters({ groupIds: [1, 2], actors: ["A", "B"], disks: ["d:\\"] }),
      { id: "actor:A", kind: "actor", value: "A", label: "A" },
    );
    expect(next).toEqual(filters({ groupIds: [1, 2], actors: ["B"], disks: ["d:\\"] }));
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
