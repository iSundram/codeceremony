import { useEffect, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";

import { Badge, EmptyState, ErrorState, Skeleton, Status } from "../../components/feedback";
import { Button, LinkButton, SearchField } from "../../components/controls";
import { PageHead, Tabs, TabLink } from "../../components/shell";
import {
  Card,
  CardActions,
  CardBody,
  CardTitle,
  DescriptionList,
  DescriptionTerm,
  DescriptionValue,
  Table,
  Td,
  Th,
  Tr,
} from "../../components/data";
import { api, ApiError, type EventDetail, type Project } from "../../lib/api";
import { useSession } from "../../lib/session";

/**
 * The public app: the event directory, an event, and its gallery.
 *
 * Nothing here is behind a role, because a public gallery is one of the things
 * the event brief asks for and one of the things an organizer would otherwise
 * have to bolt on afterwards. Scores are not on this surface: results are hidden
 * until an organizer publishes them, which is a server-side decision the
 * response reflects rather than something this page decides.
 */
export function PublicApp() {
  const params = useParams();
  const slug = params["slug"];
  const [search] = useSearchParams();
  const view = search.get("view");

  if (view === "gallery" || (slug === "gallery" && !search.get("event"))) return <GalleryApp />;
  if (slug) return <EventApp slug={slug} />;
  return <EventDirectory />;
}

function EventDirectory() {
  const [events, setEvents] = useState<Awaited<ReturnType<typeof api.events>>["data"] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    api
      .events()
      .then((response) => live && setEvents(response.data))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, []);

  if (error) return <ErrorState title="Events could not be loaded" body={error} />;
  if (!events) return <Skeleton lines={4} />;

  return (
    <div className="stack stack-8">
      <PageHead
        title="Events"
        lede="Every event on this portal, with its state, deadline and submission count."
      />
      {events.length === 0 ? (
        <EmptyState
          title="No events yet"
          body="Once an event is created it appears here with its submissions, panel and gallery."
          icon="calendar"
        />
      ) : (
        <div className="gallery-grid">
          {events.map((event) => (
            <Card key={event.id} interactive>
              <div className="cluster cluster-2">
                {event.submissions_open ? (
                  <Status icon="circle-check">Submissions open</Status>
                ) : (
                  <Status icon="lock" muted>
                    Closed
                  </Status>
                )}
                <Badge>{event.state}</Badge>
              </div>
              <CardTitle>{event.name}</CardTitle>
              {event.summary ? <CardBody>{event.summary}</CardBody> : null}
              <div className="project-card__meta">
                <span className="table__meta">
                  Closes {new Date(event.submissions_close).toLocaleDateString()}
                </span>
              </div>
              <CardActions>
                <LinkButton to={`/events/${event.slug}`} variant="secondary" icon="arrow-right">
                  Open
                </LinkButton>
              </CardActions>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

function EventApp({ slug }: { slug: string }) {
  const [event, setEvent] = useState<EventDetail | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    api
      .event(slug)
      .then((response) => live && setEvent(response.data))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, [slug]);

  if (error) return <ErrorState title="That event could not be loaded" body={error} />;
  if (!event) return <Skeleton lines={5} />;

  return (
    <div className="stack stack-8">
      <PageHead
        title={event.name}
        {...(event.summary ? { lede: event.summary } : {})}
        actions={
          <LinkButton to={`/events/${slug}?view=gallery`} variant="primary" icon="folder-kanban">
            Project gallery
          </LinkButton>
        }
      />

      <Tabs>
        <TabLink to={`/events/${slug}`} label="Overview" active icon="file-text" />
        <TabLink to={`/events/${slug}?view=gallery`} label="Gallery" active={false} icon="folder-kanban" />
        <TabLink to={`/events/${slug}?view=results`} label="Results" active={false} icon="trophy" />
      </Tabs>

      <div className="grid">
        <div className="col-7">
          <Card>
            <CardTitle>About this event</CardTitle>
            <p className="card__lede">{event.description}</p>
          </Card>
        </div>
        <div className="col-5">
          <Card>
            <CardTitle>Details</CardTitle>
            <DescriptionList>
              <DescriptionTerm>State</DescriptionTerm>
              <DescriptionValue>
                <Badge>{event.state}</Badge>
              </DescriptionValue>
              <DescriptionTerm>Timezone</DescriptionTerm>
              <DescriptionValue>{event.timezone}</DescriptionValue>
              <DescriptionTerm>Submissions</DescriptionTerm>
              <DescriptionValue>
                {event.submissions_open ? "Open" : "Closed"} ·{" "}
                {new Date(event.submissions_close).toLocaleDateString()}
              </DescriptionValue>
              <DescriptionTerm>Tracks</DescriptionTerm>
              <DescriptionValue>{event.tracks.length}</DescriptionValue>
              <DescriptionTerm>Prizes</DescriptionTerm>
              <DescriptionValue>{event.prizes.length}</DescriptionValue>
            </DescriptionList>
          </Card>
        </div>
      </div>

      {event.tracks.length > 0 ? (
        <section className="stack stack-4">
          <h3 className="card__title">Tracks</h3>
          <div className="cluster">
            {event.tracks.map((track) => (
              <Badge key={track.id} icon="route">
                {track.name}
              </Badge>
            ))}
          </div>
        </section>
      ) : null}

      {event.milestones.length > 0 ? (
        <section className="stack stack-4">
          <h3 className="card__title">Schedule</h3>
          <Table caption="The milestones that define the event's shape.">
            <thead>
              <tr>
                <Th>Milestone</Th>
                <Th>Detail</Th>
                <Th numeric>Due</Th>
              </tr>
            </thead>
            <tbody>
              {event.milestones.map((milestone) => (
                <Tr key={milestone.id}>
                  <Td strong>{milestone.title}</Td>
                  <Td meta>{milestone.detail ?? "—"}</Td>
                  <Td numeric>
                    {milestone.due_at ? new Date(milestone.due_at).toLocaleDateString() : "—"}
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        </section>
      ) : null}
    </div>
  );
}

/**
 * The gallery.
 *
 * Search and track filter are server-side queries, not a client-side filter over
 * an already-fetched page: a gallery of forty projects that silently shows only
 * the first twenty matches is worse than one that says there are more.
 */
function GalleryApp() {
  const session = useSession();
  const [searchParams, setSearchParams] = useSearchParams();
  const [query, setQuery] = useState(searchParams.get("q") ?? "");
  const [events, setEvents] = useState<Awaited<ReturnType<typeof api.events>>["data"]>([]);
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const eventSlug = searchParams.get("event") ?? events[0]?.slug ?? "";
  const track = searchParams.get("track") ?? "";
  const page = Number(searchParams.get("page") ?? "1");

  useEffect(() => {
    let live = true;
    api
      .events()
      .then((response) => live && setEvents(response.data))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);

  useEffect(() => {
    if (!eventSlug) return;
    let live = true;
    setProjects(null);
    setError(null);
    api
      .projects(eventSlug, {
        ...(searchParams.get("q") ? { q: searchParams.get("q") as string } : {}),
        ...(track ? { track } : {}),
        page,
      })
      .then((response) => live && setProjects(response.data))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, [eventSlug, track, page, searchParams]);

  function apply(next: Record<string, string | number | undefined>) {
    const merged = new URLSearchParams(searchParams);
    for (const [key, value] of Object.entries(next)) {
      if (value === undefined || value === "") merged.delete(key);
      else merged.set(key, String(value));
    }
    setSearchParams(merged, { replace: true });
  }

  return (
    <div className="stack stack-6">
      <PageHead
        title="Project gallery"
        lede="Everything submitted to this event, with search and track filtering."
      />

      <div className="gallery-filters">
        <SearchField
          id="gallery-search"
          label="Search projects"
          value={query}
          onChange={(value) => {
            setQuery(value);
            apply({ q: value || undefined, page: 1 });
          }}
        />
        <label className="field">
          <span className="field__label">Event</span>
          <select
            className="select"
            value={eventSlug}
            onChange={(event) => apply({ event: event.target.value, page: 1 })}
          >
            {events.map((event) => (
              <option key={event.id} value={event.slug}>
                {event.name}
              </option>
            ))}
          </select>
        </label>
      </div>

      {error ? <ErrorState title="The gallery could not be loaded" body={error} /> : null}
      {!projects && !error ? <Skeleton lines={4} /> : null}

      {projects && projects.length === 0 ? (
        <EmptyState
          title="No projects match"
          body="Nothing has been submitted under those filters. Clearing the search shows everything."
          icon="folder-kanban"
          action={
            <Button
              variant="secondary"
              icon="x"
              onClick={() => {
                setQuery("");
                setSearchParams(new URLSearchParams(), { replace: true });
              }}
            >
              Clear filters
            </Button>
          }
        />
      ) : null}

      {projects && projects.length > 0 ? (
        <div className="gallery-grid">
          {projects.map((project) => (
            <ProjectCard key={project.id} project={project} signedIn={Boolean(session.user)} />
          ))}
        </div>
      ) : null}
    </div>
  );
}

function ProjectCard({ project, signedIn }: { project: Project; signedIn: boolean }) {
  return (
    <Card interactive>
      <div className="cluster cluster-2">
        {project.track_name ? <Badge icon="route">{project.track_name}</Badge> : null}
        {/* design.md 10.3: the outcome is words and a glyph, never a colour. */}
        {project.rank ? (
          <Status icon="trophy">{ordinal(project.rank)} place</Status>
        ) : (
          <Status icon="clock" muted>
            {project.status}
          </Status>
        )}
        {project.low_information ? (
          <Badge icon="circle-alert" variant="outline">
            Low information
          </Badge>
        ) : null}
      </div>
      <h3 className="card__title">{project.title}</h3>
      {project.summary ? <p className="card__lede">{project.summary}</p> : null}
      {project.team_name ? <p className="table__meta">by {project.team_name}</p> : null}
      {project.tags && project.tags.length > 0 ? (
        <div className="cluster cluster-2">
          {project.tags.slice(0, 4).map((tag) => (
            <Badge key={tag} variant="outline">
              {tag}
            </Badge>
          ))}
        </div>
      ) : null}
      {signedIn ? (
        <CardActions>
          <LinkButton to={`/projects/${project.id}`} variant="tertiary" icon="arrow-right">
            Details
          </LinkButton>
        </CardActions>
      ) : null}
    </Card>
  );
}

function ordinal(n: number): string {
  const suffix = n % 100 >= 11 && n % 100 <= 13 ? "th" : ["th", "st", "nd", "rd"][n % 10] ?? "th";
  return `${n}${suffix}`;
}

function describe(caught: unknown): string {
  if (caught instanceof ApiError) return caught.message;
  return "The portal could not be reached.";
}
