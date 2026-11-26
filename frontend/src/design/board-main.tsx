// Entry for the design-system sheet. See board.tsx for why the sheet exists.
import { installPlatformStubs } from "./mock/platform";

installPlatformStubs();

import React from "react";
import { createRoot } from "react-dom/client";
import "../style.css";
import "./board.css";
import { configureClient } from "../services/client";
import { mockClient } from "./mock/client";
import { applyZoom } from "../utils/zoom";
import Board from "./board";
import { AppTheme } from "../ui/theme";

configureClient(mockClient);

try {
  localStorage.removeItem("muse.zoom");
} catch {
  /* storage unavailable; the default is already 1 */
}
applyZoom(1);

// Unlock the shell's one-viewport document so the sheet can be one long page.
document.documentElement.dataset.designBoard = "true";

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AppTheme>
      <Board />
    </AppTheme>
  </React.StrictMode>,
);
