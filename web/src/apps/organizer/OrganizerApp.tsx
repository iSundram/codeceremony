import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Route, Routes, useSearchParams } from "react-router-dom";

import { Alert, Badge, EmptyState, ErrorState, Modal, Progress, Skeleton, Status } from "../../components/feedback";
import { Button, Field, LinkButton, SearchField, Select } from "../../components/controls";
import { PageHead, TabLink, Tabs } from "../../components/shell";
import {
  Avatar,
  Card,
  CardBody,
  CardTitle,
  InlineCode,
  StatCard,
  StatGrid,
  Table,
  Td,
  Th,
  Tr,
} from "../../components/data";
import {
  api,
  ApiError,
  type AuditEntry,
  type AuditVerification,
  type EventSummary,
  type Grant,
  type JudgeRow,
  type Progress as EventProgress,
  type Project,
  type ResultsSummary,
} from "../../lib/api";
import { isStaff, useSession } from "../../lib/session";

/**
 * The organizer app.
 *
 * An organizer runs an event: they watch the panel work, they publish results,
 * and they answer for the trail afterwards. The audit and grants surfaces live
 * here rather than behind the admin group, because an event organizer needs them
 * for their own event and a platform admin is not always present.
 */
export function OrganizerApp() {
  const session = useSession();
  if (session.loading) return <Skeleton lines={5} />;
  if (!session.user) {
    return (
      <Alert kind="info" title="Sign in to organize">
        Progress, results and the audit trail belong to an organizer account.
      </Alert>
    );
  }
  if (!isStaff(session.user)) {
    // The sidebar already hid this group. Saying so is clearer than a blank
    // page, and the backend refuses these routes for the same reason.
    return (
      <Alert kind="warning" title="This section is for organizers and admins">
        Your role is {session.user.role}. The backend refuses these routes whatever the navigation
        shows.
      </Alert>
    );
  }
  return (
    <Routes>
      <Route index element={<ProgressApp />} />
      <Route path="submissions" element={<SubmissionsApp />} />
      <Route path="panel" element={<PanelApp />} />
      <Route path="results" element={<ResultsApp />} />
      <Route path="audit" element={<AuditApp />} />
      <Route path="grants" element={<GrantsApp />} />
      <Route path="*" element={<EmptyState title="No such organizing page" body="That address does not match an organizing view." icon="clipboard-list" />} />
    </Routes>
  );
}

function OrganizerTabs({ active }: { active: string }) {
  return (
    <Tabs>
      <TabLink to="/organizer" label="Progress" active={active === "progress"} icon="gauge" />
      <TabLink to="/organizer/submissions" label="Submissions" active={active === "submissions"} icon="folder-kanban" />
      <TabLink to="/organizer/panel" label="Panel" active={active === "panel"} icon="users" />
      <TabLink to="/organizer/results" label="Results" active={active === "results"} icon="trophy" />
      <TabLink to="/organizer/audit" label="Audit" active={active === "audit"} icon="shield" />
      <TabLink to="/organizer/grants" label="Grants" active={active === "grants"} icon="key" />
    </Tabs>
  );
}

function useEvent() {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [eventsError, setEventsError] = useState<string | null>(null);
  const [params, setParams] = useSearchParams();
  const eventSlug = params.get("event") ?? events[0]?.slug ?? "";
  useEffect(() => {
    let live = true;
    api
      .events()
      .then((response) => {
        if (!live) return;
        setEvents(response.data);
        setEventsError(null);
      })
      // Swallowing this left eventSlug empty, so every view below ran its
      // effect, got nothing, and sat on its skeleton for the life of the page
      // with nothing to tell the organizer why.
      .catch((caught) => live && setEventsError(describe(caught)));
    return () => {
      live = false;
    };
  }, []);
  return { events, eventSlug, setParams, eventsError };
}

/**
 * Project titles for the standings.
 *
 * A results row is keyed by project_id and carries no title, so the names come
 * from the project listing. That listing is paged at 24 a page and a leaderboard
 * routinely carries more rows than one page returns, so every page is read
 * rather than the first.
 */
async function loadProjectTitles(slug: string): Promise<Map<string, string>> {
  const titles = new Map<string, string>();
  const first = await api.projects(slug);
  for (const project of first.data) titles.set(project.id, project.title);
  const pages = Math.ceil(first.meta.total / Math.max(1, first.meta.page_size));
  const rest = await Promise.all(
    Array.from({ length: Math.max(0, pages - 1) }, (_, index) => api.projects(slug, { page: index + 2 })),
  );
  for (const response of rest) {
    for (const project of response.data) titles.set(project.id, project.title);
  }
  return titles;
}

function EventPicker({
  events,
  eventSlug,
  setParams,
}: {
  events: EventSummary[];
  eventSlug: string;
  setParams: (params: URLSearchParams) => void;
}) {
  if (events.length <= 1) return null;
  return (
    <Select
      id="organizer-event"
      label="Event"
      value={eventSlug}
      onChange={(value) => {
        const merged = new URLSearchParams();
        merged.set("event", value);
        setParams(merged);
      }}
      options={events.map((event) => ({ value: event.slug, label: event.name }))}
    />
  );
}

/**
 * The progress dashboard.
 *
 * T2 asks for a live view of who has not started. A bar per judge says it more
 * honestly than a single percentage, because a panel where 80% of reviews are in
 * can be one person holding everything up or thirty people moving evenly, and
 * those need different interventions.
 */
function ProgressApp() {
  const { events, eventSlug, setParams, eventsError } = useEvent();
  const [progress, setProgress] = useState<EventProgress | null>(null);
  const [panel, setPanel] = useState<JudgeRow[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!eventSlug) return;
    let live = true;
    setProgress(null);
    setError(null);
    void (async () => {
      try {
        const [loadedProgress, loadedPanel] = await Promise.all([
          api.progress(eventSlug),
          api.panel(eventSlug),
        ]);
        if (!live) return;
        setProgress(loadedProgress.data);
        setPanel(loadedPanel.data);
      } catch (caught) {
        if (live) setError(describe(caught));
      }
    })();
    return () => {
      live = false;
    };
  }, [eventSlug]);

  // Above every return. It used to sit below `if (error) return`, so the first
  // failed fetch changed the hook count between renders and React unmounted the
  // whole tree with "Rendered fewer hooks than expected" — and with no error
  // boundary above it, the organizer got a blank page instead of a message.
  const notStarted = useMemo(
    () => panel.filter((judge) => judge.assigned > 0 && judge.started === 0),
    [panel],
  );

  return (
    <div className="stack stack-6">
      <PageHead
        title="Progress"
        lede="How much of the panel has been assigned, started and finished."
        actions={
          <LinkButton to="/organizer/panel" variant="secondary" icon="users">
            The panel
          </LinkButton>
        }
      />
      <EventPicker events={events} eventSlug={eventSlug} setParams={setParams} />
      <OrganizerTabs active="progress" />

      {eventsError ? <ErrorState title="The event list could not be loaded" body={eventsError} /> : null}
      {error ? <ErrorState title="Progress could not be loaded" body={error} /> : null}
      {!progress && !error && !eventsError ? <Skeleton lines={4} /> : null}

      {progress ? (
        <>
          <StatGrid>
            <StatCard label="Submissions" value={progress.submissions} meta={`${progress.eligible_submissions} eligible`} icon="folder-kanban" />
            <StatCard label="Assignments" value={progress.assignments} meta="Judge to project" icon="clipboard-list" />
            <StatCard label="Completed" value={progress.reviews_completed} meta="Submitted and locked" icon="circle-check" />
            <StatCard
              label="Pending"
              value={progress.reviews_pending}
              meta={progress.reviews_pending === 0 ? "Nothing outstanding" : "Not yet submitted"}
              icon="clock"
            />
          </StatGrid>

          <Card>
            <CardTitle>Reviews completed</CardTitle>
            <Progress label="Panel progress" value={progress.reviews_completed} total={progress.assignments} />
          </Card>

          {notStarted.length > 0 ? (
            <Alert kind="warning" title={`${notStarted.length} judges have not started`}>
              {notStarted.map((judge) => judge.display_name).join(", ")}. A judge who has not opened a
              single assignment is usually a scheduling problem rather than a judging one, and it is
              worth a message rather than a reminder.
            </Alert>
          ) : null}

          <PanelTable panel={panel} />
        </>
      ) : null}
    </div>
  );
}

function PanelTable({ panel }: { panel: JudgeRow[] }) {
  if (panel.length === 0) {
    return (
      <EmptyState
        title="No judges on this panel"
        body="A panel is built from the event's judge roster. Nobody has been added yet."
        icon="users"
      />
    );
  }
  return (
    <Table caption="Every judge on the panel, with their workload and how far through it they are.">
      <thead>
        <tr>
          <Th>Judge</Th>
          <Th numeric>Assigned</Th>
          <Th numeric>Started</Th>
          <Th numeric>Completed</Th>
          <Th>State</Th>
        </tr>
      </thead>
      <tbody>
        {panel.map((judge) => (
          <Tr key={judge.id}>
            <Td strong>
              <span className="cluster cluster-2">
                <Avatar name={judge.display_name} />
                {judge.display_name}
              </span>
            </Td>
            <Td numeric>{judge.assigned}</Td>
            <Td numeric>{judge.started}</Td>
            <Td numeric>{judge.completed}</Td>
            <Td>
              {judge.has_conflict ? (
                <Badge icon="circle-alert" variant="outline">
                  Conflict declared
                </Badge>
              ) : judge.completed === judge.assigned && judge.assigned > 0 ? (
                <Status icon="circle-check">Finished</Status>
              ) : judge.started === 0 ? (
                <Status icon="clock" muted>
                  Not started
                </Status>
              ) : (
                <Status icon="pencil">In progress</Status>
              )}
            </Td>
          </Tr>
        ))}
      </tbody>
    </Table>
  );
}

function SubmissionsApp() {
  const { events, eventSlug, setParams, eventsError } = useEvent();
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [query, setQuery] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!eventSlug) return;
    let live = true;
    setProjects(null);
    setError(null);
    api
      .projects(eventSlug, query ? { q: query } : {})
      .then((response) => live && setProjects(response.data))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, [eventSlug, query]);

  return (
    <div className="stack stack-6">
      <PageHead title="Submissions" lede="Every project submitted to this event." />
      <EventPicker events={events} eventSlug={eventSlug} setParams={setParams} />
      <OrganizerTabs active="submissions" />
      <SearchField id="submission-search" label="Search" value={query} onChange={setQuery} />
      {eventsError ? <ErrorState title="The event list could not be loaded" body={eventsError} /> : null}
      {error ? <ErrorState title="Submissions could not be loaded" body={error} /> : null}
      {!projects && !error && !eventsError ? <Skeleton lines={4} /> : null}
      {projects ? (
        projects.length === 0 ? (
          <EmptyState title="Nothing submitted" body="No project matches that search." icon="folder-kanban" />
        ) : (
          <Table caption={`${projects.length} projects.`}>
            <thead>
              <tr>
                <Th>Project</Th>
                <Th>Track</Th>
                <Th>Team</Th>
                <Th>Status</Th>
              </tr>
            </thead>
            <tbody>
              {projects.map((project) => (
                <Tr key={project.id}>
                  <Td strong>{project.title}</Td>
                  <Td meta>{project.track_name ?? project.track_id}</Td>
                  <Td meta>{project.team_name ?? "—"}</Td>
                  <Td>
                    <Status icon={project.status === "submitted" ? "circle-check" : "pencil"}>
                      {project.status}
                    </Status>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )
      ) : null}
    </div>
  );
}

function PanelApp() {
  const { events, eventSlug, setParams, eventsError } = useEvent();
  const [panel, setPanel] = useState<JudgeRow[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!eventSlug) return;
    let live = true;
    setPanel(null);
    setError(null);
    api
      .panel(eventSlug)
      .then((response) => live && setPanel(response.data))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, [eventSlug]);

  return (
    <div className="stack stack-6">
      <PageHead title="The panel" lede="Who is judging, what they carry, and what is outstanding." />
      <EventPicker events={events} eventSlug={eventSlug} setParams={setParams} />
      <OrganizerTabs active="panel" />
      {eventsError ? <ErrorState title="The event list could not be loaded" body={eventsError} /> : null}
      {error ? <ErrorState title="The panel could not be loaded" body={error} /> : null}
      {!panel && !error && !eventsError ? <Skeleton lines={4} /> : null}
      {panel ? <PanelTable panel={panel} /> : null}
    </div>
  );
}

/**
 * The standings.
 *
 * The endpoint returns a judging summary rather than a row array: the method
 * that produced it, the whole fit, and the per-project rows under `projects`.
 * It was read as a row array, so `rows.map` never ran and every cell was blank.
 */
function ResultsApp() {
  const { events, eventSlug, setParams, eventsError } = useEvent();
  const [summary, setSummary] = useState<ResultsSummary | null>(null);
  const [titles, setTitles] = useState<Map<string, string>>(new Map());
  const [error, setError] = useState<string | null>(null);
  const [publishError, setPublishError] = useState<string | null>(null);
  const [published, setPublished] = useState(false);
  const [publishing, setPublishing] = useState(false);
  // A ref as well as the state: the disabled button stops the second click
  // after React has re-rendered, but two clicks inside one batch both read the
  // same stale state, and publishing mails every team on the event.
  const publishingRef = useRef(false);

  useEffect(() => {
    if (!eventSlug) return;
    let live = true;
    setSummary(null);
    setTitles(new Map());
    setError(null);
    void (async () => {
      try {
        const [results, projectTitles] = await Promise.all([
          api.results(eventSlug),
          // A project list that fails must not blank the standings, so a row
          // falls back to its own id rather than to a missing title.
          loadProjectTitles(eventSlug).catch(() => new Map<string, string>()),
        ]);
        if (!live) return;
        setSummary(results.data);
        setTitles(projectTitles);
      } catch (caught) {
        if (live) setError(describe(caught));
      }
    })();
    return () => {
      live = false;
    };
  }, [eventSlug]);

  async function publish() {
    if (publishingRef.current) return;
    publishingRef.current = true;
    setPublishing(true);
    setPublishError(null);
    setPublished(false);
    try {
      await api.publishResults(eventSlug);
      setPublished(true);
    } catch (caught) {
      setPublishError(describe(caught));
    } finally {
      publishingRef.current = false;
      setPublishing(false);
    }
  }

  const rows = summary?.projects ?? [];

  return (
    <div className="stack stack-6">
      <PageHead
        title="Results"
        lede="The normalized standings. Publishing makes them public and cannot be undone from here."
      />
      <EventPicker events={events} eventSlug={eventSlug} setParams={setParams} />
      <OrganizerTabs active="results" />

      {publishError ? (
        <Alert kind="error" title="Publishing failed">
          {publishError}
        </Alert>
      ) : null}
      {published ? (
        <Alert kind="success" title="Results published">
          The leaderboard is public and the teams have been notified. The publication is in the
          audit trail.
        </Alert>
      ) : null}

      {eventsError ? <ErrorState title="The event list could not be loaded" body={eventsError} /> : null}
      {error ? <ErrorState title="Results could not be loaded" body={error} /> : null}
      {!summary && !error && !eventsError ? <Skeleton lines={5} /> : null}

      {summary ? (
        <>
          <Alert kind="info" title={`Computed by ${summary.method}`}>
            Every judge is calibrated against themselves and shrunk toward the panel before the
            aggregate, so one lenient judge cannot move the standings. JUDGING.md explains why, and
            shows what happens to a judge who rates everything identically.
          </Alert>
          {rows.length === 0 ? (
            <EmptyState
              title="Nothing has been scored yet"
              body="No review has been submitted for this event, so there are no standings to show. The table appears as soon as one judge locks a score."
              icon="trophy"
            />
          ) : (
            <Table caption={`${rows.length} projects, from ${summary.reviews} reviews. A low-information review is flagged rather than dropped.`}>
              <thead>
                <tr>
                  <Th numeric>Rank</Th>
                  <Th>Project</Th>
                  <Th numeric>Normalized</Th>
                  <Th numeric>Raw</Th>
                  <Th numeric>Reviews</Th>
                  <Th>Notes</Th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <Tr key={row.project_id}>
                    <Td numeric>{row.rank}</Td>
                    <Td strong>{titles.get(row.project_id) ?? row.project_id}</Td>
                    <Td numeric>
                      <span className="stack stack-1">
                        {row.normalized_mean.toFixed(1)}
                        <span className="table__meta">
                          {`${row.low.toFixed(1)}–${row.high.toFixed(1)}`}
                        </span>
                      </span>
                    </Td>
                    <Td numeric>{row.raw_mean.toFixed(1)}</Td>
                    <Td numeric>{row.review_count}</Td>
                    <Td>
                      <span className="stack stack-1">
                        {row.low_information_reviews > 0 ? (
                          <Badge icon="circle-alert" variant="outline">
                            Low information
                          </Badge>
                        ) : row.separable ? (
                          <Badge icon="circle-check" variant="outline">
                            Separable
                          </Badge>
                        ) : (
                          <Status icon="minus" muted>
                            Overlapping
                          </Status>
                        )}
                        {/* The rank this project failed to beat, so an overlap is
                            a statement about one pair rather than about the
                            column. */}
                        {row.separable || row.previous_rank === 0 ? null : (
                          <span className="table__meta">Did not beat rank {row.previous_rank}</span>
                        )}
                      </span>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
          {rows.length === 0 ? null : (
            <div className="actions">
              <Button
                variant="secondary"
                icon="megaphone"
                type="button"
                loading={publishing}
                onClick={() => void publish()}
              >
                Publish results
              </Button>
              <span className="table__meta">
                Publishing is recorded in the audit trail and notifies the teams.
              </span>
            </div>
          )}
        </>
      ) : null}
    </div>
  );
}

/**
 * The audit surface.
 *
 * The chain is verified on demand rather than trusted. A hash chain that is only
 * ever written and never checked is a log with extra steps, and the verification
 * endpoint is what makes the export something a third party can check.
 */
function AuditApp() {
  const { events, eventSlug, setParams, eventsError } = useEvent();
  // The listing is held with the event it belongs to, so switching event cannot
  // leave the previous event's decisions on screen under the new event's name.
  // Changing the action filter does not blank it, because typing in a search
  // field should not replace the results with a skeleton on every keystroke.
  const [listing, setListing] = useState<{ eventSlug: string; entries: AuditEntry[] } | null>(null);
  const [verification, setVerification] = useState<AuditVerification | null>(null);
  const [action, setAction] = useState("");
  const [onlyDenied, setOnlyDenied] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    setError(null);
    void (async () => {
      try {
        const eventID = events.find((event) => event.slug === eventSlug)?.id;
        const [loaded, verified] = await Promise.all([
          api.audit({ ...(eventID ? { eventID } : {}), ...(action ? { action } : {}), limit: 100 }),
          api.verifyAudit(),
        ]);
        if (!live) return;
        setListing({ eventSlug, entries: loaded.data });
        setVerification(verified.data);
      } catch (caught) {
        if (live) setError(describe(caught));
      }
    })();
    return () => {
      live = false;
    };
  }, [events, eventSlug, action]);

  const entries = listing && listing.eventSlug === eventSlug ? listing.entries : null;

  const shown = useMemo(
    () => (entries ?? []).filter((entry) => (onlyDenied ? !entry.allowed : true)),
    [entries, onlyDenied],
  );

  return (
    <div className="stack stack-6">
      <PageHead
        title="Audit trail"
        lede="Every authorization decision, including the refusals, in a hash-chained log."
      />
      <EventPicker events={events} eventSlug={eventSlug} setParams={setParams} />
      <OrganizerTabs active="audit" />

      {verification ? (
        verification.valid ? (
          <Alert kind="success" title="Chain verified">
            {`${verification.entries} entries reconcile against the genesis hash${
              verification.dropped ? `, with ${verification.dropped} older entries trimmed by retention` : ""
            }. The head is `}
            <InlineCode>{`${verification.head.slice(0, 16)}…`}</InlineCode>
          </Alert>
        ) : (
          <Alert kind="error" title="Chain verification failed">
            {verification.detail} at sequence {verification.broken_at_seq}. An entry has been edited
            or removed since it was written.
          </Alert>
        )
      ) : null}

      <div className="audit-filters">
        <SearchField
          id="audit-action"
          label="Filter by action"
          placeholder="results.read"
          value={action}
          onChange={setAction}
        />
        <label className="check">
          <input
            type="checkbox"
            checked={onlyDenied}
            onChange={(event) => setOnlyDenied(event.target.checked)}
          />
          <span className="check__text">Refusals only</span>
        </label>
      </div>

      {eventsError ? <ErrorState title="The event list could not be loaded" body={eventsError} /> : null}
      {error ? <ErrorState title="The audit trail could not be loaded" body={error} /> : null}
      {!entries && !error && !eventsError ? <Skeleton lines={5} /> : null}

      {entries ? (
        shown.length === 0 ? (
          <EmptyState
            title="No entries match"
            body="Nothing in the log matches that action, or nothing has been refused yet."
            icon="shield"
          />
        ) : (
          <Table caption="Newest first. Each row carries the hash of the one before it.">
            <thead>
              <tr>
                <Th numeric>Seq</Th>
                <Th>Actor</Th>
                <Th>Action</Th>
                <Th>Outcome</Th>
                <Th>Reason</Th>
                <Th>When</Th>
              </tr>
            </thead>
            <tbody>
              {shown.map((entry) => (
                <Tr key={entry.seq}>
                  <Td numeric>{entry.seq}</Td>
                  <Td>
                    <span className="cluster cluster-2">
                      <Avatar name={entry.actor_id || "system"} />
                      {entry.actor_id || "anonymous"}
                    </span>
                  </Td>
                  <Td>
                    <InlineCode>{entry.action}</InlineCode>
                  </Td>
                  <Td>
                    {/* design.md 10.3: outcome is a word and a glyph. A red cell
                        would be the colour-only status this contract forbids. */}
                    {entry.allowed ? (
                      <Status icon="circle-check">Allowed</Status>
                    ) : (
                      <Status icon="ban">Refused</Status>
                    )}
                  </Td>
                  <Td meta>{entry.reason || "—"}</Td>
                  <Td meta>{new Date(entry.created_at).toLocaleString()}</Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )
      ) : null}

      {entries ? (
        <details className="audit-detail">
          <summary>What the chain does and does not prove</summary>
          <CardBody>
            Each entry carries the hash of the entry before it and an HMAC over its own contents, so
            an edit or a removal breaks the chain from that point on. What a third party can check
            without holding the key is that the log is internally consistent: every row&apos;s
            prev_hash matches the row above. What only the key holder can prove is that a row&apos;s
            contents are unedited, because the chain is keyed with HMAC rather than a published
            signature. When retention trims old entries the remainder verifies as a suffix, and the
            dropped count is reported so a truncated log is not passed off as a whole one.
          </CardBody>
        </details>
      ) : null}
    </div>
  );
}

/**
 * Explicit grants.
 *
 * A deny is evaluated before any allow, so this is the one place where adding a
 * record takes a permission away as well as giving one. That is deliberate: an
 * incident response should be one API call, not a role change and a wait for the
 * next session to expire.
 */
function GrantsApp() {
  const { events, eventSlug, setParams, eventsError } = useEvent();
  const [grants, setGrants] = useState<Grant[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [userID, setUserID] = useState("");
  const [action, setAction] = useState("results.read");
  const [allow, setAllow] = useState(true);
  const [reason, setReason] = useState("");
  // The revoke is irreversible, so the row that is about to be revoked is held
  // rather than acted on, and the confirmation names it.
  const [revoking, setRevoking] = useState<Grant | null>(null);
  const [revokeError, setRevokeError] = useState<string | null>(null);
  const [revokeBusy, setRevokeBusy] = useState(false);
  // A ref as well as the state: the disabled button only stops the second click
  // once React has re-rendered, and two clicks in one batch read the same value.
  const revokingRef = useRef(false);

  async function load(live: () => boolean = () => true) {
    try {
      const eventID = events.find((event) => event.slug === eventSlug)?.id;
      const response = await api.grants(eventID ? { eventID } : {});
      if (!live()) return;
      setGrants(response.data);
      setError(null);
    } catch (caught) {
      if (live()) setError(describe(caught));
    }
  }

  useEffect(() => {
    // Without this guard a slow response for the event that was selected a
    // moment ago lands after the newer one and replaces it with its own list.
    let live = true;
    setGrants(null);
    setError(null);
    void load(() => live);
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [eventSlug, events]);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setFormError(null);
    try {
      const eventID = events.find((event) => event.slug === eventSlug)?.id;
      await api.createGrant({
        user_id: userID,
        action,
        allow,
        reason,
        ...(eventID ? { event_id: eventID } : {}),
      });
      setReason("");
      await load();
    } catch (caught) {
      setFormError(describe(caught));
    } finally {
      setBusy(false);
    }
  }

  async function revoke() {
    if (!revoking || revokingRef.current) return;
    revokingRef.current = true;
    setRevokeBusy(true);
    setRevokeError(null);
    try {
      await api.revokeGrant(revoking.id);
      setRevoking(null);
      await load();
    } catch (caught) {
      // A refused revoke has to be visible: the row is still in force, and a
      // silent failure here would read as though the permission had gone.
      setRevokeError(describe(caught));
    } finally {
      revokingRef.current = false;
      setRevokeBusy(false);
    }
  }

  return (
    <div className="stack stack-6">
      <PageHead
        title="Grants"
        lede="Explicit allows and denies, per user, per action, per event. A deny beats any allow."
      />
      <EventPicker events={events} eventSlug={eventSlug} setParams={setParams} />
      <OrganizerTabs active="grants" />

      <Alert kind="info" title="A deny is evaluated before any allow">
        A grant can add a permission. Only a deny can take one away, which is what makes an incident
        response a single call: revoke is immediate, and it does not wait for a role change to
        propagate.
      </Alert>

      <Card>
        <CardTitle>Record a grant</CardTitle>
        {formError ? (
          <Alert kind="error" title="That was not recorded">
            {formError}
          </Alert>
        ) : null}
        <form className="stack stack-4" onSubmit={onSubmit}>
          <div className="form-grid">
            <Field id="grant-user" label="User id" hint="The account identifier, not an email.">
              {({ id }) => (
                <input
                  id={id}
                  className="input"
                  value={userID}
                  onChange={(event) => setUserID(event.target.value)}
                  required
                />
              )}
            </Field>
            <Field id="grant-action" label="Action" hint="From the action vocabulary, not free text.">
              {({ id }) => (
                <input
                  id={id}
                  className="input"
                  value={action}
                  onChange={(event) => setAction(event.target.value)}
                  required
                />
              )}
            </Field>
            <Field id="grant-allow" label="Effect" hint="Whether this grants or denies the action.">
              {({ id }) => (
                <select
                  id={id}
                  className="select"
                  value={allow ? "allow" : "deny"}
                  onChange={(event) => setAllow(event.target.value === "allow")}
                >
                  <option value="allow">Allow</option>
                  <option value="deny">Deny</option>
                </select>
              )}
            </Field>
            <Field
              id="grant-reason"
              label="Reason"
              hint="Required, and kept with the grant so a later reader knows why it exists."
            >
              {({ id }) => (
                <input
                  id={id}
                  className="input"
                  value={reason}
                  onChange={(event) => setReason(event.target.value)}
                  required
                />
              )}
            </Field>
          </div>
          <div className="actions">
            <Button variant="primary" icon="key" type="submit" loading={busy}>
              Record grant
            </Button>
          </div>
        </form>
      </Card>

      {eventsError ? <ErrorState title="The event list could not be loaded" body={eventsError} /> : null}
      {error ? <ErrorState title="Grants could not be loaded" body={error} /> : null}
      {revokeError ? (
        <Alert kind="error" title="That grant was not revoked">
          {revokeError} The grant is still in force.
        </Alert>
      ) : null}
      {!grants && !error && !eventsError ? <Skeleton lines={3} /> : null}

      {grants ? (
        grants.length === 0 ? (
          <EmptyState
            title="No explicit grants"
            body="Nothing is granted or denied by hand. Roles decide everything until you record something here."
            icon="key"
          />
        ) : (
          <Table caption="Explicit grants for this event.">
            <thead>
              <tr>
                <Th>User</Th>
                <Th>Action</Th>
                <Th>Effect</Th>
                <Th>Reason</Th>
                <Th>Expires</Th>
                <Th>Revoke</Th>
              </tr>
            </thead>
            <tbody>
              {grants.map((grant) => (
                <Tr key={grant.id}>
                  <Td strong>{grant.user_id}</Td>
                  <Td>
                    <InlineCode>{grant.action}</InlineCode>
                  </Td>
                  <Td>
                    {grant.allow ? (
                      <Status icon="circle-check">Allow</Status>
                    ) : (
                      <Status icon="ban">Deny</Status>
                    )}
                  </Td>
                  <Td meta>{grant.reason}</Td>
                  <Td meta>
                    {grant.expires_at ? new Date(grant.expires_at).toLocaleDateString() : "No expiry"}
                  </Td>
                  <Td>
                    <Button
                      variant="ghost"
                      icon="trash-2"
                      type="button"
                      onClick={() => {
                        setRevokeError(null);
                        setRevoking(grant);
                      }}
                    >
                      Revoke
                    </Button>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )
      ) : null}

      <Modal
        open={revoking !== null}
        title="Revoke this grant?"
        onClose={() => {
          if (!revokeBusy) setRevoking(null);
        }}
        closeLabel="Keep the grant"
        actions={
          <>
            <Button variant="tertiary" type="button" disabled={revokeBusy} onClick={() => setRevoking(null)}>
              Keep it
            </Button>
            <Button variant="ghost" icon="trash-2" type="button" loading={revokeBusy} onClick={() => void revoke()}>
              Revoke
            </Button>
          </>
        }
      >
        <p>
          {revoking ? (
            <>
              <InlineCode>{revoking.action}</InlineCode> for{" "}
              <strong>{revoking.user_id}</strong> will stop applying immediately. It cannot be
              restored from here, and the decision is recorded in the audit trail.
            </>
          ) : null}
        </p>
      </Modal>
    </div>
  );
}

function describe(caught: unknown): string {
  return caught instanceof ApiError ? caught.message : "The portal could not be reached.";
}
