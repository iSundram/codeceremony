import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";

import { ApiError, api, type SessionUser } from "./api";

/**
 * The session, resolved once at start-up and shared by everything.
 *
 * The portal authenticates with a cookie, so "am I signed in" is a question
 * about the server and not about anything held in the browser. That is the whole
 * point of backend-enforced authorization: there is no token in localStorage to
 * steal, and a role change takes effect on the next request because the role is
 * re-read from the store rather than trusted from a claim.
 */
export interface Session {
  user: SessionUser | null;
  loading: boolean;
  error: ApiError | null;
  refresh: () => Promise<Session>;
  signOut: () => Promise<void>;
}

const SIGNED_OUT: Session = {
  user: null,
  loading: true,
  error: null,
  refresh: async () => SIGNED_OUT,
  signOut: async () => {},
};

let current: Session = SIGNED_OUT;
const listeners = new Set<(session: Session) => void>();

function publish(session: Session) {
  current = session;
  for (const listener of listeners) listener(session);
}

export function useSession(): Session {
  const [session, setSession] = useState<Session>(current);
  useEffect(() => {
    const listener = (next: Session) => setSession(next);
    listeners.add(listener);
    // A component that mounts after the first resolve would otherwise read a
    // stale value until something else happened to publish.
    setSession(current);
    return () => {
      listeners.delete(listener);
    };
  }, []);
  return session;
}

/** Resolves the session once, before the router renders anything. */
export async function loadSession(): Promise<Session> {
  try {
    const response = await api.me();
    publish({
      user: response.data,
      loading: false,
      error: null,
      refresh: loadSession,
      signOut,
    });
  } catch (error) {
    // A signed-out visitor is not an error state; it is the normal case for the
    // public gallery. A genuine failure is reported so a broken backend is not
    // silently presented as "nobody is signed in".
    if (error instanceof ApiError && error.isForbidden) {
      publish({ ...SIGNED_OUT, loading: false, refresh: loadSession, signOut });
    } else {
      publish({
        ...SIGNED_OUT,
        loading: false,
        error: error instanceof ApiError ? error : null,
        refresh: loadSession,
        signOut,
      });
    }
  }
  return current;
}

async function signOut() {
  await fetch("/logout", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: "{}",
  });
  publish({ ...SIGNED_OUT, loading: false, refresh: loadSession, signOut });
}

/** True when the signed-in viewer holds one of the roles. */
export function hasRole(user: SessionUser | null, ...roles: SessionUser["role"][]): boolean {
  if (!user) return false;
  return roles.includes(user.role);
}

export function isStaff(user: SessionUser | null): boolean {
  return hasRole(user, "organizer", "admin");
}

/** Sends a signed-out visitor to the sign-in page, remembering where they were. */
export function useRequireAuth() {
  const session = useSession();
  const location = useLocation();
  const navigate = useNavigate();
  useEffect(() => {
    if (session.loading) return;
    if (!session.user) {
      const next = encodeURIComponent(location.pathname + location.search);
      navigate(`/login?next=${next}`, { replace: true });
    }
  }, [session.loading, session.user, location.pathname, location.search, navigate]);
  return session;
}
