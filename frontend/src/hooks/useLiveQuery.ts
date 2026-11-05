import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { liveRevision, subscribeLive } from "../services/live";

/**
 * How often a burst of invalidations is folded into one re-read. A scrape emits
 * one event per video; without this, an open view would re-read per video.
 */
const THROTTLE_MS = 200;

export type LiveQuery<T> = {
  data: T | null;
  /** True only until the first answer for the current key arrives. */
  loading: boolean;
  /** Set only when there is nothing to show; a failed background read keeps the
   *  list the user is reading. */
  error: string | null;
  /** Re-reads immediately (a 重试 button, or a write the backend does not
   *  announce with an event). */
  reload: () => void;
  /** Replaces the data without a read, for an optimistic write. */
  mutate: (next: T | null) => void;
};

type Options = {
  /** The view is not on screen and does not need its data right now. */
  enabled?: boolean;
  /** Keep the previous answer on screen while a *different* query loads. */
  keepPreviousData?: boolean;
  /** Every failure, background ones included, for a toast. */
  onError?: (error: unknown) => void;
};

function describeError(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

/**
 * The one way a view reads data that the library can change.
 *
 * It re-reads when the query key changes (a different actor, group, keyword or
 * video), and — quietly, in the background — whenever anything in the app is
 * invalidated. A view therefore stays correct without knowing which events or
 * writes exist, which is the whole point: freshness is a property of the data
 * layer, not something each new view has to remember.
 *
 * Background reads never blank the screen and never surface an error over data
 * the user can still use.
 */
export function useLiveQuery<T>(
  fetcher: () => Promise<T>,
  key: string,
  options: Options = {},
): LiveQuery<T> {
  const { enabled = true, keepPreviousData = false } = options;

  const [state, setState] = useState<{ data: T | null; loading: boolean; error: string | null }>(
    () => ({ data: null, loading: enabled, error: null }),
  );
  // Re-renders this hook (and only this hook) on every invalidation.
  const revision = useSyncExternalStore(subscribeLive, liveRevision, liveRevision);

  // The fetcher closes over the current render's values; keep the latest one so
  // a read triggered later still asks the question the key describes.
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;
  const onErrorRef = useRef(options.onError);
  onErrorRef.current = options.onError;

  const dataRef = useRef<T | null>(null);
  const sequence = useRef(0);
  const alive = useRef(true);
  const throttle = useRef<number | null>(null);

  const run = useCallback(async (mode: "initial" | "quiet") => {
    const mine = ++sequence.current;
    if (mode === "initial") {
      dataRef.current = null;
      setState({ data: null, loading: true, error: null });
    }
    try {
      const data = await fetcherRef.current();
      if (mine !== sequence.current || !alive.current) return;
      dataRef.current = data;
      setState({ data, loading: false, error: null });
    } catch (error) {
      if (mine !== sequence.current || !alive.current) return;
      onErrorRef.current?.(error);
      setState((prev) => ({
        ...prev,
        loading: false,
        // A quiet failure must not replace a list the user can still use.
        error: dataRef.current === null ? describeError(error) : prev.error,
      }));
    }
  }, []);

  const cancelThrottle = useCallback(() => {
    if (throttle.current !== null) {
      window.clearTimeout(throttle.current);
      throttle.current = null;
    }
  }, []);

  // Unmount guard, declared first so it runs before the reads below.
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      cancelThrottle();
    };
  }, [cancelThrottle]);

  // A different key is a different question: the previous rows are not its
  // answer, so they go away unless the caller asked to keep them on screen —
  // and there is nothing to keep when the previous read never landed.
  useEffect(() => {
    cancelThrottle();
    if (!enabled) return;
    void run(keepPreviousData && dataRef.current !== null ? "quiet" : "initial");
  }, [key, enabled, keepPreviousData, run, cancelThrottle]);

  // Something changed. Throttled, not debounced: a long scrape emits events
  // continuously, and a debounce would keep postponing the read until it ends.
  const seenRevision = useRef(revision);
  useEffect(() => {
    if (revision === seenRevision.current) return;
    seenRevision.current = revision;
    if (!enabled || throttle.current !== null) return;
    throttle.current = window.setTimeout(() => {
      throttle.current = null;
      void run("quiet");
    }, THROTTLE_MS);
  }, [revision, enabled, run]);

  const reload = useCallback(() => {
    cancelThrottle();
    // A retry after a failure has nothing to preserve; a refresh of a loaded
    // view stays silent.
    void run(dataRef.current === null ? "initial" : "quiet");
  }, [cancelThrottle, run]);

  const mutate = useCallback((next: T | null) => {
    dataRef.current = next;
    setState({ data: next, loading: false, error: null });
  }, []);

  return { data: state.data, loading: state.loading, error: state.error, reload, mutate };
}
