// URL routes for the LAN web client.
//
// The phone UI is a stack of screens, but a browser only offers forward/back
// through its own history. Putting the current screen in the URL lets the
// browser's back and forward buttons drive the same navigation as the header
// arrow, and makes every screen deep-linkable (refresh keeps the screen).
//
// Routes are deliberately flat. The parent of a pushed screen is whatever entry
// the browser recorded when it was opened, so the history list already holds
// the nesting: 收藏 → 详情 → 演员 → 详情 is four entries, and back unwinds them
// one by one. A fresh deep link has no parent entry, so the shell falls back to
// the screen's natural root (see App.tsx).
//
// Actor names travel in a query value, not a path segment: they are arbitrary
// strings (CJK, punctuation, even "/") and a "/" would be split by the browser
// before this module could decode the segment.

export type Tab = "library" | "actors" | "scrape" | "duplicates" | "trash";

export type Route =
  | { kind: "tab"; tab: Tab }
  | { kind: "detail"; videoId: number }
  | { kind: "actor"; name: string };

const TAB_PATHS: Record<Tab, string> = {
  library: "/",
  actors: "/actors",
  scrape: "/scrape",
  duplicates: "/duplicates",
  trash: "/trash",
};

/**
 * Reads a location into a route. Unknown or malformed paths fall back to the
 * library root; the shell normalises the address bar to the canonical URL.
 */
export function parseRoute(pathname: string, search = ""): Route {
  const segments = pathname.split("/").filter(Boolean);

  if (segments.length === 0) return { kind: "tab", tab: "library" };

  if (segments.length === 1) {
    switch (segments[0]) {
      case "library":
        return { kind: "tab", tab: "library" };
      case "actors":
        return { kind: "tab", tab: "actors" };
      case "scrape":
        return { kind: "tab", tab: "scrape" };
      case "duplicates":
        return { kind: "tab", tab: "duplicates" };
      case "trash":
        return { kind: "tab", tab: "trash" };
      case "actor": {
        const name = (new URLSearchParams(search).get("name") ?? "").trim();
        return name ? { kind: "actor", name } : { kind: "tab", tab: "actors" };
      }
      default:
        return { kind: "tab", tab: "library" };
    }
  }

  if (segments.length === 2 && segments[0] === "videos") {
    const id = Number(segments[1]);
    if (Number.isSafeInteger(id) && id > 0) {
      return { kind: "detail", videoId: id };
    }
  }

  return { kind: "tab", tab: "library" };
}

/** The canonical address-bar URL for a route. */
export function routeHref(route: Route): string {
  switch (route.kind) {
    case "tab":
      return TAB_PATHS[route.tab];
    case "detail":
      return `/videos/${route.videoId}`;
    case "actor":
      return `/actor?name=${encodeURIComponent(route.name)}`;
  }
}
