#!/usr/bin/env node
// Sanity checks for the browser extension. The extension has no build step and
// no test framework, so this script is what CI and a release can run:
//
//   - every script parses (content scripts and the worker are not type-checked)
//   - manifest.json is valid and every file it references exists
//   - the content script does not extract 番号 from page text: it must read the
//     rendered code element, and must not contain a title-parsing regex
//   - the CORS allowlist in backend/server/extension.go covers every origin the
//     manifest matches (the two lists must move together)
//
// Usage: node scripts/check-extension.mjs   (from the repo root)
import { readFileSync, existsSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const extDir = path.join(root, "browser-extension");
const problems = [];

function fail(message) {
  problems.push(message);
}

// ---------- scripts parse ----------
for (const name of ["config.js", "background.js", "content.js", "open.js", "options.js"]) {
  const file = path.join(extDir, name);
  if (!existsSync(file)) {
    fail(`${name}: missing`);
    continue;
  }
  try {
    execFileSync(process.execPath, ["--check", file], { stdio: "pipe" });
  } catch (error) {
    fail(`${name}: does not parse\n${error.stderr ?? ""}`);
  }
}

// ---------- manifest ----------
let manifest;
try {
  manifest = JSON.parse(readFileSync(path.join(extDir, "manifest.json"), "utf8"));
} catch (error) {
  fail(`manifest.json: invalid JSON (${error.message})`);
  process.exit(report());
}

if (manifest.manifest_version !== 3) fail("manifest.json: manifest_version must be 3");

const referenced = [
  manifest.background?.service_worker,
  manifest.options_ui?.page,
  ...Object.values(manifest.icons ?? {}),
  ...Object.values(manifest.action?.default_icon ?? {}),
  ...(manifest.content_scripts ?? []).flatMap((cs) => [...(cs.js ?? []), ...(cs.css ?? [])]),
].filter(Boolean);
for (const rel of referenced) {
  if (!existsSync(path.join(extDir, rel))) fail(`manifest.json: referenced file is missing: ${rel}`);
}

const hostPermissions = manifest.host_permissions ?? [];
for (const required of ["http://127.0.0.1:8899/*", "http://localhost:8899/*"]) {
  if (!hostPermissions.includes(required)) {
    fail(`manifest.json: host_permissions must include ${required}`);
  }
}

// ---------- no 番号 parsing in the content script ----------
const content = readFileSync(path.join(extDir, "content.js"), "utf8");
if (!content.includes('const CODE_SELECTOR = ".video-title strong"')) {
  fail("content.js: the 番号 must be read from the rendered code element (.video-title strong)");
}
// A code-shaped pattern anywhere in the content script means someone started
// guessing 番号 from titles again; that is the design this extension avoids.
if (/[a-z]\{2,6\}/i.test(content) || /-\[\]_ ?\?\[0-9\]/.test(content)) {
  fail("content.js: looks like a 番号 extraction regex — read the page field instead");
}

// ---------- the two origin lists agree ----------
const serverGo = readFileSync(path.join(root, "backend", "server", "extension.go"), "utf8");
const matches = (manifest.content_scripts ?? []).flatMap((cs) => cs.matches ?? []);
for (const pattern of matches) {
  const host = pattern.replace(/^\*:\/\//, "").replace(/\/\*$/, "");
  // "*://*.javdb*.com/*" → the allowlist must handle javdb.com and the numbered
  // mirrors; "*://javdb.com/*" → the permanent domain; "*://jav.app/*" → the app
  // domain. Each of the three shapes has a named counterpart in extension.go.
  const handled =
    (host === "javdb.com" && serverGo.includes('host == "javdb.com"')) ||
    (host === "jav.app" && serverGo.includes('host == "jav.app"')) ||
    (host === "*.javdb*.com" && serverGo.includes('strings.CutSuffix(host, ".com")'));
  if (!handled) {
    fail(`origin list drift: manifest matches ${pattern}, but backend/server/extension.go has no counterpart — update both`);
  }
}

process.exit(report());

function report() {
  if (problems.length === 0) {
    console.log(`extension checks passed (${referenced.length} files referenced by the manifest)`);
    return 0;
  }
  for (const problem of problems) console.error(`error: ${problem}`);
  return 1;
}
