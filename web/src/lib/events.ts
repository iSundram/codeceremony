import { useEffect, useState } from "react";

import { api, type EventSummary } from "./api";
import { describe } from "./errors";

/**
 * The event list, fetched once for the whole document.
 *
 * The judge resolved the current event from a route param that no route
 * declared, the organizer resolved it from a query param with a hook of its
 * own, and the gallery resolved it a third way — so a single route change
 * fired as many as six identical `GET /v1/events`, each with its own loading,
 * error and empty states. Two of the three hooks had a defect the other
 * avoided: one had no "there are no events at all" state and sat on a skeleton
 * forever, the other could never select anything but the first event.
 *
 * One shared list fixes the duplicate requests and gives every surface the
 * same three answers: the events, the failure if they could not be read, and
 * the explicit "this portal has none" case that is neither loading nor an
 * error.
 */
interface EventsStore {
  events: EventSummary[] | null;
  error: string | null;
  version: number;
}

let store: EventsStore = { events: null, error: null, version: 0 };
let inFlight: Promise<void> | null = null;
const listeners = new Set<() => void>();

function emit(): void {
  for (const listener of listeners) listener();
}

/**
 * Load the list, or reuse the request already in flight.
 *
 * `force` is for the retry control: it must bypass the in-flight reuse, or a
 * failure would latch and "Try again" would return the same rejected promise
 * that produced the error in the first place.
 */
export function loadEvents(force = false): Promise<void> {
  if (inFlight && !force) return inFlight;
  inFlight = api
    .events()
    .then((response) => {
      store = { events: response.data, error: null, version: store.version + 1 };
    })
    .catch((caught: unknown) => {
      store = { events: store.events, error: describe(caught), version: store.version + 1 };
    })
    .finally(() => {
      inFlight = null;
      emit();
    });
  return inFlight;
}

export interface EventsList {
  events: EventSummary[];
  /** Null until the first response settles — the state a skeleton belongs in. */
  pending: boolean;
  error: string | null;
  /** True when the portal answered with an empty list, which is not an error. */
  empty: boolean;
  reload: () => void;
}

export function useEvents(): EventsList {
  const [, setVersion] = useState(store.version);

  useEffect(() => {
    const listener = () => setVersion(store.version);
    listeners.add(listener);
    void loadEvents();
    return () => {
      listeners.delete(listener);
    };
  }, []);

  const reload = () => {
    store = { ...store, error: null };
    void loadEvents(true);
  };

  const events = store.events ?? [];
  return {
    events,
    pending: store.events === null && store.error === null,
    error: store.error,
    empty: store.events !== null && store.events.length === 0,
    reload,
  };
}
