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

// ---------- match patterns are valid for Chrome ----------
// Chrome accepts a wildcard only as the whole leftmost label ("*.example.com");
// an embedded one ("*.javdb*.com") makes the whole manifest invalid, which the
// browser reports as "Invalid host wildcard". Validate every pattern here so a
// load failure is never the first feedback.
// Host permissions may carry an explicit port, content-script matches may not
// (and adding one would be a mistake: it would stop matching after a port change).
const MATCH_PATTERN = /^(\*|https?):\/\/(\*|(\*\.)?[a-z0-9-]+(\.[a-z0-9-]+)*)(:([0-9]+|\*))?\/?(\/.*)?$/i;
const SHARED_SPECIAL = ["*", "*/*", "<all_urls>"];
const contentMatches = (manifest.content_scripts ?? []).flatMap((cs) => cs.matches ?? []);
for (const pattern of contentMatches) {
  if (SHARED_SPECIAL.includes(pattern)) continue;
  if (!MATCH_PATTERN.test(pattern)) {
    fail(`manifest.json: invalid match pattern ${pattern} (a wildcard may only be the whole leftmost host label, e.g. *.jav.app)`);
    continue;
  }
  const host = pattern.replace(/^\*:\/\//, "").replace(/\/.*$/, "");
  if (host.includes("*") && !host.startsWith("*.")) {
    fail(`manifest.json: invalid host wildcard in ${pattern} — use *.domain.tld, not *.domain*.tld`);
  }
  if (host.startsWith("*.") && host.slice(2).includes("*")) {
    fail(`manifest.json: invalid host wildcard in ${pattern} — only one wildcard label is allowed`);
  }
}

const hostPermissions = manifest.host_permissions ?? [];
for (const required of ["http://127.0.0.1:8899/*", "http://localhost:8899/*"]) {
  if (!hostPermissions.includes(required)) {
    fail(`manifest.json: host_permissions must include ${required}`);
  }
}
for (const pattern of hostPermissions) {
  if (!SHARED_SPECIAL.includes(pattern) && !MATCH_PATTERN.test(pattern)) {
    fail(`manifest.json: invalid host_permissions pattern ${pattern}`);
  }
}

// Content-script matches must not pin a port: the page is whatever port the
// site serves, and a pinned one would silently stop matching.
for (const pattern of contentMatches) {
  const port = pattern.replace(/^\*:\/\//, "").replace(/\/.*$/, "").split(":")[1];
  if (port && port !== "*") {
    fail(`manifest.json: content_scripts matches ${pattern} pins a port; use *://host/* instead`);
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
// The content script only runs on the hosts the manifest lists; the server only
// answers the origins extension.go allows. A host that runs the script but is
// missing from the allowlist renders badges that can never load.
const serverGo = readFileSync(path.join(root, "backend", "server", "extension.go"), "utf8");
const allowlistBody = serverGo.slice(
  serverGo.indexOf("func extensionOriginAllowed"),
  serverGo.indexOf("func isDigits"),
);
for (const pattern of contentMatches) {
  const host = pattern.replace(/^\*:\/\//, "").replace(/\/.*$/, "").toLowerCase();
  const covered = (() => {
    if (host === "javdb.com") return serverGo.includes('host == "javdb.com"');
    if (host === "www.javdb.com") return serverGo.includes('host == "www.javdb.com"');
    if (host === "jav.app") return serverGo.includes('host == "jav.app"');
    // "*.jav.app" is covered by the subdomain suffix rule.
    if (host === "*.jav.app") return allowlistBody.includes('strings.HasSuffix(host, ".jav.app")');
    return false;
  })();
  if (!covered) {
    fail(
      `origin list drift: the manifest runs on ${host}, but backend/server/extension.go does not allow that origin — add it to extensionOriginAllowed and the manifest together`,
    );
  }
}

// The numbered mirrors (javdb581.com, app.javdb582.com) cannot be expressed as a
// match pattern — "*://javdb*.com/*" is not a valid host wildcard, which is why
// the manifest does not list them. That means the server's numbered-mirror rule
// is the only thing keeping a mirror page from being blocked once its domain is
// added to the manifest; keep both halves of it present.
const mirrorRule = allowlistBody.indexOf('strings.CutSuffix(host, ".com")');
const digitsGuard = allowlistBody.indexOf("isDigits(label)");
if (mirrorRule === -1 || digitsGuard === -1) {
  fail("extension.go: the numbered-mirror origin rule is missing — mirrors would be refused even after the manifest lists them");
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
