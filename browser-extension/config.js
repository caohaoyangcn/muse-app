// Settings shared by the content script, the service worker and the options
// page. Everything the extension knows about Muse lives here.
//
// The address is a host + port pair the user can edit: Muse lets the LAN server
// move to any port, and guessing which one is running would be guesswork. The
// manifest grants the default port up front; other loopback ports are requested
// from the options page with one click (Chrome only grants what a user gesture
// asks for), after which the choice is remembered.
export const DEFAULT_HOST = "127.0.0.1";
export const DEFAULT_PORT = 8899;
export const DEFAULT_MUSE_BASE = `http://${DEFAULT_HOST}:${DEFAULT_PORT}`;

// The loopback hosts the extension will talk to. A remote host is refused: the
// LAN server is unauthenticated, so pointing the extension at another machine
// would hand the library to that machine's network too.
const LOOPBACK_HOSTS = new Set(["127.0.0.1", "localhost", "::1", "[::1]"]);

// checkTimeoutMs bounds one query. Muse answers from its own database, so a
// slow answer means the app is not running; waiting longer would only delay the
// "Muse 未运行" badge.
export const CHECK_TIMEOUT_MS = 3000;

/** Whether a host is one the extension is allowed to reach. */
export function isLoopbackHost(host) {
  return LOOPBACK_HOSTS.has(String(host ?? "").trim().toLowerCase());
}

/** Validates a port number typed by the user; returns 0 when invalid. */
export function parsePort(raw) {
  const value = Number.parseInt(String(raw ?? "").trim(), 10);
  if (!Number.isInteger(value) || value < 1 || value > 65535) return 0;
  return value;
}

/**
 * Builds the base URL from a host and port. Anything unexpected falls back to
 * the default so a stored value can never become a javascript: or file: URL.
 */
export function buildBaseURL(host, port) {
  const normalizedHost = isLoopbackHost(host) ? String(host).trim() : DEFAULT_HOST;
  const normalizedPort = parsePort(port) || DEFAULT_PORT;
  const bracketed = normalizedHost === "::1" ? "[::1]" : normalizedHost;
  return `http://${bracketed}:${normalizedPort}`;
}

/** Splits a stored base URL (or the default) back into its parts. */
export function parseBaseURL(raw) {
  const value = String(raw ?? "").trim();
  try {
    const parsed = new URL(value || DEFAULT_MUSE_BASE);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return { host: DEFAULT_HOST, port: DEFAULT_PORT };
    }
    return {
      host: isLoopbackHost(parsed.hostname) ? parsed.hostname : DEFAULT_HOST,
      port: parsePort(parsed.port) || DEFAULT_PORT,
    };
  } catch {
    return { host: DEFAULT_HOST, port: DEFAULT_PORT };
  }
}

/** The match-pattern a host permission needs for this base URL. */
export function originPattern(base) {
  const { host, port } = parseBaseURL(base);
  return `${buildBaseURL(host, port)}/*`;
}

/** The configured Muse address, or the default when nothing is stored yet. */
export async function loadMuseBaseURL() {
  try {
    const stored = await chrome.storage.local.get("museBaseUrl");
    const { host, port } = parseBaseURL(stored?.museBaseUrl);
    return buildBaseURL(host, port);
  } catch {
    return DEFAULT_MUSE_BASE;
  }
}

/** Persists a host/port pair and returns the address now in effect. */
export async function saveMuseAddress(host, port) {
  const base = buildBaseURL(host, port);
  await chrome.storage.local.set({ museBaseUrl: base });
  return base;
}

/**
 * Whether the extension may currently call base. Must be called in the worker
 * (a content script has no `permissions` object).
 */
export async function hasOriginPermission(base) {
  try {
    return await chrome.permissions.contains({ origins: [originPattern(base)] });
  } catch {
    return false;
  }
}

/**
 * Asks for access to base. Only works from a user gesture (a click on the
 * options page); returns whether the grant now exists.
 */
export async function requestOriginPermission(base) {
  try {
    return await chrome.permissions.request({ origins: [originPattern(base)] });
  } catch {
    return false;
  }
}
