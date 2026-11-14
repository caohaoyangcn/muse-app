// Service worker: the only place that talks to Muse over the network.
//
// The content script runs inside javdb.com, so a fetch() from there would be a
// cross-origin request subject to the page's CSP. The worker runs in the
// extension's own context and uses the host permission instead.
import { CHECK_TIMEOUT_MS, hasOriginPermission, loadMuseBaseURL } from "./config.js";

/**
 * Asks Muse whether the given 番号 (exactly as rendered on the page) are
 * collected.
 *
 * Resolves — never rejects — with one of:
 *   { ok: true, codes: [{ code, matches: [...] }] }
 *   { ok: false, reason: "noPermission" | "offline" | "timeout" | "http", status?, message?, base? }
 */
async function checkCodes(codes) {
  const wanted = Array.isArray(codes) ? codes.filter((c) => typeof c === "string" && c) : [];
  if (wanted.length === 0) return { ok: true, codes: [] };

  const base = await loadMuseBaseURL();
  // A custom port is not covered by manifest.json's host permissions until the
  // user grants it on the options page; say so instead of reporting "offline".
  if (!(await hasOriginPermission(base))) {
    return { ok: false, reason: "noPermission", base, message: "扩展没有访问该地址的权限" };
  }
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), CHECK_TIMEOUT_MS);
  try {
    const res = await fetch(`${base}/api/extension/check`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ codes: wanted }),
      signal: controller.signal,
      cache: "no-store",
    });
    if (!res.ok) {
      let message = `HTTP ${res.status}`;
      try {
        const body = await res.json();
        if (body && typeof body.error === "string") message = body.error;
      } catch {
        // A non-JSON error body keeps the status-based message.
      }
      return { ok: false, reason: "http", status: res.status, message, base };
    }
    const body = await res.json();
    return { ok: true, codes: Array.isArray(body?.codes) ? body.codes : [] };
  } catch (error) {
    if (error && error.name === "AbortError") {
      return { ok: false, reason: "timeout", message: "Muse 查询超时", base };
    }
    // A refused connection is the normal case when Muse is closed.
    return { ok: false, reason: "offline", message: "无法连接 Muse", base };
  } finally {
    clearTimeout(timer);
  }
}

/**
 * Opens a Muse detail page in a new tab through the extension's own jump page.
 * The jump page exists because tabs.create() would otherwise be given an
 * http://127.0.0.1 URL that no host permission covers once the port differs
 * from the declared one; the extension page itself is always reachable.
 */
async function openVideo(videoId) {
  const id = Number(videoId);
  if (!Number.isInteger(id) || id <= 0) return { ok: false };
  const base = await loadMuseBaseURL();
  const target = `${base}/videos/${id}`;
  await chrome.tabs.create({ url: chrome.runtime.getURL(`open.html#${encodeURIComponent(target)}`) });
  return { ok: true };
}

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (!message || typeof message.type !== "string") return false;
  if (message.type === "check") {
    checkCodes(message.codes).then(sendResponse);
    return true; // keep the channel open for the async answer
  }
  if (message.type === "openVideo") {
    openVideo(message.videoId).then(sendResponse);
    return true;
  }
  if (message.type === "testConnection") {
    // The options page uses this to prove the whole path end to end.
    checkCodes(["ABP-576"]).then((result) => {
      sendResponse(result);
    });
    return true;
  }
  if (message.type === "permission") {
    // Reported to the content script so a missing grant is explained rather
    // than shown as "Muse 未连接".
    (async () => {
      const base = await loadMuseBaseURL();
      sendResponse({ base, granted: await hasOriginPermission(base) });
    })();
    return true;
  }
  return false;
});

// Clicking the toolbar icon opens the options page: there is nothing to toggle.
chrome.action.onClicked.addListener(() => {
  chrome.runtime.openOptionsPage();
});
