import { describe, expect, it } from "vitest";

import { providerDisplayName } from "./providerName";

/**
 * The scrape record stores a provider id, the detail panel shows a name.
 *
 * Getting this wrong is invisible until a lookup falls back: the panel keeps
 * saying "JavBus" while the metadata on screen came from Avmoo.
 */
describe("providerDisplayName", () => {
  it("names the provider that actually answered", () => {
    expect(providerDisplayName("avmoo")).toBe("Avmoo");
    expect(providerDisplayName("javbus")).toBe("JavBus");
    expect(providerDisplayName("fc2cmadb")).toBe("FC2CMADB");
  });

  it("keeps a stored provider it does not know instead of borrowing another name", () => {
    expect(providerDisplayName("newprovider")).toBe("newprovider");
  });

  it("treats a missing provider as unnamed", () => {
    expect(providerDisplayName(undefined)).toBe("");
    expect(providerDisplayName("  ")).toBe("");
  });
});
