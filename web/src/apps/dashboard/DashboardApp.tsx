import { Alert, EmptyState, ErrorState, Skeleton, Status } from "../../components/feedback";
import { Button, LinkButton } from "../../components/controls";
import { PageHead } from "../../components/shell";
import { Card, CardActions, CardBody, CardTitle, StatCard, StatGrid } from "../../components/data";
import { EventCard } from "../../components/cards";
import { api, type EventSummary, type SessionUser } from "../../lib/api";
import { isStaff, useSession } from "../../lib/session";
import { useAsync } from "../../hooks/useAsync";

/**
 * The dashboard app.
 *
 * Role-aware by construction: what a viewer is offered follows from the role the
 * backend reported, and every link leads to a route that authorizes
 * independently. Nothing here decides what anyone may do — it decides what to
 * draw. The event rules are explicit that a hidden button is not access control,
 * so the distinction is kept visible in the structure: the counts come from the
 * same API the pages behind them use, and a viewer who reaches a URL another way
 * is refused by the server.
 *
 * The roles are separate components rather than branches interleaved through one
 * render, so adding a role means adding a function instead of finding the four
 * places the current one is tested for.
 */
export function DashboardApp() {
  const session = useSession();
  const events = useAsync(() => api.events(), []);
  const user = session.user;
  const list = events.data?.data ?? [];
  const openCount = list.filter((event) => event.submissions_open).length;

  return (
    <div className="stack stack-8">
      <PageHead
        title={welcomeTitle(user)}
        lede={user ? "Where you left off, and what is waiting on you." : VISITOR_LEDE}
        actions={
          user ? undefined : (
            <LinkButton to="/login" variant="primary" icon="log-in">
              Sign in
            </LinkButton>
          )
        }
      />

      {/* One state at a time: a failure with a real retry, the first load, or
          the settled answer. The failure used to offer a link back to this same
          page, which is a permanent error screen with no way out. */}
      {events.error ? (
        <ErrorState
          title="Events could not be loaded"
          body={events.error}
          retry={
            <Button variant="secondary" icon="refresh-cw" onClick={events.reload}>
              Try again
            </Button>
          }
        />
      ) : !events.data ? (
        <Skeleton lines={4} />
      ) : (
        <>
          {user ? (
            <SignedIn user={user} events={list} openCount={openCount} />
          ) : (
            <Visitor />
          )}
          <EventsSection events={list} />
        </>
      )}
    </div>
  );
}

/**
 * What the visitor lede now says, security blurb included.
 *
 * The portal-guarantees note used to be an Alert that appeared above the fold
 * on every visit and pushed the page down; its substance is one clause of the
 * lede instead, where it is read once and does not cost a block of space each
 * time the dashboard is opened.
 */
const VISITOR_LEDE =
  "A self-hostable hackathon submission and judging platform. Weighted rubrics, cross-judge normalization, and role isolation enforced in the backend: a judge cannot read another judge's scores by changing a URL, a query parameter or a request path.";

function SignedIn({
  user,
  events,
  openCount,
}: {
  user: SessionUser;
  events: EventSummary[];
  openCount: number;
}) {
  return (
    <>
      <div className="cluster">
        <AccountState state={user.state} />
        <span className="table__meta">
          Signed in as {user.email} with the {user.role} role.
        </span>
      </div>

      <StatGrid>
        <StatCard
          label="Events"
          value={events.length}
          meta={`${openCount} open for submissions`}
          icon="calendar"
        />
        {isStaff(user) ? <OrganizingStat /> : null}
        {user.role === "judge" ? <JudgingStat /> : null}
        {user.role === "participant" ? (
          <StatCard
            label="Your account"
            value="—"
            meta="Teams and projects are in My account"
            icon="users"
          />
        ) : null}
      </StatGrid>

      <NextSteps user={user} />
    </>
  );
}

function Visitor() {
  return (
    <Alert kind="info" title="You are browsing as a visitor">
      The gallery and every published result are public. Signing in shows your teams, your
      assignments, and the events you organize.
    </Alert>
  );
}

/**
 * The account state, in the mapping the admin surface already uses.
 *
 * design.md 10.3: a state is words plus a glyph, never a colour, and the glyph
 * has to be the one the state deserves. One hard-coded `circle-check` meant a
 * suspended or pending-deletion account was drawn as a green tick.
 */
function AccountState({ state }: { state?: string }) {
  const value = state ?? "active";
  if (value === "active") return <Status icon="circle-check">Active</Status>;
  if (value === "deletion_pending") return <Status icon="clock">Deletion pending</Status>;
  return <Status icon="circle-alert">{value.replace(/_/g, " ")}</Status>;
}

/**
 * The two role entries.
 *
 * They used to render only when the portal already had an event, because they
 * read a slug they never used: a staff account on a fresh portal was offered
 * nothing, on a page whose entry links are valid without one. The destination
 * lives in the link below, which carries no event at all.
 */
function OrganizingStat() {
  return (
    <StatCard
      label="Organizing"
      value="—"
      meta="Progress, panel, results and the audit trail"
      icon="clipboard-list"
    />
  );
}

function JudgingStat() {
  return (
    <StatCard
      label="Judging"
      value="—"
      meta="Your assignments and the rubric you score against"
      icon="gavel"
    />
  );
}

function NextSteps({ user }: { user: SessionUser }) {
  const staff = isStaff(user);
  const judges = user.role === "judge" || staff;
  return (
    <section className="stack stack-4">
      <div className="cluster cluster-between">
        <h3 className="card__title">Where to go next</h3>
        <LinkButton to="/account" variant="tertiary" icon="user">
          My account
        </LinkButton>
      </div>
      <div className="grid">
        {staff ? (
          <StepCard
            title="Progress"
            body="Who has not started, and who is finished."
            to="/organizer"
          />
        ) : null}
        {judges ? (
          <StepCard
            title="Judging"
            body="Your assignments, the rubric, and pairwise comparison."
            to="/judge"
          />
        ) : null}
        <StepCard
          title="Events"
          body="Every event on this portal, with its state and deadline."
          to="/events"
          variant="secondary"
        />
      </div>
    </section>
  );
}

function StepCard({
  title,
  body,
  to,
  variant = "primary",
}: {
  title: string;
  body: string;
  to: string;
  variant?: "primary" | "secondary";
}) {
  return (
    <div className="col-4">
      <Card interactive>
        <CardTitle>{title}</CardTitle>
        <CardBody>{body}</CardBody>
        <CardActions>
          <LinkButton to={to} variant={variant} icon="arrow-right">
            Open
          </LinkButton>
        </CardActions>
      </Card>
    </div>
  );
}

function EventsSection({ events }: { events: EventSummary[] }) {
  return (
    <section className="stack stack-4">
      <h3 className="card__title">Events</h3>
      {events.length === 0 ? (
        <EmptyState
          title="No events yet"
          body="Once an event is created it appears here with its submissions, panel and gallery."
          icon="calendar"
        />
      ) : (
        <div className="gallery-grid">
          {events.map((event) => (
            <EventCard key={event.id} event={event} />
          ))}
        </div>
      )}
    </section>
  );
}

/**
 * The greeting, with the empty display name handled rather than pasted.
 *
 * `firstName("")` is an empty string, and interpolating it produced
 * "Welcome back, " with a trailing space — a greeting that reads as a slip on an
 * account that has simply never been given a name.
 */
function welcomeTitle(user: SessionUser | null): string {
  if (!user) return "CodeCeremony";
  const first = firstName(user.display_name);
  return first ? `Welcome back, ${first}` : "Welcome back";
}

function firstName(name: string): string {
  return name.trim().split(/\s+/)[0] ?? "";
}
