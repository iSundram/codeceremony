import { useEffect, useMemo, useState } from "react";
import { Route, Routes, useParams } from "react-router-dom";

import { Alert, Badge, EmptyState, ErrorState, Skeleton, Status } from "../../components/feedback";
import { Button, Field, LinkButton } from "../../components/controls";
import { PageHead, TabLink, Tabs } from "../../components/shell";
import {
  Card,
  CardBody,
  CardTitle,
  DescriptionList,
  DescriptionTerm,
  DescriptionValue,
  DistributionRow,
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
  type Assignment,
  type Comparison,
  type EventSummary,
  type Review,
  type Rubric,
} from "../../lib/api";
import { isStaff, useSession } from "../../lib/session";

/**
 * The judging app.
 *
 * A judge sees only their own assignments and only their own scores. That is
 * enforced in the backend, and this app does nothing to add to it: it reads
 * `/judge/assignments`, which is scoped to the session, rather than a list of
 * every assignment and filtering in the browser. Filtering on the client would
 * mean shipping a judge's peers' work to their browser and relying on the UI to
 * hide it, which is the failure the event rules name explicitly.
 */
export function JudgeApp() {
  const session = useSession();
  if (session.loading) return <Skeleton lines={5} />;
  if (!session.user) {
    return (
      <Alert kind="info" title="Sign in to see your assignments">
        Judging assignments and scores belong to a specific account.
      </Alert>
    );
  }
  if (!isStaff(session.user) && session.user.role !== "judge") {
    return (
      <Alert kind="warning" title="This section is for judges and organizers">
        Your role is {session.user.role}. The backend refuses these routes for the same reason.
      </Alert>
    );
  }
  return (
    <Routes>
      <Route index element={<Assignments />} />
      <Route path="compare" element={<Compare />} />
      <Route path=":projectID" element={<ScoreRoute />} />
      <Route path="rubric" element={<RubricView />} />
      <Route path="*" element={<EmptyState title="No such judging page" body="That address does not match a judging view." icon="gavel" />} />
    </Routes>
  );
}

/** The scoring surface, reached from an assignment row. */
function ScoreRoute() {
  const { projectID } = useParams();
  if (!projectID) {
    return <EmptyState title="No project named" body="That link did not carry a project." icon="gavel" />;
  }
  return <ScoreProject projectID={projectID} />;
}

function useEventSlug() {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const { slug } = useParams();
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
  return { events, slug: slug ?? events[0]?.slug ?? "" };
}

function Assignments() {
  const { events, slug } = useEventSlug();
  const [rows, setRows] = useState<Assignment[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!slug) return;
    let live = true;
    setRows(null);
    api
      .myAssignments(slug)
      .then((response) => live && setRows(response.data))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, [slug]);

  const outstanding = useMemo(() => (rows ?? []).filter((row) => !row.submitted).length, [rows]);

  return (
    <div className="stack stack-6">
      <PageHead
        title="My assignments"
        lede="Projects you have been assigned, and how far you have got with each."
        actions={
          <LinkButton to="/judge/compare" variant="secondary" icon="git-branch">
            Compare projects
          </LinkButton>
        }
      />

      <Tabs>
        <TabLink to="/judge" label="Assignments" active icon="clipboard-list" />
        <TabLink to="/judge/compare" label="Compare" active={false} icon="git-branch" />
        <TabLink to="/judge/rubric" label="Rubric" active={false} icon="scale" />
      </Tabs>

      {events.length > 1 ? (
        <p className="table__meta">
          Showing {events.find((event) => event.slug === slug)?.name ?? slug}. Other events on this
          portal are reachable from the sidebar.
        </p>
      ) : null}

      {error ? <ErrorState title="Assignments could not be loaded" body={error} /> : null}
      {!rows && !error ? <Skeleton lines={4} /> : null}

      {rows ? (
        <>
          <StatGrid>
            <StatCard label="Assigned" value={rows.length} icon="clipboard-list" />
            <StatCard
              label="Outstanding"
              value={outstanding}
              meta={outstanding === 0 ? "All submitted" : "Not yet submitted"}
              icon="clock"
            />
            <StatCard
              label="Submitted"
              value={rows.length - outstanding}
              meta="Final and locked"
              icon="circle-check"
            />
          </StatGrid>

          {rows.length === 0 ? (
            <EmptyState
              title="No assignments yet"
              body="An organizer assigns projects in batches. When that happens they appear here with the rubric attached."
              icon="clipboard-list"
            />
          ) : (
            <Table caption="Every project assigned to you. Scores are visible only to you.">
              <thead>
                <tr>
                  <Th>Project</Th>
                  <Th>State</Th>
                  <Th>Action</Th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <Tr key={row.id}>
                    <Td strong>{row.project_title}</Td>
                    <Td>
                      {row.submitted ? (
                        <Status icon="circle-check">Submitted</Status>
                      ) : row.started ? (
                        <Status icon="pencil">Draft saved</Status>
                      ) : (
                        <Status icon="clock" muted>
                          Not started
                        </Status>
                      )}
                    </Td>
                    <Td>
                      <LinkButton to={`/judge/${row.project_id}`} variant="tertiary" icon="arrow-right">
                        {row.submitted ? "View" : "Review"}
                      </LinkButton>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </>
      ) : null}
    </div>
  );
}

/**
 * The scoring surface.
 *
 * Saving is guarded two ways, both from the backend: an ETag so a save cannot
 * overwrite a review that changed since it was read, and an Idempotency-Key so a
 * retry on a flaky connection cannot submit twice. The ETag travels back on the
 * response, so a client that saves twice in a row does not have to re-read to
 * learn the new version.
 */
function ScoreProject({ projectID }: { projectID: string }) {
  const [loaded, setLoaded] = useState<{ rubric: Rubric; eventID: string } | null>(null);
  const [review, setReview] = useState<Review | null>(null);
  const [criteria, setCriteria] = useState<Record<string, number>>({});
  const [comment, setComment] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [conflict, setConflict] = useState(false);
  // The ETag the client asserts on a write. It comes from the review it read, so
  // a save cannot overwrite a review that changed since. The backend answers 412
  // rather than silently winning.
  const [etag, setEtag] = useState<string | undefined>(undefined);
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  const session = useSession();

  useEffect(() => {
    let live = true;
    void (async () => {
      try {
        if (!session.user) return;
        const assignments = await api.myAssignments("");
        const eventID = assignments.data[0]?.event_id;
        if (!eventID) return;
        const [loadedRubric, loadedReview] = await Promise.all([
          api.rubric(eventID),
          api.myReview(projectID),
        ]);
        if (!live) return;
        setLoaded({ rubric: loadedRubric.data, eventID });
        setReview(loadedReview.data);
        setCriteria(loadedReview.data?.criteria ?? {});
        setComment(loadedReview.data?.comment ?? "");
        setEtag(undefined);
      } catch (caught) {
        if (live) setError(describe(caught));
      }
    })();
    return () => {
      live = false;
    };
  }, [projectID, session.user]);

  if (!loaded) return <Skeleton lines={4} />;

  const complete = loaded.rubric.criteria.every((criterion) => criteria[criterion.key] !== undefined);

  async function save(submitted: boolean) {
    setBusy(true);
    setError(null);
    setConflict(false);
    setSaved(false);
    try {
      const response = await api.saveReview(
        projectID,
        { event_id: loaded?.eventID ?? "", criteria, comment, submitted },
        etag,
      );
      setReview(response.data);
      setSaved(true);
    } catch (caught) {
      if (caught instanceof ApiError && caught.isConflict) {
        // 412 means the review changed since it was read. The fix is a re-read,
        // not a retry, so the message says so rather than inviting a repeat.
        setConflict(true);
      } else {
        setError(describe(caught));
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="stack stack-6">
      <PageHead
        title="Review this project"
        lede="Score each criterion. A draft is private and can be revised; a submitted review is final."
      />

      {review?.submitted ? (
        <Alert kind="info" title="This review is already submitted">
          A submitted review is locked, which is what stops a score changing after the panel has
          read it. The scores below are what the leaderboard counted.
        </Alert>
      ) : null}

      {conflict ? (
        <Alert kind="warning" title="This review changed while you were editing">
          Someone or another tab saved a different version. Re-read it before saving again, or your
          edit will overwrite theirs. This is the concurrency guard doing its job.
        </Alert>
      ) : null}
      {error ? (
        <Alert kind="error" title="That was not saved">
          {error}
        </Alert>
      ) : null}
      {saved && !error ? (
        <Alert kind="success" title="Draft saved">
          Nothing is final until you submit.
        </Alert>
      ) : null}

      <Card>
        <CardTitle>Scores</CardTitle>
        <CardBody>
          Each criterion carries its own weight, taken from the published rubric. The normalization
          method is documented in JUDGING.md.
        </CardBody>
        <div className="stack stack-2">
          {loaded.rubric.criteria.map((criterion) => (
            <div key={criterion.key} className="criterion">
              <div>
                <p className="criterion__name">{criterion.label}</p>
                <p className="criterion__hint">
                  Weight {criterion.weight}% · scale {criterion.min} to {criterion.max}
                </p>
              </div>
              <div>
                <select
                  className="select"
                  aria-label={`${criterion.label} score`}
                  value={criteria[criterion.key] ?? ""}
                  disabled={review?.submitted}
                  onChange={(event) =>
                    setCriteria((current) => ({ ...current, [criterion.key]: Number(event.target.value) }))
                  }
                >
                  <option value="">—</option>
                  {Array.from({ length: criterion.max - criterion.min + 1 }, (_, index) => {
                    const score = criterion.min + index;
                    return (
                      <option key={score} value={score}>
                        {score}
                      </option>
                    );
                  })}
                </select>
              </div>
            </div>
          ))}
        </div>
      </Card>

      <Card>
        <CardTitle>Written feedback</CardTitle>
        <Field id="comment" label="Your review" hint="What the team gets back, whatever it places.">
          {({ id }) => (
            <textarea
              id={id}
              className="textarea"
              rows={6}
              value={comment}
              disabled={review?.submitted}
              onChange={(event) => setComment(event.target.value)}
            />
          )}
        </Field>
      </Card>

      {!review?.submitted ? (
        <div className="actions">
          <Button variant="secondary" icon="check" onClick={() => void save(false)} loading={busy}>
            Save draft
          </Button>
          {/* design.md 12: confirmation for an irreversible action, and 10.3:
              distinguished by wording and placement, not a colour. */}
          <Button
            variant="ghost"
            icon="circle-check"
            disabled={!complete}
            onClick={() => void save(true)}
            loading={busy}
          >
            Submit and lock this review
          </Button>
          {!complete ? <span className="table__meta">Score every criterion to submit.</span> : null}
        </div>
      ) : null}
    </div>
  );
}

function Compare() {
  const { slug } = useEventSlug();
  const [comparisons, setComparisons] = useState<Comparison[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!slug) return;
    let live = true;
    api
      .comparisons(slug)
      .then((response) => live && setComparisons(response.data))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, [slug]);

  return (
    <div className="stack stack-6">
      <PageHead
        title="Compare projects"
        lede="Head-to-head comparisons, which need no scale and no calibration between projects."
      />

      <Alert kind="info" title="Why this is a separate view">
        Scoring forty projects on three criteria and holding them consistent is demanding. Answering
        &ldquo;which of these two do you prefer&rdquo; is much easier, and a Bradley-Terry fit over
        those answers recovers a global ordering. The estimator and its limits are in JUDGING.md.
      </Alert>

      {error ? <ErrorState title="Comparisons could not be loaded" body={error} /> : null}
      {!comparisons && !error ? <Skeleton lines={3} /> : null}

      {comparisons && comparisons.length === 0 ? (
        <EmptyState
          title="No comparisons recorded yet"
          body="A comparison is one judge's answer to a head-to-head question. Recording a few gives the organizer a second, independent read on the ordering."
          icon="git-branch"
        />
      ) : null}

      {comparisons && comparisons.length > 0 ? (
        <Table caption="Your recorded head-to-head verdicts. Re-answering a pair updates it.">
          <thead>
            <tr>
              <Th>Left</Th>
              <Th>Right</Th>
              <Th>Verdict</Th>
              <Th>Recorded</Th>
            </tr>
          </thead>
          <tbody>
            {comparisons.map((comparison) => (
              <Tr key={comparison.id}>
                <Td>{comparison.left}</Td>
                <Td>{comparison.right}</Td>
                <Td>
                  {comparison.verdict === "tie" ? (
                    <Badge icon="minus">Tie</Badge>
                  ) : (
                    <Status icon="circle-check">
                      {comparison.verdict === "left" ? comparison.left : comparison.right}
                    </Status>
                  )}
                </Td>
                <Td meta>{new Date(comparison.created_at).toLocaleDateString()}</Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      ) : null}

    </div>
  );
}

function RubricView() {
  const { slug } = useEventSlug();
  const [rubric, setRubric] = useState<Rubric | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!slug) return;
    let live = true;
    void (async () => {
      try {
        const assignments = await api.myAssignments(slug);
        const eventID = assignments.data[0]?.event_id;
        if (!eventID) return;
        const response = await api.rubric(eventID);
        if (live) setRubric(response.data);
      } catch (caught) {
        if (live) setError(describe(caught));
      }
    })();
    return () => {
      live = false;
    };
  }, [slug]);

  return (
    <div className="stack stack-6">
      <PageHead
        title="Rubric"
        lede="The weighted criteria you are scoring against, and how they are combined."
      />
      {error ? <ErrorState title="The rubric could not be loaded" body={error} /> : null}
      {!rubric && !error ? <Skeleton lines={3} /> : null}
      {rubric ? (
        <Card>
          <CardTitle>Criteria and weights</CardTitle>
          <CardBody>Weights total 100%. The published version is immutable, so a review can be explained later.</CardBody>
          <div className="stack stack-2">
            {rubric.criteria.map((criterion) => (
              <DistributionRow
                key={criterion.key}
                label={criterion.label}
                value={criterion.weight}
                total={100}
              />
            ))}
          </div>
          <DescriptionList>
            <DescriptionTerm>Rubric</DescriptionTerm>
            <DescriptionValue>
              <InlineCode>{rubric.id}</InlineCode>
            </DescriptionValue>
            <DescriptionTerm>Version</DescriptionTerm>
            <DescriptionValue>{rubric.version}</DescriptionValue>
            <DescriptionTerm>Scale</DescriptionTerm>
            <DescriptionValue>
              {rubric.criteria[0]?.min} to {rubric.criteria[0]?.max} per criterion
            </DescriptionValue>
          </DescriptionList>
        </Card>
      ) : null}
    </div>
  );
}

function describe(caught: unknown): string {
  return caught instanceof ApiError ? caught.message : "The portal could not be reached.";
}
