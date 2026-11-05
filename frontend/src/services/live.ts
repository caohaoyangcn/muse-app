/**
 * The app's single "the data behind a view changed" signal.
 *
 * Every local write, and every backend event the desktop receives, funnels into
 * `invalidate()`. Views never subscribe to event names and never accept a
 * refresh prop: they read through `useLiveQuery`, which follows this revision.
 *
 * The direction of the failure matters. A view that declares nothing still
 * re-reads on every change, so forgetting to wire something costs an extra
 * read, never a stale screen. That is why there is no per-view interest list
 * here — a mistake in one would silently make that view stale again.
 */
type Listener = () => void;

let revision = 0;
const listeners = new Set<Listener>();

/** Announces that data this client can see (or has just written) changed. */
export function invalidate(): void {
  revision += 1;
  // Iterate a copy: a listener may unsubscribe while being notified.
  for (const listener of [...listeners]) listener();
}

/** The current revision. Monotonic, so it doubles as a store snapshot. */
export function liveRevision(): number {
  return revision;
}

/** Subscribes to invalidations and returns the unsubscribe function. */
export function subscribeLive(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
