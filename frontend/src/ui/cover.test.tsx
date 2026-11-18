import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";

import { CoverImage } from "./cover";

/**
 * The two cover presentations.
 *
 * Wide packshots carry the front cover and the back cover side by side, so the
 * frame decides what the reader sees: the 3:4 poster crops to the front cover,
 * the 16:9 card fits the whole artwork. Getting this wrong never throws — the
 * card just quietly shows half of each — which is why both variants are pinned
 * here instead of relying on each view's className.
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
  it("portrait posters crop from the right so the cover, not the seam, is shown", () => {
    const img = render(<CoverImage className="size-full" src="/thumbnails/1.jpg" variant="portrait" />);

    expect(img.className).toContain("object-cover");
    expect(img.className).toContain("object-right");
    expect(img.className).not.toContain("object-contain");
  });

  it("landscape cards fit the whole packshot, back cover included", () => {
    const img = render(
      <CoverImage className="block size-full" src="/thumbnails/1.jpg" variant="landscape" />,
    );

    expect(img.className).toContain("object-contain");
    expect(img.className).not.toContain("object-cover");
    expect(img.className).not.toContain("object-right");
  });

  it("keeps the caller's frame without letting a stray fit undo the variant", () => {
    // Built at runtime on purpose: these classes only prove the variant wins a
    // merge conflict, and a literal would make Tailwind ship an unused utility.
    const strayFit = ["object", "contain"].join("-");
    const strayPosition = ["object", "center"].join("-");
    const img = render(
      <CoverImage
        className={`h-14 w-24 ${strayFit} ${strayPosition}`}
        src="/thumbnails/1.jpg"
        variant="portrait"
      />,
    );

    expect(img.className).toContain("h-14");
    expect(img.className).toContain("w-24");
    expect(img.className).toContain("object-cover");
    expect(img.className).toContain("object-right");
    expect(img.className).not.toContain(strayFit);
    expect(img.className).not.toContain(strayPosition);
  });

  it("stays decorative, leaving the accessible name to the surrounding control", () => {
    const img = render(<CoverImage className="size-full" src="/thumbnails/1.jpg" variant="portrait" />);

    expect(img.getAttribute("alt")).toBe("");
  });
});
