// Browser entry for design work. Not referenced by index.html or mobile.html,
// so neither the Wails bundle nor the LAN web build includes it.
import { installPlatformStubs } from "./mock/platform";

installPlatformStubs();

import React from "react";
import { createRoot } from "react-dom/client";
import "../style.css";
import { applyZoom } from "../utils/zoom";
import { configureClient } from "../services/client";
import { mockClient } from "./mock/client";
import App from "../App";
import ErrorBoundary from "../ui/error-boundary";
import { AppTheme } from "../ui/theme";

configureClient(mockClient);

// The real entry restores the user's saved scale from localStorage; a preview
// must not inherit it, or every capture would be resized by whatever this
// browser profile last chose. 100% is the size the app opens with.
try {
  localStorage.removeItem("muse.zoom");
} catch {
  /* storage unavailable; the default is already 1 */
}
applyZoom(1);

document.documentElement.dataset.designPreview = "true";

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AppTheme>
      <ErrorBoundary scope="Muse · 设计预览" variant="page">
        <App />
      </ErrorBoundary>
    </AppTheme>
  </React.StrictMode>,
);
