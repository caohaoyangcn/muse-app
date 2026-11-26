import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

/*
 * Design preview server. Started by `npm run dev:design`, never by `build`.
 *
 * Two jobs:
 *  1. Serve `src/design/preview.html`, which boots the real app against the
 *     mock transport in `src/design/mock/`.
 *  2. Answer the artwork URLs the app asks for (`/thumbnails/12.jpg`,
 *     `/covers/12.jpg`, `/avatars/名字.jpg`) with deterministic generated
 *     posters, so the grid has real <img> content without shipping any
 *     placeholder file into the Wails bundle or the LAN web build.
 */

const POSTER_WIDTH = 1280;
const POSTER_HEIGHT = 720;

function hash(value: string): number {
  let h = 2166136261;
  for (let i = 0; i < value.length; i += 1) {
    h ^= value.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return Math.abs(h);
}

function escapeXml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/** A poster-shaped placeholder: two-stop gradient, code, studio line. */
function posterSvg(kind: string, name: string): string {
  const seed = hash(`${kind}/${name}`);
  const hue = seed % 360;
  const hueB = (hue + 42 + (seed % 30)) % 360;
  // The app asks for `/thumbnails/<id>.jpg`; the poster only has to look like
  // cover art, so the label stays generic instead of pretending to be a code.
  const scene = name.padStart(2, "0");
  const studio = ["MUSE WORKS", "AURA FILMS", "NOCT STUDIO", "LUMI PICTURES"][seed % 4];
  const title = escapeXml(`SCENE ${scene}`);
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${POSTER_WIDTH}" height="${POSTER_HEIGHT}" viewBox="0 0 ${POSTER_WIDTH} ${POSTER_HEIGHT}">
  <defs>
    <linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="hsl(${hue} 34% 22%)"/>
      <stop offset="0.55" stop-color="hsl(${hueB} 30% 14%)"/>
      <stop offset="1" stop-color="hsl(${hue} 24% 9%)"/>
    </linearGradient>
    <radialGradient id="v" cx="0.72" cy="0.28" r="0.85">
      <stop offset="0" stop-color="hsl(${hueB} 60% 62%)" stop-opacity="0.34"/>
      <stop offset="1" stop-color="hsl(${hueB} 60% 62%)" stop-opacity="0"/>
    </radialGradient>
  </defs>
  <rect width="${POSTER_WIDTH}" height="${POSTER_HEIGHT}" fill="url(#g)"/>
  <rect width="${POSTER_WIDTH}" height="${POSTER_HEIGHT}" fill="url(#v)"/>
  <g opacity="0.18" stroke="#ffffff" stroke-width="2">
    <line x1="80" y1="0" x2="80" y2="${POSTER_HEIGHT}"/>
    <line x1="${POSTER_WIDTH - 80}" y1="0" x2="${POSTER_WIDTH - 80}" y2="${POSTER_HEIGHT}"/>
    <line x1="0" y1="${POSTER_HEIGHT - 96}" x2="${POSTER_WIDTH}" y2="${POSTER_HEIGHT - 96}"/>
  </g>
  <circle cx="${POSTER_WIDTH * 0.74}" cy="${POSTER_HEIGHT * 0.4}" r="150" fill="hsl(${hueB} 45% 70%)" opacity="0.12"/>
  <text x="80" y="${POSTER_HEIGHT - 150}" font-family="Segoe UI, Helvetica, Arial, sans-serif" font-size="96" font-weight="700" letter-spacing="6" fill="#f4f4f7" opacity="0.94">${title}</text>
  <text x="80" y="${POSTER_HEIGHT - 96}" font-family="Segoe UI, Helvetica, Arial, sans-serif" font-size="26" letter-spacing="10" fill="#ffffff" opacity="0.55">${studio}</text>
</svg>`;
}

/** A round-ish avatar placeholder: initials over a soft gradient. */
function avatarSvg(name: string): string {
  const seed = hash(`avatar/${name}`);
  const hue = seed % 360;
  const initial = escapeXml(Array.from(name)[0] ?? "?");
  return `<svg xmlns="http://www.w3.org/2000/svg" width="400" height="500" viewBox="0 0 400 500">
  <defs>
    <linearGradient id="a" x1="0" y1="0" x2="0.6" y2="1">
      <stop offset="0" stop-color="hsl(${hue} 38% 30%)"/>
      <stop offset="1" stop-color="hsl(${(hue + 30) % 360} 30% 14%)"/>
    </linearGradient>
  </defs>
  <rect width="400" height="500" fill="url(#a)"/>
  <text x="50%" y="56%" text-anchor="middle" font-family="Segoe UI, Helvetica, Arial, sans-serif" font-size="150" font-weight="600" fill="#ffffff" opacity="0.82">${initial}</text>
</svg>`;
}

function mockArtwork(): Plugin {
  return {
    name: "muse-design-mock-artwork",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const url = req.url ?? "";
        const match = url.match(
          /^\/(thumbnails|covers|avatars)\/([^/?]+)\.(?:jpg|jpeg|png|webp)(?:\?.*)?$/i,
        );
        if (!match) {
          next();
          return;
        }
        const [, kind, rawName] = match;
        const name = decodeURIComponent(rawName);
        const svg = kind === "avatars" ? avatarSvg(name) : posterSvg(kind, name);
        res.setHeader("Content-Type", "image/svg+xml; charset=utf-8");
        res.setHeader("Cache-Control", "no-store");
        res.end(svg);
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss(), mockArtwork()],
  publicDir: false,
  server: {
    port: 5199,
    strictPort: true,
  },
});
