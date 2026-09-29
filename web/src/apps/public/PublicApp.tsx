import { useEffect, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";

import { Badge, EmptyState, ErrorState, Skeleton, Status } from "../../components/feedback";
import { Icon } from "../../lib/icons";
import { Button, LinkButton, SearchField } from "../../components/controls";
import { Pagination, PageHead, Tabs, TabLink } from "../../components/shell";
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
  Tag,
} from "../../components/data";
import { api, ApiError, type Project } from "../../lib/api";
import { useSession } from "../../lib/session";

/** The server's page size. meta.page_size reports it, but the cap is the contract. */
const PAGE_SIZE = 24;

/** How long the search field rests before it becomes a query. */
const SEARCH_DEBOUNCE_MS = 300;

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
  const [search] = useSearchParams();

  // The route is declared `/events/*`, which yields the splat under "*" and
  // nothing else, so params.slug was always undefined and every event link fell
  // through to the directory. The first segment names the event and the rest is
  // the sub-route.
  const segments = (params["*"] ?? "").split("/").filter(Boolean);
  const isGalleryRoute = segments[0] === "gallery";
  const slug = isGalleryRoute ? "" : (segments[0] ?? "");
  const view = search.get("view") ?? (isGalleryRoute ? "gallery" : segments[1] ?? "");

  // The project route is declared with a named parameter rather than a splat,
  // so it arrives here from the other branch. Every project card in the gallery
  // links to it, and it had no route at all: the click landed on the app-level
  // not-found from a page that looked perfectly healthy.
  if (params.projectID) return <ProjectApp projectID={params.projectID} />;

  if (view === "gallery") return <GalleryApp eventSlug={slug} />;
  if (slug) return <EventApp slug={slug} />;
  return <EventDirectory />;
}

/**
 * One project, for a signed-in visitor.
 *
 * Deliberately read-only. A submission is the team's work, and the routes that
 * change it require team captaincy; a public page that offered an edit would be
 * offering it to anyone who read the URL.
 */
function ProjectApp({ projectID }: { projectID: string }) {
  const [project, setProject] = useState<Project | null>(null);
  const [teamName, setTeamName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let live = true;
    setLoading(true);
    setError(null);
    api
      .submission(projectID)
      .then((response) => {
        if (!live) return;
        setProject(response.data);
        setTeamName(response.team?.name ?? "");
      })
      .catch((caught) => live && setError(describe(caught)))
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [projectID]);

  if (loading) return <Skeleton lines={6} />;
  if (error) return <ErrorState title="That project could not be loaded" body={error} />;
  if (!project) return null;

  return (
    <section className="stack stack-5">
      <PageHead title={project.title} lede={project.summary} />
      <Card>
        <CardBody>
          <div className="stack stack-4">
            <div className="cluster cluster-3">
              <Status icon="circle-check">{project.status.replace(/_/g, " ")}</Status>
              {teamName ? <Tag>{teamName}</Tag> : null}
            </div>
            {project.description ? <p className="card__lede">{project.description}</p> : null}
            {project.story ? <p className="card__lede">{project.story}</p> : null}
            <div className="cluster cluster-3">
              {project.repo_url ? (
                <a
                  className="btn btn--tertiary"
                  href={project.repo_url}
                  target="_blank"
                  rel="noreferrer noopener"
                >
                  <Icon name="external-link" size={16} />
                  Repository
                </a>
              ) : null}
              {project.live_url ? (
                <a
                  className="btn btn--tertiary"
                  href={project.live_url}
                  target="_blank"
                  rel="noreferrer noopener"
                >
                  <Icon name="external-link" size={16} />
                  Live site
                </a>
              ) : null}
            </div>
            {project.tags.length > 0 ? (
              <div className="cluster cluster-3">
                {project.tags.map((tag) => (
                  <Tag key={tag}>{tag}</Tag>
                ))}
              </div>
            ) : null}
          </div>
        </CardBody>
      </Card>
      <LinkButton to="/events" variant="ghost" icon="chevron-left">
        Back to events
      </LinkButton>
    </section>
  );
}

function RetryButton({ onRetry }: { onRetry: () => void }) {
  return (
    <Button variant="secondary" icon="refresh-cw" onClick={onRetry}>
      Try again
    </Button>
  );
}

function EventDirectory() {
  const [events, setEvents] = useState<Awaited<ReturnType<typeof api.events>>["data"] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    let live = true;
    setError(null);
    api
      .events()
      .then((response) => live && setEvents(response.data))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, [reload]);

  if (error) {
    return (
      <ErrorState
        title="Events could not be loaded"
        body={error}
        retry={<RetryButton onRetry={() => setReload((value) => value + 1)} />}
      />
    );
  }
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
  const [loaded, setLoaded] = useState<Awaited<ReturnType<typeof api.event>> | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    let live = true;
    setError(null);
    api
      .event(slug)
      .then((response) => live && setLoaded(response))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, [slug, reload]);

  if (error) {
    return (
      <ErrorState
        title="That event could not be loaded"
        body={error}
        retry={<RetryButton onRetry={() => setReload((value) => value + 1)} />}
      />
    );
  }
  if (!loaded) return <Skeleton lines={5} />;

  // The related collections are siblings of `data`, not fields inside it. Reading
  // event.tracks found nothing, and the count below threw on the first line.
  const event = loaded.data;
  const tracks = loaded.tracks;
  const prizes = loaded.prizes;
  const milestones = loaded.milestones;

  return (
    <div className="stack stack-8">
      <PageHead
        title={event.name}
        {...(event.summary ? { lede: event.summary } : {})}
        actions={
          <LinkButton to={`/events/${slug}/gallery`} variant="primary" icon="folder-kanban">
            Project gallery
          </LinkButton>
        }
      />

      <Tabs>
        <TabLink to={`/events/${slug}`} label="Overview" active icon="file-text" />
        <TabLink to={`/events/${slug}/gallery`} label="Gallery" active={false} icon="folder-kanban" />
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
              <DescriptionValue>{tracks.length}</DescriptionValue>
              <DescriptionTerm>Prizes</DescriptionTerm>
              <DescriptionValue>{prizes.length}</DescriptionValue>
              <DescriptionTerm>Judges</DescriptionTerm>
              <DescriptionValue>{loaded.judge_count}</DescriptionValue>
            </DescriptionList>
          </Card>
        </div>
      </div>

      {tracks.length > 0 ? (
        <section className="stack stack-4">
          <h3 className="card__title">Tracks</h3>
          <div className="cluster">
            {tracks.map((track) => (
              <Badge key={track.id} icon="route">
                {track.name}
              </Badge>
            ))}
          </div>
        </section>
      ) : null}

      {milestones.length > 0 ? (
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
              {milestones.map((milestone) => (
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

      {prizes.length > 0 ? (
        <section className="stack stack-4">
          <h3 className="card__title">Prizes</h3>
          <Table caption="What the panel is awarding.">
            <thead>
              <tr>
                <Th>Prize</Th>
                <Th>Description</Th>
                <Th numeric>Rank</Th>
              </tr>
            </thead>
            <tbody>
              {prizes.map((prize) => (
                <Tr key={prize.id}>
                  <Td strong>{prize.name}</Td>
                  <Td meta>{prize.description || "—"}</Td>
                  <Td numeric>{ordinal(prize.rank)}</Td>
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
 * the first twenty matches is worse than one that says there are more. That is
 * also why the page control exists at all — the server caps a page at 24, so
 * without it every project past the first page is unreachable.
 */
function GalleryApp({ eventSlug }: { eventSlug: string }) {
  const session = useSession();
  const [searchParams, setSearchParams] = useSearchParams();
  const [events, setEvents] = useState<Awaited<ReturnType<typeof api.events>>["data"]>([]);
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [meta, setMeta] = useState<{ page: number; page_size: number; total: number } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [eventsError, setEventsError] = useState<string | null>(null);
  const [eventsPending, setEventsPending] = useState(true);
  const [reload, setReload] = useState(0);

  const query = searchParams.get("q") ?? "";
  // The route's own event beats the portal's first event, so a gallery reached
  // from an event page shows that event rather than whichever sorts first. `||`
  // rather than `??`, because /events/gallery names no event and an empty string
  // is a value here, not an absent one.
  const eventSlugParam = searchParams.get("event") || eventSlug || events[0]?.slug || "";
  const track = searchParams.get("track") ?? "";
  const page = Math.max(1, Number(searchParams.get("page") ?? "1") || 1);

  // Typed but not yet applied. A request and a history entry per keystroke makes
  // the server chase the typist and fills the back button with half-typed queries.
  const [typed, setTyped] = useState(query);

  useEffect(() => {
    let live = true;
    setEventsError(null);
    setEventsPending(true);
    api
      .events()
      .then((response) => {
        if (!live) return;
        setEvents(response.data);
        setEventsPending(false);
      })
      .catch((caught) => {
        if (!live) return;
        setEventsError(describe(caught));
        setEventsPending(false);
      });
    return () => {
      live = false;
    };
  }, [reload]);

  useEffect(() => {
    if (typed === query) return;
    const timer = setTimeout(() => {
      setSearchParams(
        (current) => {
          const next = new URLSearchParams(current);
          if (typed) next.set("q", typed);
          else next.delete("q");
          next.delete("page");
          return next;
        },
        { replace: true },
      );
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [typed, query, setSearchParams]);

  useEffect(() => {
    if (!eventSlugParam) return;
    let live = true;
    setProjects(null);
    setMeta(null);
    setError(null);
    api
      .projects(eventSlugParam, {
        ...(query ? { q: query } : {}),
        ...(track ? { track } : {}),
        page,
      })
      .then((response) => {
        if (!live) return;
        setProjects(response.data);
        setMeta(response.meta);
      })
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, [eventSlugParam, track, query, page, reload]);

  function apply(next: Record<string, string | number | undefined>) {
    setSearchParams(
      (current) => {
        const merged = new URLSearchParams(current);
        for (const [key, value] of Object.entries(next)) {
          if (value === undefined || value === "") merged.delete(key);
          else merged.set(key, String(value));
        }
        return merged;
      },
      { replace: true },
    );
  }

  const pageSize = meta?.page_size ?? PAGE_SIZE;
  const total = meta?.total ?? projects?.length ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));

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
          value={typed}
          onChange={setTyped}
          placeholder="Title, summary or tag"
        />
        <label className="field">
          <span className="field__label">Event</span>
          <select
            className="select"
            value={eventSlugParam}
            onChange={(event) => apply({ event: event.target.value, page: undefined })}
          >
            {events.length === 0 ? (
              <option value={eventSlugParam}>{eventSlugParam || "No events"}</option>
            ) : null}
            {events.map((event) => (
              <option key={event.id} value={event.slug}>
                {event.name}
              </option>
            ))}
          </select>
        </label>
      </div>

      {eventsError ? (
        <ErrorState
          title="The event list could not be loaded"
          body={eventsError}
          retry={<RetryButton onRetry={() => setReload((value) => value + 1)} />}
        />
      ) : null}
      {error ? (
        <ErrorState
          title="The gallery could not be loaded"
          body={error}
          retry={<RetryButton onRetry={() => setReload((value) => value + 1)} />}
        />
      ) : null}
      {!projects && !error && !eventsError ? <Skeleton lines={4} /> : null}

      {!eventSlugParam && !eventsError && !eventsPending && !projects ? (
        <EmptyState
          title="There are no events to browse"
          body="A gallery belongs to an event, and this portal has none yet. Once one is created its projects appear here."
          icon="calendar"
        />
      ) : null}

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
                setTyped("");
                setSearchParams(new URLSearchParams(), { replace: true });
              }}
            >
              Clear filters
            </Button>
          }
        />
      ) : null}

      {projects && projects.length > 0 ? (
        <>
          <p className="table__meta">
            {total === 1 ? "1 project" : `${total} projects`}
            {total > pageSize
              ? ` · showing ${(page - 1) * pageSize + 1}–${Math.min(page * pageSize, total)}`
              : null}
          </p>
          <div className="gallery-grid">
            {projects.map((project) => (
              <ProjectCard key={project.id} project={project} signedIn={Boolean(session.user)} />
            ))}
          </div>
          <Pagination
            page={page}
            pageCount={pageCount}
            onPage={(next) => apply({ page: next > 1 ? next : undefined })}
          />
        </>
      ) : null}
    </div>
  );
}

function ProjectCard({ project, signedIn }: { project: Project; signedIn: boolean }) {
  return (
    <Card interactive>
      <div className="cluster cluster-2">
        {/* design.md 10.3: the outcome is words and a glyph, never a colour.
            The project list carries a track id, not a track name, and no rank:
            both were declared on the client and never sent, so the badge they fed
            rendered blank rather than wrong, which is harder to notice. */}
        {project.track_id ? <Badge icon="route">{project.track_id}</Badge> : null}
        <Status icon={project.status === "submitted" ? "circle-check" : "pencil"} muted>
          {project.status.replace(/_/g, " ")}
        </Status>
      </div>
      <h3 className="card__title">{project.title}</h3>
      {project.summary ? <p className="card__lede">{project.summary}</p> : null}
      {project.eligibility && project.eligibility !== "pending" ? (
        <p className="table__meta">{project.eligibility}</p>
      ) : null}
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
