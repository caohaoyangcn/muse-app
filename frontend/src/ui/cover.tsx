import * as React from "react";

import { cn } from "../lib/utils";

/**
 * A cover that fills the frame it is given, cropped from the right.
 *
 * Wide packshots carry the front cover and the back cover side by side, and they
 * are common in this library. A centre crop lands exactly on the seam, so a 3:4
 * poster would show half of the front and half of the back; cropping from the
 * right shows the front cover. That rule lives here, once, so a new cover frame
 * (poster, row thumbnail, duplicate member) inherits it instead of re-deciding
 * it — and so the next "why is this cover cut in half?" is a one-line fix.
 *
 * `className` supplies the frame (width/height, aspect ratio, radius,
 * background) and everything else passes through to the `<img>`. The crop is
 * appended last, so a stray `object-fit`/`object-position` utility cannot undo
 * it: a view that genuinely needs the whole artwork (the detail hero, the cover
 * editor preview) uses a plain `<img>` instead.
 *
 * `alt` stays empty: the artwork is decorative, and the surrounding card, row or
 * link carries the accessible name.
 */
export interface CoverImageProps
  extends Omit<React.ImgHTMLAttributes<HTMLImageElement>, "alt" | "draggable" | "src"> {
  src: string;
}

export function CoverImage({ className, src, ...props }: CoverImageProps) {
  return (
    <img
      {...props}
      alt=""
      className={cn(className, "object-cover object-right")}
      draggable={false}
      src={src}
    />
  );
}
