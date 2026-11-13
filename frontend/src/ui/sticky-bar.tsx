import * as React from "react";
import { cn } from "../lib/utils";

const StickyBar = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      data-slot="sticky-bar"
      className={cn(
        "sticky top-0 z-[5] mb-3 flex items-center gap-2 bg-[var(--bg)] py-3.5 text-[13px] text-[var(--muted)]",
        className,
      )}
      {...props}
    />
  ),
);
StickyBar.displayName = "StickyBar";

export { StickyBar };
