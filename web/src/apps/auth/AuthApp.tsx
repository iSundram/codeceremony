import { useState, type FormEvent } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";

import { Alert } from "../../components/feedback";
import { Button, Field, LinkButton } from "../../components/controls";
import { ApiError, api } from "../../lib/api";
import { useSession } from "../../lib/session";

/**
 * The auth app.
 *
 * design.md 5.2's shell assumes a signed-in viewer with a sidebar to navigate.
 * A signed-out visitor has neither, so sign-in renders on its own rather than
 * inside an empty shell. It is also the one place the full wordmark belongs at a
 * large size, per 10.2.
 *
 * The form posts to the same `/login` endpoint the server-rendered pages used, so
 * there is one sign-in path rather than two that could drift. The session cookie
 * it sets is what every other request authenticates with.
 */
export function AuthApp() {
  const session = useSession();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const next = params.get("next");

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.signIn(email, password);
      // The session is re-read from the server rather than patched locally, so
      // what the UI shows is exactly what the backend believes.
      await session.refresh();
      // A `next` that is not a path on this origin would be an open redirect, so
      // only same-site paths are honoured.
      navigate(safeNext(next), { replace: true });
    } catch (caught) {
      setError(
        caught instanceof ApiError
          ? caught.message
          : "The portal could not be reached. Check that the server is running.",
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth">
      <div className="auth__brand">
        <img className="auth__logo" src="/brand/logo.svg" alt="CodeCeremony" width={300} height={100} />
        <h1 className="auth__title">Sign in to your portal</h1>
        <p className="auth__lede">
          One account carries every role you hold: your team, your judging assignments, and the
          events you organize.
        </p>
        <ul className="auth__points">
          <li>Role isolation is enforced in the backend, not in this form.</li>
          <li>Judging scores are visible only to the judge who wrote them.</li>
          <li>Every action is recorded, including the refusals.</li>
        </ul>
      </div>

      <div className="auth__panel">
        <div className="card">
          <h2 className="card__title">Sign in</h2>
          <p className="card__lede">Use the email and password for your account.</p>

          {error ? (
            <Alert kind="error" title="That did not work">
              {error}
            </Alert>
          ) : null}

          <form className="stack stack-5" onSubmit={onSubmit} noValidate>
            <Field id="email" label="Email" {...(error ? { error: "Check the email and password" } : {})}>
              {({ id, invalid }) => (
                <input
                  id={id}
                  className="input"
                  type="email"
                  name="email"
                  autoComplete="username"
                  autoCapitalize="none"
                  spellCheck={false}
                  required
                  value={email}
                  aria-invalid={invalid}
                  onChange={(event) => setEmail(event.target.value)}
                />
              )}
            </Field>

            <Field id="password" label="Password">
              {({ id }) => (
                <input
                  id={id}
                  className="input"
                  type="password"
                  name="password"
                  autoComplete="current-password"
                  required
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
              )}
            </Field>

            <div className="actions">
              <Button variant="primary" icon="log-in" type="submit" loading={busy}>
                Sign in
              </Button>
              <LinkButton to="/events" variant="ghost">
                Browse the gallery instead
              </LinkButton>
            </div>
          </form>

          <details className="auth__seeded">
            <summary>Seeded accounts</summary>
            <p className="field__hint">
              A seeded portal creates these accounts so the acceptance checker and a first run have
              something to authenticate as. They are not production credentials, and seeding is
              refused in production without a private password.
            </p>
            <dl className="description-list">
              <dt>Organizer</dt>
              <dd>
                <code className="code-inline">organizer@example.org</code>
              </dd>
              <dt>Judge</dt>
              <dd>
                <code className="code-inline">judge-a@example.org</code>
              </dd>
              <dt>Participant</dt>
              <dd>
                <code className="code-inline">participant@example.org</code>
              </dd>
              <dt>Admin</dt>
              <dd>
                <code className="code-inline">admin@example.org</code>
              </dd>
            </dl>
            <p className="field__hint">The shared password is printed on every boot.</p>
          </details>
        </div>
      </div>
    </div>
  );
}

/**
 * Only same-site absolute paths are followed after sign-in.
 *
 * `next` arrives from the query string, so it is caller-controlled. Passing it to
 * a redirect unexamined would be an open redirect: a crafted link would sign a
 * visitor in and then drop them on an attacker's page with the session cookie
 * still attached. A protocol-relative value like `//evil.test` is the shape that
 * actually gets exploited, so it is rejected too.
 */
function safeNext(next: string | null): string {
  if (!next) return "/dashboard";
  if (!next.startsWith("/")) return "/dashboard";
  if (next.startsWith("//")) return "/dashboard";
  if (next.startsWith("/\\")) return "/dashboard";
  return next;
}
