import { ApiError } from "./api";

/**
 * One place that turns a caught value into words a person can act on.
 *
 * Every application used to carry its own copy of this function — five
 * definitions and two inline variants — and they disagreed about the
 * connection case, so the same failure read differently depending on which
 * page the viewer happened to be on. `ApiError` already knows the difference
 * between "not permitted", "gone", "someone else changed this" and "the
 * server never answered"; the words are the last step of that classification,
 * not a thing each page re-derives.
 */
export function describe(caught: unknown): string {
  if (caught instanceof ApiError) {
    if (caught.isForbidden) {
      return `${caught.message} Your session may have ended: sign in again, or ask an organizer for access.`;
    }
    if (caught.isConflict) {
      return `${caught.message} Reload the page so you are editing the current version, then try again.`;
    }
    if (caught.isNotFound) {
      return `${caught.message} It may have been removed since the page was loaded.`;
    }
    return caught.message;
  }
  if (caught instanceof DOMException && caught.name === "AbortError") {
    return "The request was cancelled before it finished.";
  }
  return "The portal could not be reached. This is a connection problem rather than a refusal.";
}

/** The words for a failure that is a connection problem, kept in one place too. */
export const OFFLINE_MESSAGE = "The portal could not be reached.";
