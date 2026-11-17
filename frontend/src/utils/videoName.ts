import type { Video } from "../types";

/**
 * The name a favorite is shown under, on both desktop and mobile.
 *
 * A user-authored title always wins. A successfully scraped favorite is shown
 * as 【番号】译名: the Chinese translation is the readable half, and the
 * bracketed 番号 keeps the work's identity visible even when the title is
 * truncated. Until a translation exists the scraped original title stands in,
 * so a card never loses its name while the translation queue catches up.
 *
 * A favorite with no scraped title keeps its file name — the 【番号】 format is
 * a scrape result, not a file-name decoration, so an unscraped or failed
 * favorite still reads as the file it actually is.
 */
export function videoDisplayName(v: Video): string {
  const title = v.title?.trim();
  if (title) return title;

  const code = v.javCode?.trim();
  if (!code) return v.fileName;

  const translated = v.javTitleZh?.trim();
  const original = v.javTitle?.trim();
  // A missing title is the signal that the work never scraped successfully (a
  // manually entered code alone stores no title), so the card stays on the
  // file name instead of claiming a work identity it does not have.
  if (!translated && !original) return v.fileName;

  // Some providers publish nothing but the 番号 as the title; showing 【番号】
  // once is still correct, so only the echoed copy is stripped.
  const body = stripLeadingCode(code, translated || original);
  return body ? `【${code}】${body}` : `【${code}】`;
}

/**
 * Drops a 番号 echoed at the start of a title: the display already shows it in
 * 【】, so keeping it would render 【IPX-633】IPX-633 标题.
 */
function stripLeadingCode(code: string, title: string): string {
  const trimmed = title.trim();
  if (!code || !trimmed) return trimmed;
  const escaped = code.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  // Bracketed form (and a possible separator after it), e.g. 【IPX-633】标题.
  const bracketed = new RegExp(`^[【\\[]?\\s*${escaped}\\s*[】\\]]\\s*[-—:：·|]?\\s*`, "i");
  // Bare form, guarded by a boundary so AB-1 never eats the start of AB-123.
  const bare = new RegExp(`^${escaped}(?:\\s*[-—:：·|]\\s*|\\s+|$)`, "i");
  return trimmed.replace(bracketed, "").replace(bare, "").trim();
}
