import type { ReactNode } from "react";
import { Theme } from "@astryxdesign/core/theme";
import { neutralTheme } from "@astryxdesign/theme-neutral/built";

/**
 * Astryx provider for both clients.
 *
 * The palette is deliberately *not* passed in here. `src/theme.css` points
 * Astryx's colour and radius tokens at the product tokens (`--accent`,
 * `--panel`, `--radius`, …) for every `[data-astryx-theme]` subtree, so a
 * product colour change stays a one-file edit and the design system cannot
 * drift from the hand-written shell. Building the theme with `defineTheme()`
 * instead would copy those values into TypeScript and leave two palettes to
 * keep in step.
 *
 * `mode="dark"` is fixed because Muse has no light theme. The root Theme also
 * writes `data-theme` / `data-astryx-theme` onto `<html>`, which is what makes
 * portalled surfaces — dialog, popover, toast — resolve the same tokens as the
 * shell they open over.
 */
export function AppTheme({ children }: { children: ReactNode }) {
  return (
    <Theme theme={neutralTheme} mode="dark">
      {children}
    </Theme>
  );
}
