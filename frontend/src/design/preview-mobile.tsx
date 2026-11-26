// Phone (LAN web) preview for design work.
//
// The mobile shell talks HTTP through services/web.ts, so unlike the desktop
// preview it needs no Wails stubs — it needs `fetch`. Everything the shell calls
// outside the shared client (`getVideo`, `markVideoOpened`, session) goes to the
// same two paths, and this module answers them from the mock library.
import { VIDEOS } from "./mock/data";
import { mockClient } from "./mock/client";

const API = "/api";

/** Minimal stand-in for backend/server: the two endpoints the shell calls directly. */
function installFetchStub(): void {
  const realFetch = window.fetch.bind(window);
  window.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    const path = url.startsWith("http") ? new URL(url).pathname : url.split("?")[0];

    const opened = path.match(/^\/api\/videos\/(\d+)\/opened$/);
    if (opened && (init?.method ?? "GET").toUpperCase() === "POST") {
      return new Response(null, { status: 204 });
    }
    const one = path.match(/^\/api\/videos\/(\d+)$/);
    if (one) {
      const video = VIDEOS.find((item) => item.id === Number(one[1]));
      return video
        ? Response.json(video)
        : Response.json({ error: "没有这条收藏" }, { status: 404 });
    }
    if (path === `${API}/session`) {
      return Response.json({ key: "design-preview" });
    }
    if (path.startsWith(API)) {
      console.warn(`[design-preview] 未模拟的接口：${init?.method ?? "GET"} ${path}`);
      return Response.json({ error: "design preview: not mocked" }, { status: 404 });
    }
    return realFetch(input, init);
  };
}

installFetchStub();

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "../mobile/mobile.css";
import App from "../mobile/App";
import ErrorBoundary from "../ui/error-boundary";
import { AppTheme } from "../ui/theme";
import { configureClient } from "../services/client";

configureClient(mockClient);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <AppTheme>
      <ErrorBoundary scope="Muse · 移动端设计预览" variant="page">
        <App />
      </ErrorBoundary>
    </AppTheme>
  </StrictMode>,
);
