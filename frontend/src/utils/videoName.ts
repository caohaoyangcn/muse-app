import type { Video } from "../types";

/**
 * The name a favorite is shown under.
 *
 * A user-authored title always wins. Otherwise the authoritative 番号 a
 * successful scrape recorded names the work — it is the identity the user cares
 * about, while the file name only says how this copy happens to be stored.
 * Favorites without a scrape result keep their file name, so nothing becomes
 * nameless just because scraping has not reached it yet.
 */
export function videoDisplayName(v: Video): string {
  const title = v.title?.trim();
  if (title) return title;
  const code = v.javCode?.trim();
  if (code) return code;
  return v.fileName;
}
