import { useEffect, useState, type ReactNode } from "react";
import { useParams, useSearchParams } from "react-router-dom";

import { Badge, EmptyState, ErrorState, Skeleton, Status } from "../../components/feedback";
import { Icon } from "../../lib/icons";
import { Button, LinkButton, SearchField, Select } from "../../components/controls";
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
import { EventCard } from "../../components/cards";
import { api, type Project } from "../../lib/api";
import { useAsync } from "../../hooks/useAsync";
import { useEvents } from "../../lib/events";
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
 *
 * Every read goes through `useAsync` or `useEvents`, so every surface owes the
 * viewer the same four answers — first load, failure with a retry, an explicit
 * "there is nothing", and the data — and no surface can render two of them at
 * once.
 */
export function PublicApp() {
  const params = useParams();

  // The route is declared `/events/*`, which yields the splat under "*" and
  // nothing else, so params.slug was always undefined and every event link fell
  // through to the directory. The first segment names the event and the rest is
  // the sub-route.
  const segments = (params["*"] ?? "").split("/").filter(Boolean);

  // The project route is declared with a named parameter rather than a splat,
  // so it arrives here from the other branch. Every project card in the gallery
  // links to it, and it had no route at all: the click landed on the app-level
  // not-found from a page that looked perfectly healthy.
  if (params.projectID) return <ProjectApp projectID={params.projectID} />;

  const [first, second] = segments;

  // `/events/gallery` is the portal-wide gallery the sidebar links to. It names
  // no event, so the gallery resolves one from the shared event list.
  if (first === "gallery") {
    return segments.length === 1 ? <GalleryApp eventSlug="" /> : <UnknownEventPage />;
  }

  if (!first) return <EventDirectory />;
  if (segments.length === 1) return <EventApp slug={first} />;
  if (segments.length === 2 && second === "gallery") return <GalleryApp eventSlug={first} />;

  // `/events/slug/anything` used to fall through to the overview, so an old or
  // mistyped link showed a healthy-looking page that answered nothing. design.md
  // 12 asks for a clear state on every surface, and this is that state.
  return <UnknownEventPage />;
}

function UnknownEventPage() {
  return (
    <EmptyState
      title="No such page"
      body="That address does not match a view of an event. The link may be from an older version of the portal, or the page may have moved."
      icon="calendar"
    />
  );
}

/** The retry every failed read owes the viewer, worded the same way everywhere. */
function RetryButton({ onRetry }: { onRetry: () => void }) {
  return (
    <Button variant="secondary" icon="refresh-cw" onClick={onRetry}>
      Try again
    </Button>
  );
}

/**
 * One project, for a signed-in visitor.
 *
 * Deliberately read-only. A submission is the team's work, and the routes that
 * change it require team captaincy; a public page that offered an edit would be
 * offering it to anyone who read the URL.
 */
function ProjectApp({ projectID }: { projectID: string }) {
  const submission = useAsync(() => api.submission(projectID), [projectID]);
  const project = submission.data?.data ?? null;
  const teamName = submission.data?.team?.name ?? "";
  const back = (
    <LinkButton to="/events" variant="ghost" icon="chevron-left">
      Back to events
    </LinkButton>
  );

  if (submission.error) {
    return (
      <ErrorState
        title="That project could not be loaded"
        body={submission.error}
        retry={
          <>
            <RetryButton onRetry={submission.reload} />
            {back}
          </>
        }
      />
    );
  }
  // Nothing has settled: an unset resource is the first attempt whether or not
  // the loading flag has flipped yet, so the skeleton covers both.
  if (!submission.data) return <Skeleton lines={6} />;
  // A settled answer with no project in it. The old code returned null here,
  // which left a blank page with no way back to the gallery.
  if (!project) {
    return (
      <EmptyState
        title="That project is not available"
        body="It may have been removed since the page was loaded, or the link may be out of date."
        icon="folder-kanban"
        action={back}
      />
    );
  }

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
                  <span className="visually-hidden">(opens in new tab)</span>
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
                  <span className="visually-hidden">(opens in new tab)</span>
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
      {back}
    </section>
  );
}

function EventDirectory() {
  // The event list is fetched once for the whole document, so the directory and
  // the gallery's picker read the same request rather than issuing their own.
  const { events, pending, error, empty, reload } = useEvents();

  return (
    <div className="stack stack-8">
      <PageHead
        title="Events"
        lede="Every event on this portal, with its state, deadline and submission count."
      />
      {/* Exactly one of the four: a failure with a retry, the first load, the
          portal's own "none yet", or the list. */}
      {error ? (
        <ErrorState
          title="Events could not be loaded"
          body={error}
          retry={<RetryButton onRetry={reload} />}
        />
      ) : pending ? (
        <Skeleton lines={4} />
      ) : empty ? (
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
    </div>
  );
}

function EventApp({ slug }: { slug: string }) {
  const loaded = useAsync(() => api.event(slug), [slug]);

  if (loaded.error) {
    return (
      <ErrorState
        title="That event could not be loaded"
        body={loaded.error}
        retry={<RetryButton onRetry={loaded.reload} />}
      />
    );
  }
  if (!loaded.data) return <Skeleton lines={5} />;

  // The related collections are siblings of `data`, not fields inside it. Reading
  // event.tracks found nothing, and the count below threw on the first line.
  const event = loaded.data.data;
  const tracks = loaded.data.tracks;
  const prizes = loaded.data.prizes;
  const milestones = loaded.data.milestones;

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
                <time dateTime={event.submissions_close}>
                  {new Date(event.submissions_close).toLocaleDateString()}
                </time>
              </DescriptionValue>
              <DescriptionTerm>Tracks</DescriptionTerm>
              <DescriptionValue>{tracks.length}</DescriptionValue>
              <DescriptionTerm>Prizes</DescriptionTerm>
              <DescriptionValue>{prizes.length}</DescriptionValue>
              <DescriptionTerm>Judges</DescriptionTerm>
              <DescriptionValue>{loaded.data.judge_count}</DescriptionValue>
            </DescriptionList>
          </Card>
        </div>
      </div>

      {/* Each of the three states below stands in for its section rather than
          disappearing: a viewer who sees nothing cannot tell "no prizes" from
          "prizes further down the page", which is what design.md 12 means by a
          clear empty state. */}
      {tracks.length > 0 ? (
        <section className="stack stack-4">
          <CardTitle>Tracks</CardTitle>
          <div className="cluster">
            {tracks.map((track) => (
              <Badge key={track.id} icon="route">
                {track.name}
              </Badge>
            ))}
          </div>
        </section>
      ) : (
        <EmptyState
          title="No tracks yet"
          body="Submissions are grouped by track when the event defines them. This one has none listed."
          icon="route"
        />
      )}

      {milestones.length > 0 ? (
        <section className="stack stack-4">
          <CardTitle>Schedule</CardTitle>
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
                  <Th scope="row">{milestone.title}</Th>
                  <Td meta>{milestone.detail ?? "—"}</Td>
                  <Td numeric>
                    {milestone.due_at ? (
                      <time dateTime={milestone.due_at}>
                        {new Date(milestone.due_at).toLocaleDateString()}
                      </time>
                    ) : (
                      "—"
                    )}
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        </section>
      ) : (
        <EmptyState
          title="No schedule yet"
          body="Milestones appear here once the organizers set the dates that shape the event."
          icon="calendar"
        />
      )}

      {prizes.length > 0 ? (
        <section className="stack stack-4">
          <CardTitle>Prizes</CardTitle>
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
                  <Th scope="row">{prize.name}</Th>
                  <Td meta>{prize.description || "—"}</Td>
                  <Td numeric>{ordinal(prize.rank)}</Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        </section>
      ) : (
        <EmptyState
          title="No prizes yet"
          body="The panel's awards are listed here once the event publishes them."
          icon="trophy"
        />
      )}
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
  const eventList = useEvents();

  const query = searchParams.get("q") ?? "";
  // The route's own event beats the portal's first event, so a gallery reached
  // from an event page shows that event rather than whichever sorts first. `||`
  // rather than `??`, because /events/gallery names no event and an empty string
  // is a value here, not an absent one.
  const eventSlugParam = searchParams.get("event") || eventSlug || eventList.events[0]?.slug || "";
  const track = searchParams.get("track") ?? "";
  const page = Math.max(1, Number(searchParams.get("page") ?? "1") || 1);

  // Typed but not yet applied. A request and a history entry per keystroke makes
  // the server chase the typist and fills the back button with half-typed queries.
  const [typed, setTyped] = useState(query);

  // The address is the source of truth, so the field follows it whenever
  // something else writes the query — Back, a shared link, "Clear filters". The
  // debounce below only ever *applies* typed; without this re-sync it also
  // re-applied it, overwriting the query that Back had just restored.
  useEffect(() => {
    setTyped(query);
  }, [query]);

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

  // The data is retained across a page or filter change, so the results block —
  // including the pagination control the viewer is standing on — keeps its
  // mounted instance and its focus instead of unmounting on every refetch.
  const projects = useAsync(
    () =>
      api.projects(eventSlugParam, {
        ...(query ? { q: query } : {}),
        ...(track ? { track } : {}),
        page,
      }),
    [eventSlugParam, query, track, page],
    // With no event named there is nothing to ask for, and the answer would be
    // an error where the truth is "this portal has no events yet".
    { enabled: Boolean(eventSlugParam) },
  );

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

  // The picker is a real labelled control, and it always has an option that
  // matches the value it holds — including the moment before the list arrives.
  const eventOptions = eventList.events.map((event) => ({
    value: event.slug,
    label: event.name,
  }));
  if (!eventOptions.some((option) => option.value === eventSlugParam)) {
    eventOptions.unshift({
      value: eventSlugParam,
      label: eventSlugParam || (eventList.pending ? "Loading events" : "No events"),
    });
  }

  // The results region answers with exactly one state. `projects.data` being
  // non-null means a request has succeeded, so a refresh keeps the current grid
  // (and the focus inside it) on screen rather than replacing it with a
  // skeleton, and the first load is the only time a skeleton appears here.
  let results: ReactNode;
  if (!eventSlugParam) {
    // No event to browse: the event list decides, and only one of its three
    // answers renders — never a skeleton beside the "no events" empty state.
    if (eventList.error) {
      results = (
        <ErrorState
          title="The event list could not be loaded"
          body={eventList.error}
          retry={<RetryButton onRetry={eventList.reload} />}
        />
      );
    } else if (eventList.pending) {
      results = <Skeleton lines={4} />;
    } else {
      results = (
        <EmptyState
          title="There are no events to browse"
          body="A gallery belongs to an event, and this portal has none yet. Once one is created its projects appear here."
          icon="calendar"
        />
      );
    }
  } else if (projects.error) {
    results = (
      <ErrorState
        title="The gallery could not be loaded"
        body={projects.error}
        retry={<RetryButton onRetry={projects.reload} />}
      />
    );
  } else if (!projects.data) {
    results = <Skeleton lines={4} />;
  } else if (projects.data.data.length === 0) {
    results = (
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
    );
  } else {
    const list = projects.data.data;
    const total = projects.data.meta.total;
    const pageSize = projects.data.meta.page_size || PAGE_SIZE;
    const pageCount = Math.max(1, Math.ceil(total / pageSize));
    results = (
      <>
        {/* A live region: the count is the one thing that changes when a page
            or a search changes, so a screen reader hears that the results moved
            instead of being left on the previous set. */}
        <p className="table__meta" role="status">
          {total === 1 ? "1 project" : `${total} projects`}
          {total > pageSize
            ? ` · showing ${(page - 1) * pageSize + 1}–${Math.min(page * pageSize, total)}`
            : null}
        </p>
        <div className="gallery-grid">
          {list.map((project) => (
            <ProjectCard key={project.id} project={project} signedIn={Boolean(session.user)} />
          ))}
        </div>
        <Pagination
          page={page}
          pageCount={pageCount}
          onPage={(next) => apply({ page: next > 1 ? next : undefined })}
        />
      </>
    );
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
          value={typed}
          onChange={setTyped}
          placeholder="Title, summary or tag"
        />
        <Select
          id="gallery-event"
          label="Event"
          value={eventSlugParam}
          onChange={(value) => apply({ event: value, page: undefined })}
          options={eventOptions}
        />
      </div>

      {/* The picker reads the same list as the results. When an event is named
          by the route the gallery still loads, and the picker's own failure is
          reported rather than leaving a select with no options to explain it. */}
      {eventSlugParam && eventList.error ? (
        <ErrorState
          title="The event list could not be loaded"
          body={eventList.error}
          retry={<RetryButton onRetry={eventList.reload} />}
        />
      ) : null}

      {results}
    </div>
  );
}

function ProjectCard({ project, signedIn }: { project: Project; signedIn: boolean }) {
  return (
    // A card that only *looks* clickable must have something to click. When the
    // viewer is signed out there is no link at all — the project route belongs
    // to a signed-in visitor — so the hover affordance is dropped rather than
    // promising a destination the keyboard cannot reach.
    <Card interactive={signedIn}>
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
