import { describe, expect, it } from "vitest";

import { makeVideo } from "../test/fixtures";
import {
  collectDiskOptions,
  diskKey,
  diskLabel,
  filterVideosByDisk,
  withSelectedDisks,
  type DiskOption,
} from "./diskFilter";

/**
 * The 硬盘 filter shared by both clients.
 *
 * A favorite's disk is the volume root the backend stamps on every read, so the
 * filter is a join over an identity string — case-insensitive, trimmed, and never
 * invented for a row that has no root. Getting this wrong silently hides
 * favorites behind a chip that looks reasonable, which is why it is pinned.
 */
describe("diskKey", () => {
  it("trims and lower-cases so one volume has one identity", () => {
    expect(diskKey("  D:\\ ")).toBe("d:\\");
    expect(diskKey("\\\\NAS\\Video\\")).toBe("\\\\nas\\video\\");
  });
});

describe("diskLabel", () => {
  it("drops the trailing separator and upper-cases a drive letter", () => {
    expect(diskLabel("D:\\")).toBe("D:");
    expect(diskLabel("d:/")).toBe("D:");
    expect(diskLabel("\\\\server\\share\\")).toBe("\\\\server\\share");
  });

  it("keeps a bare root readable", () => {
    expect(diskLabel("/")).toBe("/");
    expect(diskLabel("")).toBe("");
  });
});

describe("collectDiskOptions", () => {
  it("counts the loaded favorites per volume and marks a detached one", () => {
    const options = collectDiskOptions([
      makeVideo({ id: 1, volumeRoot: "D:\\" }),
      makeVideo({ id: 2, volumeRoot: "d:\\", volumeOffline: true }),
      makeVideo({ id: 3, volumeRoot: "E:\\" }),
      // No stamped root: there is no disk to attribute this favorite to.
      makeVideo({ id: 4, volumeRoot: "" }),
    ]);

    expect(options.map((option) => [option.key, option.label, option.count, option.offline])).toEqual([
      ["d:\\", "D:", 2, true],
      ["e:\\", "E:", 1, false],
    ]);
  });
});

describe("withSelectedDisks", () => {
  it("adds a selected volume the current list does not contain, at zero", () => {
    const options = withSelectedDisks(
      [{ key: "d:\\", root: "D:\\", label: "D:", count: 2, offline: false }],
      new Map([["e:\\", "E:\\"]]),
    );

    expect(options.map((option) => [option.key, option.count])).toEqual([
      ["d:\\", 2],
      ["e:\\", 0],
    ]);
  });

  it("sorts the result the way a file manager lists drives", () => {
    const options: DiskOption[] = [
      { key: "e:\\", root: "E:\\", label: "E:", count: 1, offline: false },
      { key: "d:\\", root: "D:\\", label: "D:", count: 1, offline: false },
    ];
    expect(withSelectedDisks(options, new Map()).map((option) => option.label)).toEqual(["D:", "E:"]);
  });
});

describe("filterVideosByDisk", () => {
  const library = [
    makeVideo({ id: 1, volumeRoot: "D:\\" }),
    makeVideo({ id: 2, volumeRoot: "E:\\" }),
    makeVideo({ id: 3, volumeRoot: "" }),
  ];

  it("returns the list untouched when no disk is selected", () => {
    expect(filterVideosByDisk(library, new Set())).toBe(library);
  });

  it("keeps the favorites on the selected volumes, matching the root case-insensitively", () => {
    expect(filterVideosByDisk(library, new Set([diskKey("d:\\")])).map((video) => video.id)).toEqual([1]);
    expect(filterVideosByDisk(library, new Set([diskKey("D:\\"), diskKey("E:\\")])).map((video) => video.id))
      .toEqual([1, 2]);
  });
});
