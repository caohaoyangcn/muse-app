import { describe, expect, it } from "vitest";

import { makeVideo } from "../test/fixtures";
import { compareLastOpened } from "./sortVideos";

/**
 * The 最近播放 ordering, shared by both clients.
 *
 * One rule matters more than the rest: never-played favorites stay after every
 * played one in *both* directions. Otherwise flipping the arrow would drag a wall
 * of unplayed rows to the top and bury the history the user asked for.
 */
describe("compareLastOpened", () => {
  const never = makeVideo({ id: 1, createdAt: 100, lastOpenedAt: 0 });
  const older = makeVideo({ id: 2, createdAt: 200, lastOpenedAt: 1_000 });
  const newer = makeVideo({ id: 3, createdAt: 300, lastOpenedAt: 2_000 });

  it("orders played favorites by playback time, newest first by default", () => {
    expect([older, newer].sort((a, b) => compareLastOpened(a, b, "desc")).map((v) => v.id)).toEqual([3, 2]);
  });

  it("reverses the played favorites for ascending", () => {
    expect([newer, older].sort((a, b) => compareLastOpened(a, b, "asc")).map((v) => v.id)).toEqual([2, 3]);
  });

  it("keeps never-played favorites last in both directions", () => {
    expect([never, older, newer].sort((a, b) => compareLastOpened(a, b, "desc")).map((v) => v.id)).toEqual([
      3, 2, 1,
    ]);
    // Ascending reverses the played block only; the never-played row stays last.
    expect([newer, never, older].sort((a, b) => compareLastOpened(a, b, "asc")).map((v) => v.id)).toEqual([
      2, 3, 1,
    ]);
    expect(compareLastOpened(never, older, "asc")).toBeGreaterThan(0);
    expect(compareLastOpened(older, never, "asc")).toBeLessThan(0);
  });

  it("keeps never-played favorites in newest-added order among themselves", () => {
    const olderAdded = makeVideo({ id: 5, createdAt: 100 });
    const newerAdded = makeVideo({ id: 6, createdAt: 900 });
    expect([olderAdded, newerAdded].sort((a, b) => compareLastOpened(a, b, "asc")).map((v) => v.id)).toEqual([
      6, 5,
    ]);
  });

  it("breaks ties by id so the order is stable", () => {
    const first = makeVideo({ id: 10, lastOpenedAt: 5_000 });
    const second = makeVideo({ id: 11, lastOpenedAt: 5_000 });
    expect([second, first].sort((a, b) => compareLastOpened(a, b, "desc")).map((v) => v.id)).toEqual([10, 11]);
  });
});
