import * as React from "react";

import { cn } from "../lib/utils";

/**
 * The two ways Muse presents a cover. Every framed cover goes through this
 * component and names its variant, so a new card, row or preview picks one of
 * the two instead of quietly inventing a third fit.
 *
 * Covers in this library are often wide packshots: the front cover and the back
 * cover side by side. That leaves two honest presentations:
 *
 * - `portrait` (竖版, the 3:4 poster): crop from the right, so the frame shows
 *   the front cover only. A centre crop lands exactly on the seam and shows
 *   half of each — the failure this variant exists to prevent.
 * - `landscape` (横版, the 16:9 card and row thumbnail): fit the whole artwork
 *   inside the frame, front cover and back cover together. Cropping a packshot
 *   that already fits would cut it in half for nothing.
 *
 * `className` supplies the frame (width/height, aspect ratio, radius,
 * background) and everything else passes through to the `<img>`. The variant's
 * fit is appended last, so a stray object-fit/object-position utility cannot
 * undo it.
 *
 * Views that show the artwork at its own size rather than inside a frame (the
 * detail hero, the cover viewer, the cover editor preview) use a plain `<img>`:
 * there is no crop decision to make there.
 *
 * `alt` stays empty: the artwork is decorative, and the surrounding card, row or
 * link carries the accessible name.
 */
const FIT = {
  portrait: "object-cover object-right",
  landscape: "object-contain",
} as const;

export type CoverVariant = keyof typeof FIT;

export interface CoverImageProps
  extends Omit<React.ImgHTMLAttributes<HTMLImageElement>, "alt" | "draggable" | "src"> {
  src: string;
  /** 竖版只露封面（右对齐裁切）；横版露完整封面+封底（等比收进框内）。 */
  variant: CoverVariant;
}

export function CoverImage({ className, src, variant, ...props }: CoverImageProps) {
  return (
    <img
      {...props}
      alt=""
      className={cn(className, FIT[variant])}
      draggable={false}
      src={src}
    />
  );
}
