import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Mobile (LAN web) build. It is a second Vite build over the same frontend
// package and reuses the shared utilities/types, but it is a separate entry
// from the desktop Wails app: the desktop talks to Go through Wails bindings,
// the mobile client talks HTTP. The output is written straight into the
// embedded asset directory (backend/server/webui), which `go:embed webui`
// picks up at compile time.
//
// Filenames are deliberately unhashed (`assets/app.js`, `assets/app.css`):
// the Go static handler already serves everything with `Cache-Control:
// no-cache`, so content hashing buys nothing and only adds build-artifact
// churn to the committed webui directory.
export default defineConfig({
    plugins: [react(), tailwindcss()],
    publicDir: "mobile-public",
    build: {
        outDir: "../backend/server/webui",
        emptyOutDir: true,
        rollupOptions: {
            input: "mobile.html",
            output: {
                entryFileNames: "assets/app.js",
                chunkFileNames: "assets/[name].js",
                assetFileNames: (assetInfo) => {
                    const name = assetInfo.names?.[0] ?? assetInfo.name ?? "";
                    if (name.endsWith(".css")) return "assets/app.css";
                    return "assets/[name][extname]";
                },
            },
        },
    },
});
