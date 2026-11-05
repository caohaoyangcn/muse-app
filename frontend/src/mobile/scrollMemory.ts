// Scroll offsets remembered per browser-history entry.
//
// The browser restores the page scroll for normal documents, but the phone UI
// scrolls inside its own panes and swaps those panes in React. The same URL can
// also appear several times in one session (收藏 → 详情 → 返回 → 另一个详情), so
// offsets cannot be keyed by URL. Every history entry instead carries a stable
// `museKey`; this module remembers one entry's marked panes under that key while
// the shell is leaving it, and replays them when it returns.

const offsets = new Map<string, Record<string, number>>();
// Entries whose offsets are being replayed. While one is pending its panes can
// still read 0 (the rows are loading), so leaving again must not overwrite the
// position that is on its way back.
const restoring = new Set<string>();

let seq = 0;

/** A key that is unique per history entry, without crypto.randomUUID (the LAN
 * client runs on plain HTTP, where that API is unavailable on several phones). */
export function newScrollKey(): string {
  seq += 1;
  return `${Date.now().toString(36)}-${seq}-${Math.random().toString(36).slice(2, 8)}`;
}

/** Reads every marked pane on the current screen. */
function readOffsets(): Record<string, number> {
  const result: Record<string, number> = {};
  document.querySelectorAll<HTMLElement>("[data-scroll-key]").forEach((el) => {
    const key = el.dataset.scrollKey;
    if (key) result[key] = Math.round(el.scrollTop);
  });
  return result;
}

/** Saves the current screen's offsets under one history-entry key. */
export function rememberScroll(key: string | undefined) {
  if (!key || restoring.has(key)) return;
  offsets.set(key, readOffsets());
}

export function recallScroll(key: string | undefined): Record<string, number> | undefined {
  return key ? offsets.get(key) : undefined;
}

/**
 * Re-applies one entry's saved offsets to the mounted screen. Returns a cleanup
 * for the effect that started it.
 *
 * Content is fetched per screen, so the pane is often still empty on the first
 * frame. Keep trying for a short window until the pane is tall enough to hold
 * the offset (a virtualised grid only gains its full height once the rows are
 * measured). The timeout also covers an entry whose screen genuinely shrank —
 * the pane stays at its new maximum instead of waiting forever.
 */
export function restoreScroll(key: string | undefined): () => void {
  const saved = recallScroll(key);
  if (!key || !saved || Object.keys(saved).length === 0) return () => undefined;

  restoring.add(key);

  let cancelled = false;
  let frame = 0;
  const deadline = performance.now() + 4000;

  const finish = () => {
    restoring.delete(key);
  };

  const attempt = () => {
    if (cancelled) return;

    const panes = new Map<string, HTMLElement>();
    document.querySelectorAll<HTMLElement>("[data-scroll-key]").forEach((el) => {
      const pane = el.dataset.scrollKey;
      if (pane) panes.set(pane, el);
    });

    let pending = false;
    for (const [pane, top] of Object.entries(saved)) {
      const el = panes.get(pane);
      if (!el) {
        pending = true;
        continue;
      }
      if (top > 0 && el.scrollHeight - el.clientHeight < top) {
        pending = true;
        continue;
      }
      if (Math.abs(el.scrollTop - top) > 1) el.scrollTop = top;
      if (Math.abs(el.scrollTop - top) > 1) pending = true;
    }

    if (!pending) {
      finish();
      return;
    }
    if (performance.now() < deadline) {
      frame = requestAnimationFrame(attempt);
    } else {
      finish();
    }
  };

  frame = requestAnimationFrame(attempt);

  return () => {
    cancelled = true;
    cancelAnimationFrame(frame);
    // Deliberately keep `restoring`: the entry left before its replay finished.
    // Its stored position is still the best answer, and must survive a quick
    // second traversal back into it.
  };
}
