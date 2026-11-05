// Vite names the emitted HTML after its entry file (`mobile.html`), but the Go
// static handler serves `/` from `index.html`. Rename it after the build so the
// committed webui directory needs no special-casing on the server.
//
// Failing loudly here is intentional: without it a broken build would leave a
// stale `index.html` in place and the server would serve the previous release
// while the new assets sat unused.
import { renameSync, existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const webuiDir = join(here, "..", "..", "backend", "server", "webui");
const emitted = join(webuiDir, "mobile.html");
const target = join(webuiDir, "index.html");

if (!existsSync(emitted)) {
    console.error(`finalize-mobile: expected ${emitted} to exist after the build`);
    process.exit(1);
}
renameSync(emitted, target);
console.log("finalize-mobile: wrote backend/server/webui/index.html");
