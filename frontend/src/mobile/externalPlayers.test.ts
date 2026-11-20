import { afterEach, describe, expect, it, vi } from "vitest";

import { loginWithKey } from "../services/web";
import { externalStreamURL } from "./externalPlayers";

afterEach(() => {
  vi.unstubAllGlobals();
});

/**
 * A native player cannot send the session cookie, so the URL it receives must
 * carry the key itself. This is the one place the LAN client exposes the key in
 * a URL, and the server only accepts it on /stream/.
 */
describe("externalStreamURL", () => {
  it("carries the session key so a native player can authenticate", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response(JSON.stringify({ authenticated: true, key: "abcdefghijklmnop" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    await loginWithKey("abcdefghijklmnop");

    expect(externalStreamURL(7)).toBe(
      `${window.location.origin}/stream/video/7?key=abcdefghijklmnop`,
    );
  });
});
