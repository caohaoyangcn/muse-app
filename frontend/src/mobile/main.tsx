import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import App from "./App";
import ErrorBoundary from "../ui/error-boundary";
import { configureClient } from "../services/client";
import { httpClient } from "../services/client.http";
import "./mobile.css";

// Bind the LAN transport before anything renders. Views read through
// services/client.ts, so the same view code runs on both clients.
configureClient(httpClient);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary scope="Muse Video" variant="page">
      <App />
    </ErrorBoundary>
  </StrictMode>,
);
