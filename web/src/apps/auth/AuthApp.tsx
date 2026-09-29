import { useEffect, useRef, useState, type FormEvent } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";

import { Alert } from "../../components/feedback";
import { Button, Field, LinkButton } from "../../components/controls";
import { ApiError, api, type SessionUser } from "../../lib/api";
import { describe } from "../../lib/errors";
import { navigation } from "../../lib/navigation";
import { useSession } from "../../lib/session";

/** Deliberately loose: the server is the authority on what an address may be. */
const EMAIL_SHAPE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

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
 *
 * Two rules shape the failure handling. design.md 13 wants an error message to
 * name the field and the correction, so an empty field is caught here instead of
 * costing a round trip. And one attempt produces exactly one message: a network
 * fault is a network fault, not a wrong password.
 */
export function AuthApp() {
  const session = useSession();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const next = params.get("next");

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [emailError, setEmailError] = useState<string | null>(null);
  const [passwordError, setPasswordError] = useState<string | null>(null);
  // A fresh object per attempt: the same refusal twice still counts as a new
  // failure, so the focus rule below runs again rather than silently not.
  const [failure, setFailure] = useState<{ message: string } | null>(null);
  const [busy, setBusy] = useState(false);

  const emailField = useRef<HTMLInputElement>(null);
  const passwordField = useRef<HTMLInputElement>(null);
  const failureNotice = useRef<HTMLDivElement>(null);

  // The submit button is disabled while the request runs, which drops focus to
  // the document; without this the viewer is left with nowhere to carry on from
  // when the attempt fails.
  useEffect(() => {
    if (failure) failureNotice.current?.focus();
  }, [failure]);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    if (busy) return;
    setFailure(null);

    // noValidate is kept, and replaced: the browser's own bubbles cannot be
    // styled, cannot be read by a screen reader on some engines, and vanish on
    // the next keystroke. The first invalid control takes focus so the message
    // and the field it belongs to are found together.
    const emailProblem = !email.trim()
      ? "Enter the email address for your account."
      : EMAIL_SHAPE.test(email.trim())
        ? null
        : "Enter an email address in the form name@example.com.";
    const passwordProblem = password ? null : "Enter your password.";
    setEmailError(emailProblem);
    setPasswordError(passwordProblem);
    if (emailProblem) {
      emailField.current?.focus();
      return;
    }
    if (passwordProblem) {
      passwordField.current?.focus();
      return;
    }

    setBusy(true);
    try {
      await api.signIn(email.trim(), password);
      // The session is re-read from the server rather than patched locally, so
      // what the UI shows is exactly what the backend believes.
      const signedIn = await session.refresh();
      // A `next` that is not a path on this origin would be an open redirect, so
      // only same-site paths are honoured.
      navigate(destinationFor(next, signedIn.user), { replace: true });
    } catch (caught) {
      // The server's own words when it answered; the shared connection wording
      // when it did not. Blaming the password for a server that could not be
      // reached sends the viewer to reset something that was never wrong.
      setFailure({ message: caught instanceof ApiError ? caught.message : describe(caught) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth">
      {/* The two class names below style nothing yet. They are kept as hooks for
          the stylesheet rather than stripped, so a future rule for this column
          does not have to rename the markup. */}
      <div className="auth__brand">
        {/* 320 is the width .auth__logo is capped at and 107 the height that
            matches the lockup's own proportions, so the reserved box and the
            rendered box agree instead of shoving the heading down on load. */}
        {/* No lockup here. The public header above already carries one, and
            two of them on one screen reads as a mistake rather than a brand. */}
        <p className="auth__eyebrow">Judging, organized</p>
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

      {/* The panel is the surface. It is not a card inside a panel: a light card
          on the dark ramp would put a small pale rectangle on a large dark one,
          which is two competing edges rather than one. */}
      <div className="auth__panel">
        <div className="stack stack-4">
          <h2 className="auth__panel-title">Sign in</h2>
          <p className="auth__panel-lede">Use the email and password for your account.</p>

          {/* One attempt, one message: this or a field error, never both. */}
          {failure ? (
            <div tabIndex={-1} ref={failureNotice}>
              <Alert kind="error" title="That did not work">
                {failure.message}
              </Alert>
            </div>
          ) : null}

          <form className="stack stack-5" onSubmit={onSubmit} noValidate>
            <Field id="email" label="Email" {...(emailError ? { error: emailError } : {})}>
              {({ id, invalid, describedBy }) => (
                <input
                  id={id}
                  ref={emailField}
                  className="input"
                  type="email"
                  name="email"
                  autoComplete="username"
                  autoCapitalize="none"
                  spellCheck={false}
                  aria-invalid={invalid}
                  aria-describedby={describedBy}
                  value={email}
                  onChange={(event) => {
                    setEmail(event.target.value);
                    if (emailError) setEmailError(null);
                  }}
                />
              )}
            </Field>

            <Field id="password" label="Password" {...(passwordError ? { error: passwordError } : {})}>
              {({ id, invalid, describedBy }) => (
                <input
                  id={id}
                  ref={passwordField}
                  className="input"
                  type="password"
                  name="password"
                  autoComplete="current-password"
                  aria-invalid={invalid}
                  aria-describedby={describedBy}
                  value={password}
                  onChange={(event) => {
                    setPassword(event.target.value);
                    if (passwordError) setPasswordError(null);
                  }}
                />
              )}
            </Field>

            {/* Always in the document so the change to "Signing in" is announced
                by a region that already exists, which is how a live region is
                reliably heard. */}
            <p className="visually-hidden" role="status">
              {busy ? "Signing in…" : ""}
            </p>

            <div className="actions">
              <Button variant="primary" icon="log-in" type="submit" loading={busy}>
                Sign in
              </Button>
              {/* A ghost button on the dark panel would be a transparent box with
                  a mist label, which is legible but reads as an absent control
                  next to a filled one. A quiet underline is the right weight
                  for the secondary action on a sign-in form. */}
              <LinkButton to="/events" variant="ghost" className="auth__panel-link">
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

/**
 * A destination, or the dashboard when the destination would refuse the viewer.
 *
 * `safeNext` answers "is this address on our origin". This answers the other
 * question: `?next=/judge` for an account that does not judge landed on that
 * page's refusal alert, which never says where the viewer was headed or how to
 * get back, so the intent behind the link was lost. A path whose navigation
 * group is not offered to the role that just signed in is dropped for the
 * dashboard, where there is nothing to refuse.
 *
 * A path outside the navigation map is followed as it stands. Not being listed
 * is not being forbidden: a project page, for one, is reachable without a
 * sidebar entry, and only the server decides what an account may read.
 */
function destinationFor(next: string | null, user: SessionUser | null): string {
  const path = safeNext(next);
  if (!user) return path;
  const group = navigation.find((entry) =>
    entry.items.some((item) => path === item.to || path.startsWith(`${item.to}/`)),
  );
  if (group && !group.roles.includes(user.role)) return "/dashboard";
  return path;
}
