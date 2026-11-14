// Settings shared by the content script, the service worker and the options
// page. Everything the extension knows about Muse lives here: the port is
// fixed because manifest.json may only declare host permissions for concrete
// ports, and a runtime-requested permission would be a needless prompt on every
// install.
export const DEFAULT_MUSE_BASE = "http://127.0.0.1:8899";

// checkTimeoutMs bounds one query. Muse answers from its own database, so a
// slow answer means the app is not running; waiting longer would only delay the
// "Muse 未运行" badge.
export const CHECK_TIMEOUT_MS = 3000;

/**
 * The Muse base URL as configured, without a trailing slash. Only http(s) is
 * accepted: the value ends up in a fetch() and in a tab URL, so a javascript:
 * or file: scheme must never leave this module.
 */
export function normalizeBaseURL(raw) {
  const value = String(raw ?? "").trim().replace(/\/+$/, "");
  if (!value) return DEFAULT_MUSE_BASE;
  try {
    const parsed = new URL(value);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return DEFAULT_MUSE_BASE;
    }
    return parsed.origin;
  } catch {
    return DEFAULT_MUSE_BASE;
  }
}

/** The configured base URL, or the default when nothing is stored yet. */
export async function loadMuseBaseURL() {
  try {
    const stored = await chrome.storage.local.get("museBaseUrl");
    return normalizeBaseURL(stored?.museBaseUrl);
  } catch {
    return DEFAULT_MUSE_BASE;
  }
}

/** Persists a base URL and returns the value that is now in effect. */
export async function saveMuseBaseURL(raw) {
  const base = normalizeBaseURL(raw);
  await chrome.storage.local.set({ museBaseUrl: base });
  return base;
}
