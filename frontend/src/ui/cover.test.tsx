import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, afterAll, describe, expect, it } from "vitest";

import { CoverImage } from "./cover";

/**
 * The one cover crop.
 *
 * Wide packshots carry the front cover and the back cover side by side, so a
 * centre crop in a poster frame lands on the seam and the card quietly shows
 * half of each. The rule lives in this component instead of in every view, so it
 * is pinned here: a cover frame that stops cropping from the right is a bug that
 * no other test would notice.
 */

let root: Root | null = null;
let host: HTMLElement | null = null;

function render(node: ReactNode): HTMLImageElement {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  act(() => root?.render(node));
  const img = host.querySelector("img");
  if (!img) throw new Error("CoverImage rendered no image");
  return img;
}

beforeAll(() => {
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
});

afterAll(() => {
  delete (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT;
});

afterEach(() => {
  act(() => root?.unmount());
  root = null;
  host?.remove();
  host = null;
});

describe("CoverImage", () => {
  it("crops from the right so a poster shows the front cover, not the seam", () => {
    const img = render(<CoverImage className="size-full" src="/thumbnails/1.jpg" />);

    expect(img.className).toContain("object-cover");
    expect(img.className).toContain("object-right");
  });

  it("keeps the caller's frame without letting a stray object-fit undo the crop", () => {
    // Built at runtime on purpose: these classes exist only to prove the
    // primitive wins a merge conflict, and a literal would make Tailwind ship
    // an unused object-position utility in both bundles.
    const strayFit = ["object", "contain"].join("-");
    const strayPosition = ["object", "center"].join("-");
    const img = render(
      <CoverImage className={`h-14 w-24 ${strayFit} ${strayPosition}`} src="/thumbnails/1.jpg" />,
    );

    expect(img.className).toContain("h-14");
    expect(img.className).toContain("w-24");
    expect(img.className).toContain("object-cover");
    expect(img.className).toContain("object-right");
    expect(img.className).not.toContain(strayFit);
    expect(img.className).not.toContain(strayPosition);
  });

  it("stays decorative, leaving the accessible name to the surrounding control", () => {
    const img = render(<CoverImage className="size-full" src="/thumbnails/1.jpg" />);

    expect(img.getAttribute("alt")).toBe("");
  });
});
