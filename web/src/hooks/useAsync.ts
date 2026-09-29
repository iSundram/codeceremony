import { useCallback, useEffect, useRef, useState, type DependencyList } from "react";

import { describe } from "../lib/errors";

/**
 * The fetch lifecycle, defined once.
 *
 * Twelve copies of `let live = true; … return () => { live = false }` were
 * spread across the applications, each with its own error handling, its own
 * reset-on-deps behaviour and — because none of them shared a shape — its own
 * opinion about whether a failure deserved a retry control. That is why one
 * surface could offer "Try again" and the next, failing identically, could not.
 *
 * Three properties are deliberate:
 *
 * 1. The data is retained across a dependency change instead of being cleared
 *    to null. A keystroke in a search field must not replace a table with a
 *    skeleton; the previous rows stay on screen, `refreshing` says they are
 *    being replaced, and the new ones arrive in place.
 * 2. `initial` separates "nothing has settled yet" from "settled empty", which
 *    is the difference between a skeleton and the correct empty state — the
 *    distinction that produced a skeleton rendered beside an empty state that
 *    contradicted it.
 * 3. The runner is held in a ref, so a caller may pass an inline closure
 *    without the effect re-firing on every render of that closure.
 */
export interface AsyncResource<T> {
  /** The most recent successful value, retained while a newer attempt runs. */
  data: T | null;
  /** The words for the most recent failure, cleared when a new attempt starts. */
  error: string | null;
  /** An attempt is in flight, including a refresh that still has data to show. */
  loading: boolean;
  /** Nothing has settled yet: the first load, where a skeleton belongs. */
  initial: boolean;
  /** Run again, clearing the error. The idempotent answer to "Try again". */
  reload: () => void;
}

export interface UseAsyncOptions {
  /** When false the runner is not called and the resource stays untouched. */
  enabled?: boolean;
}

export function useAsync<T>(
  run: () => Promise<T>,
  deps: DependencyList,
  options: UseAsyncOptions = {},
): AsyncResource<T> {
  const { enabled = true } = options;
  const [attempt, setAttempt] = useState(0);
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [settled, setSettled] = useState(false);
  const runRef = useRef(run);
  runRef.current = run;

  const reload = useCallback(() => {
    setError(null);
    setAttempt((value) => value + 1);
  }, []);

  useEffect(() => {
    if (!enabled) return;
    let live = true;
    setLoading(true);
    setError(null);
    runRef
      .current()
      .then((value) => {
        if (!live) return;
        setData(value);
        setSettled(true);
        setLoading(false);
      })
      .catch((caught: unknown) => {
        if (!live) return;
        setError(describe(caught));
        setSettled(true);
        setLoading(false);
      });
    return () => {
      live = false;
    };
    // The runner is read through a ref on purpose: the caller's deps are the
    // inputs that matter, and including the closure would refire on renders
    // where nothing it reads has changed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, attempt, ...deps]);

  return { data, error, loading, initial: !settled && loading, reload };
}
