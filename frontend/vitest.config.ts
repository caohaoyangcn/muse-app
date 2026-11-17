import { defineConfig } from "vitest/config";

/**
 * Frontend tests.
 *
 * One environment for everything: jsdom. The interesting regressions in this
 * codebase are layout and interaction ones — the virtualizer measures element
 * boxes and the phone views render real DOM — and the pure helpers cost nothing
 * extra to run there, so there is no second environment to keep in step.
 *
 * `esbuild.jsx` is the automatic runtime, matching `tsconfig.json`'s `react-jsx`,
 * so test components do not need `React` in scope.
 */
export default defineConfig({
  esbuild: { jsx: "automatic" },
  test: {
    environment: "jsdom",
    // The virtualizer schedules its row window on requestAnimationFrame and
    // restores scroll offsets per frame; a visual (animated) jsdom keeps those
    // callbacks real instead of leaving them undefined.
    environmentOptions: { jsdom: { pretendToBeVisual: true } },
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
