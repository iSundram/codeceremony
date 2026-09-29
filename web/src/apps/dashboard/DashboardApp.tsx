import { useEffect, useState } from "react";

import { Alert, Badge, EmptyState, ErrorState, Skeleton, Status } from "../../components/feedback";
import { LinkButton } from "../../components/controls";
import { PageHead } from "../../components/shell";
import { Card, CardBody, CardTitle, StatCard, StatGrid } from "../../components/data";
import { api, ApiError, type EventSummary } from "../../lib/api";
import { isStaff, useSession } from "../../lib/session";

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
 */
export function DashboardApp() {
  const session = useSession();
  const [events, setEvents] = useState<EventSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    api
      .events()
      .then((response) => {
        if (live) setEvents(response.data);
      })
      .catch((caught) => {
        if (live) setError(caught instanceof ApiError ? caught.message : "The portal could not be reached.");
      });
    return () => {
      live = false;
    };
  }, []);

  if (error) {
    return (
      <ErrorState
        title="Events could not be loaded"
        body={error}
        retry={<LinkButton to="/dashboard" variant="secondary" icon="refresh-cw">Try again</LinkButton>}
      />
    );
  }

  if (!events) {
    return (
      <div className="stack stack-6">
        <Skeleton lines={1} />
        <Skeleton lines={4} />
      </div>
    );
  }

  const user = session.user;
  const open = events.filter((event) => event.submissions_open);

  return (
    <div className="stack stack-8">
      <PageHead
        title={user ? `Welcome back, ${firstName(user.display_name)}` : "CodeCeremony"}
        lede={
          user
            ? "Where you left off, and what is waiting on you."
            : "A self-hostable hackathon submission and judging platform. Weighted rubrics, cross-judge normalization, and role isolation enforced in the backend."
        }
        actions={user ? undefined : <LinkButton to="/login" variant="primary" icon="log-in">Sign in</LinkButton>}
      />

      {!user ? (
        <Alert kind="info" title="You are browsing as a visitor">
          The gallery and every published result are public. Signing in shows your teams, your
          assignments, and the events you organize.
        </Alert>
      ) : null}

      {user ? (
        <>
          <div className="cluster">
            <Status icon="circle-check">{user.state ?? "active"}</Status>
            <span className="table__meta">
              Signed in as {user.email} with the {user.role} role.
            </span>
          </div>

          <StatGrid>
            <StatCard
              label="Events"
              value={events.length}
              meta={`${open.length} open for submissions`}
              icon="calendar"
            />
            {isStaff(user) ? <StaffCards slug={events[0]?.slug} /> : null}
            {user.role === "judge" ? <JudgeCard slug={events[0]?.slug} /> : null}
            {user.role === "participant" ? (
              <StatCard label="Your account" value="—" meta="Teams and projects are in My account" icon="users" />
            ) : null}
          </StatGrid>

          <section className="stack stack-4">
            <div className="cluster cluster-between">
              <h3 className="card__title">Where to go next</h3>
              <LinkButton to="/account" variant="tertiary" icon="user">
                My account
              </LinkButton>
            </div>
            <div className="grid">
              {isStaff(user) ? (
                <div className="col-4">
                  <Card interactive>
                    <CardTitle>Progress</CardTitle>
                    <CardBody>Who has not started, and who is finished.</CardBody>
                    <div className="card__actions">
                      <LinkButton to="/organizer" variant="primary" icon="arrow-right">
                        Open
                      </LinkButton>
                    </div>
                  </Card>
                </div>
              ) : null}
              {user.role === "judge" || isStaff(user) ? (
                <div className="col-4">
                  <Card interactive>
                    <CardTitle>Judging</CardTitle>
                    <CardBody>Your assignments, the rubric, and pairwise comparison.</CardBody>
                    <div className="card__actions">
                      <LinkButton to="/judge" variant="primary" icon="arrow-right">
                        Open
                      </LinkButton>
                    </div>
                  </Card>
                </div>
              ) : null}
              <div className="col-4">
                <Card interactive>
                  <CardTitle>Events</CardTitle>
                  <CardBody>Every event on this portal, with its state and deadline.</CardBody>
                  <div className="card__actions">
                    <LinkButton to="/events" variant="secondary" icon="arrow-right">
                      Open
                    </LinkButton>
                  </div>
                </Card>
              </div>
            </div>
          </section>
        </>
      ) : (
        <Alert kind="info" title="What this portal guarantees">
          A judge cannot read another judge&apos;s scores by changing a URL, a query parameter or a
          request path. The same check runs on the JSON routes and the pages, so neither surface is
          a way around the other.
        </Alert>
      )}

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
    </div>
  );
}

function EventCard({ event }: { event: EventSummary }) {
  return (
    <Card interactive>
      <div className="cluster cluster-2">
        {event.submissions_open ? (
          <Status icon="circle-check">Submissions open</Status>
        ) : (
          <Status icon="lock" muted>
            Closed
          </Status>
        )}
        <Badge icon="tag">{event.state}</Badge>
      </div>
      <h4 className="card__title">{event.name}</h4>
      {event.summary ? <p className="card__lede">{event.summary}</p> : null}
      <div className="project-card__meta">
        <span className="table__meta">
          Submissions close {new Date(event.submissions_close).toLocaleDateString()}
        </span>
      </div>
      <div className="card__actions">
        <LinkButton to={`/events/${event.slug}`} variant="secondary" icon="arrow-right">
          Open event
        </LinkButton>
      </div>
    </Card>
  );
}

function StaffCards({ slug }: { slug?: string }) {
  if (!slug) return null;
  return (
    <StatCard
      label="Organizing"
      value="—"
      meta="Progress, panel, results and the audit trail"
      icon="clipboard-list"
    />
  );
}

function JudgeCard({ slug }: { slug?: string }) {
  if (!slug) return null;
  return (
    <StatCard
      label="Judging"
      value="—"
      meta="Your assignments and the rubric you score against"
      icon="gavel"
    />
  );
}

function firstName(name: string): string {
  return name.trim().split(/\s+/)[0] ?? name;
}
