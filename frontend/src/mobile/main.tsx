import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import App from "./App";
import ErrorBoundary from "../components/ErrorBoundary";
import "./mobile.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary scope="Muse Video" variant="page">
      <App />
    </ErrorBoundary>
  </StrictMode>,
);
