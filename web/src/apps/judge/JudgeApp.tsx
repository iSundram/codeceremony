import { useEffect, useMemo, useState } from "react";
import { Route, Routes, useParams } from "react-router-dom";

import {
  Alert,
  Badge,
  EmptyState,
  ErrorState,
  Modal,
  Skeleton,
  Status,
} from "../../components/feedback";
import { Button, Field, LinkButton, Select } from "../../components/controls";
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
  type NewComparison,
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
      <Route path="rubric" element={<RubricView />} />
      <Route path=":projectID" element={<ScoreRoute />} />
      <Route
        path="*"
        element={
          <EmptyState title="No such judging page" body="That address does not match a judging view." icon="gavel" />
        }
      />
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

interface EventContext {
  events: EventSummary[];
  /** The event being judged: the route's own, or the only one on the portal. */
  slug: string;
  /** True while the event list is still in flight. Distinct from `error`. */
  pending: boolean;
  error: string | null;
  retry: () => void;
}

/**
 * The event this judging session is about.
 *
 * `pending` and `error` are separate because callers need three states, not two:
 * still resolving is a skeleton, refused is an error, and there are no events at
 * all is neither. Collapsing the last two into "no slug" is what left a judge who
 * could not reach the portal staring at a skeleton with nothing to act on.
 */
function useEventSlug(): EventContext {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [pending, setPending] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reload, setReload] = useState(0);
  const { slug } = useParams();

  useEffect(() => {
    let live = true;
    setPending(true);
    setError(null);
    api
      .events()
      .then((response) => {
        if (!live) return;
        setEvents(response.data);
        setPending(false);
      })
      .catch((caught) => {
        if (!live) return;
        setError(describe(caught));
        setPending(false);
      });
    return () => {
      live = false;
    };
  }, [reload]);

  return {
    events,
    slug: slug ?? events[0]?.slug ?? "",
    pending,
    error,
    retry: () => setReload((value) => value + 1),
  };
}

function RetryButton({ onRetry }: { onRetry: () => void }) {
  return (
    <Button variant="secondary" icon="refresh-cw" onClick={onRetry}>
      Try again
    </Button>
  );
}

function Assignments() {
  const { events, slug, pending: eventsPending, error: eventsError, retry: retryEvents } =
    useEventSlug();
  const [rows, setRows] = useState<Assignment[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    if (!slug) return;
    let live = true;
    setRows(null);
    setError(null);
    api
      .myAssignments(slug)
      .then((response) => live && setRows(response.data))
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, [slug, reload]);

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

      {eventsError ? (
        <ErrorState
          title="The event list could not be loaded"
          body={eventsError}
          retry={<RetryButton onRetry={retryEvents} />}
        />
      ) : null}
      {error ? (
        <ErrorState
          title="Assignments could not be loaded"
          body={error}
          retry={<RetryButton onRetry={() => setReload((value) => value + 1)} />}
        />
      ) : null}
      {!rows && !error && !eventsError ? <Skeleton lines={4} /> : null}

      {rows && rows.length === 0 ? (
        <EmptyState
          title="Nothing assigned for this event"
          body="An organizer assigns projects in batches. You have no assignments here yet, so there is nothing to review and no scores to give."
          icon="clipboard-list"
        />
      ) : null}

      {!slug && !eventsError && !eventsPending && !rows ? (
        <EmptyState
          title="There are no events to judge"
          body="Judging is scoped to an event, and this portal has none yet. An organizer creates one and assigns you to it before there is anything here."
          icon="calendar"
        />
      ) : null}

      {rows && rows.length > 0 ? (
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
        </>
      ) : null}
    </div>
  );
}

/**
 * The scoring surface.
 *
 * A write is guarded from the backend by an ETag, so a save cannot overwrite a
 * review that changed since it was read. The token travels back on every
 * response, so a judge who saves twice in a row is not refused against their own
 * last write — and a judge in a second tab is, which is the whole point.
 */
function ScoreProject({ projectID }: { projectID: string }) {
  const { slug, error: eventsError, retry: retryEvents } = useEventSlug();
  const session = useSession();
  const [loaded, setLoaded] = useState<{ rubric: Rubric; eventID: string } | null>(null);
  const [review, setReview] = useState<Review | null>(null);
  const [criteria, setCriteria] = useState<Record<string, number>>({});
  const [comment, setComment] = useState("");
  // Two failures, not one. A failed load replaces the page; a failed save must
  // not, because design.md 12 keeps the judge's input when a write fails and the
  // scores they have typed are the thing they would lose.
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [conflict, setConflict] = useState(false);
  const [unassigned, setUnassigned] = useState(false);
  // The ETag the client asserts on a write. It comes from the review it read, so
  // a save cannot overwrite a review that changed since. The backend answers 412
  // rather than silently winning.
  const [etag, setEtag] = useState<string | undefined>(undefined);
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    if (!session.user || !slug) return;
    let live = true;
    setLoadError(null);
    setUnassigned(false);
    void (async () => {
      try {
        const assignments = await api.myAssignments(slug);
        if (!live) return;
        const eventID = assignments.data[0]?.event_id;
        // A judge with no assignment for this event has nothing to score. That is
        // a real state, not a load still in flight, so it is recorded rather than
        // returning early and leaving the skeleton up with no way out.
        if (!eventID) {
          setUnassigned(true);
          return;
        }
        // Captured into a local rather than setState'd from the callback, because
        // the callback fires inside the request and the effect may already be
        // torn down by the time the pair resolves.
        let readEtag: string | undefined;
        const [rubric, current] = await Promise.all([
          api.rubric(eventID),
          api.myReview(projectID, (value) => {
            readEtag = value;
          }),
        ]);
        if (!live) return;
        setLoaded({ rubric: rubric.data, eventID });
        setReview(current.data);
        setCriteria(current.data?.criteria ?? {});
        setComment(current.data?.comment ?? "");
        setEtag(readEtag);
      } catch (caught) {
        if (live) setLoadError(describe(caught));
      }
    })();
    return () => {
      live = false;
    };
  }, [projectID, session.user, slug, reload]);

  const head = (
    <PageHead
      title="Review this project"
      lede="Score each criterion. A draft is private and can be revised; a submitted review is final."
    />
  );

  if (eventsError) {
    return (
      <div className="stack stack-6">
        {head}
        <ErrorState
          title="This review could not be opened"
          body={eventsError}
          retry={<RetryButton onRetry={retryEvents} />}
        />
      </div>
    );
  }

  if (loadError) {
    return (
      <div className="stack stack-6">
        {head}
        <ErrorState
          title="This review could not be loaded"
          body={loadError}
          retry={<RetryButton onRetry={() => setReload((value) => value + 1)} />}
        />
      </div>
    );
  }

  if (unassigned) {
    return (
      <div className="stack stack-6">
        {head}
        <EmptyState
          title="This project is not assigned to you"
          body="The backend refuses a review for a project outside your batch, so there is nothing to score here. An organizer can assign it to you if it should be."
          icon="gavel"
          action={
            <LinkButton to="/judge" variant="secondary" icon="arrow-right">
              My assignments
            </LinkButton>
          }
        />
      </div>
    );
  }

  if (!loaded) return <Skeleton lines={4} />;

  // min_score/max_score, not min/max: the wire names are what the rubric
  // validator reads back, and an undefined bound makes every scale empty.
  // Only a required criterion has to be answered, which is also what the server
  // insists on, so gating the submit on all of them locked judges out.
  const incomplete = loaded.rubric.criteria.filter(
    (criterion) => criterion.required && criteria[criterion.key] === undefined,
  );
  const complete = incomplete.length === 0;

  const save = async (submitted: boolean) => {
    setBusy(true);
    setSaveError(null);
    setConflict(false);
    setSaved(false);
    try {
      const response = await api.saveReview(
        projectID,
        { event_id: loaded.eventID, criteria, comment, submitted },
        etag,
      );
      setReview(response.data);
      // A submitted review is not a draft, and the alert below says which it was.
      setSaved(!submitted);
      // The save's own ETag is the new version, but saveReview does not forward
      // the response header, so it is read back here. Without this the next save
      // in this tab asserts the version it has just replaced and the server
      // answers 412 against its own last write.
      let written: string | undefined;
      await api.myReview(projectID, (value) => {
        written = value;
      });
      setEtag(written);
    } catch (caught) {
      // 412 means the review changed since it was read. The fix is a re-read, not
      // a retry, so the message says so rather than inviting a repeat. A 409 is a
      // different thing entirely — a review the server has already locked — and
      // its own message is the one the judge needs.
      if (caught instanceof ApiError && caught.status === 412) {
        setConflict(true);
      } else {
        setSaveError(describe(caught));
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="stack stack-6">
      {head}

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
      {saveError ? (
        <Alert kind="error" title="That was not saved">
          {saveError}
        </Alert>
      ) : null}
      {saved && !saveError ? (
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
                  Weight {criterion.weight}% · scale {criterion.min_score} to {criterion.max_score}
                  {criterion.required ? " · required" : " · optional"}
                </p>
                {criterion.description ? (
                  <p className="criterion__hint">{criterion.description}</p>
                ) : null}
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
                  {Array.from({ length: criterion.max_score - criterion.min_score + 1 }, (_, index) => {
                    const score = criterion.min_score + index;
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
            onClick={() => setConfirming(true)}
          >
            Submit and lock this review
          </Button>
          {!complete ? (
            <span className="table__meta">
              Score {incomplete.map((criterion) => criterion.label).join(", ")} to submit.
            </span>
          ) : null}
        </div>
      ) : null}

      <Modal
        open={confirming}
        title="Submit and lock this review?"
        onClose={() => setConfirming(false)}
        closeLabel="Cancel submission"
        actions={
          <>
            <Button
              variant="tertiary"
              type="button"
              icon="x"
              onClick={() => setConfirming(false)}
              disabled={busy}
            >
              Keep editing
            </Button>
            <Button
              variant="ghost"
              type="button"
              icon="circle-check"
              onClick={() => {
                setConfirming(false);
                void save(true);
              }}
              loading={busy}
            >
              Submit and lock
            </Button>
          </>
        }
      >
        <p>
          Submitting is final. A submitted review cannot be edited afterwards, and the panel reads
          these scores as they stand. If you are not finished, close this and save a draft instead.
        </p>
      </Modal>
    </div>
  );
}

function Compare() {
  const { slug, error: eventsError, retry: retryEvents } = useEventSlug();
  const [comparisons, setComparisons] = useState<Comparison[] | null>(null);
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [left, setLeft] = useState("");
  const [right, setRight] = useState("");
  const [verdict, setVerdict] = useState<NewComparison["verdict"] | "">("");
  const [comment, setComment] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [recorded, setRecorded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    if (!slug) return;
    let live = true;
    setComparisons(null);
    setError(null);
    setFormError(null);
    setRecorded(false);
    // The form needs the assignments as well as the recorded verdicts: the
    // backend refuses a comparison of two projects the judge was not given, so
    // offering the whole gallery would only offer refusals.
    Promise.all([api.comparisons(slug), api.myAssignments(slug)])
      .then(([listed, mine]) => {
        if (!live) return;
        setComparisons(listed.data);
        setAssignments(mine.data);
      })
      .catch((caught) => live && setError(describe(caught)));
    return () => {
      live = false;
    };
  }, [slug, reload]);

  const titleOf = (projectID: string) =>
    assignments.find((row) => row.project_id === projectID)?.project_title ?? projectID;

  async function submit() {
    if (!left || !right) {
      setFormError("Choose two projects to compare.");
      return;
    }
    if (left === right) {
      setFormError("A project cannot be compared with itself. Choose a different second project.");
      return;
    }
    if (verdict === "") {
      setFormError("Choose which project you prefer, or that they tie.");
      return;
    }
    setBusy(true);
    setFormError(null);
    setRecorded(false);
    try {
      const response = await api.recordComparison(slug, {
        left,
        right,
        verdict,
        ...(comment.trim() ? { comment: comment.trim() } : {}),
      });
      // The pair is matched unordered on the server, so a judge who swaps the two
      // sides is revising their answer rather than adding a second one. The list
      // is keyed the same way so the two views cannot disagree.
      const created = response.data;
      setComparisons((current) => [
        created,
        ...(current ?? []).filter(
          (entry) =>
            !(
              (entry.left === created.left && entry.right === created.right) ||
              (entry.left === created.right && entry.right === created.left)
            ),
        ),
      ]);
      setRecorded(true);
      setComment("");
    } catch (caught) {
      setFormError(describe(caught));
    } finally {
      setBusy(false);
    }
  }

  const projects = assignments.map((row) => ({ value: row.project_id, label: row.project_title }));

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

      {eventsError ? (
        <ErrorState
          title="Comparisons could not be loaded"
          body={eventsError}
          retry={<RetryButton onRetry={retryEvents} />}
        />
      ) : null}
      {error ? (
        <ErrorState
          title="Comparisons could not be loaded"
          body={error}
          retry={<RetryButton onRetry={() => setReload((value) => value + 1)} />}
        />
      ) : null}
      {!comparisons && !error && !eventsError ? <Skeleton lines={3} /> : null}

      {comparisons && assignments.length >= 2 ? (
        <Card>
          <CardTitle>Record a comparison</CardTitle>
          <CardBody>
            Both projects must be in your own batch. Answering the same pair again replaces your
            earlier verdict rather than adding to it.
          </CardBody>
          <div className="stack stack-4">
            <div className="grid">
              <div className="col-6">
                <Select
                  id="comparison-left"
                  label="First project"
                  value={left}
                  onChange={setLeft}
                  options={[{ value: "", label: "Choose a project" }, ...projects]}
                />
              </div>
              <div className="col-6">
                <Select
                  id="comparison-right"
                  label="Second project"
                  value={right}
                  onChange={setRight}
                  options={[{ value: "", label: "Choose a project" }, ...projects]}
                />
              </div>
            </div>
            <Select
              id="comparison-verdict"
              label="Which one"
              value={verdict}
              onChange={(value) => setVerdict(value as NewComparison["verdict"] | "")}
              options={[
                { value: "", label: "Choose an answer" },
                { value: "left", label: left ? `I prefer ${titleOf(left)}` : "I prefer the first" },
                {
                  value: "right",
                  label: right ? `I prefer ${titleOf(right)}` : "I prefer the second",
                },
                { value: "tie", label: "They are equally good" },
              ]}
            />
            <Field id="comparison-comment" label="Why" hint="Optional. A line is enough.">
              {({ id }) => (
                <textarea
                  id={id}
                  className="textarea"
                  rows={3}
                  value={comment}
                  onChange={(event) => setComment(event.target.value)}
                />
              )}
            </Field>
            {formError ? (
              <Alert kind="error" title="That comparison was not recorded">
                {formError}
              </Alert>
            ) : null}
            {recorded && !formError ? (
              <Alert kind="success" title="Comparison recorded">
                It counts towards the pairwise fit as soon as the organizer reads results.
              </Alert>
            ) : null}
            <div className="actions">
              <Button
                variant="primary"
                icon="git-branch"
                onClick={() => void submit()}
                loading={busy}
                disabled={!left || !right || verdict === ""}
              >
                Record comparison
              </Button>
            </div>
          </div>
        </Card>
      ) : null}

      {comparisons && assignments.length < 2 ? (
        <EmptyState
          title="Nothing to compare yet"
          body="A comparison is between two of your own assignments. With fewer than two in your batch there is no pair to answer, and the form appears as soon as there is."
          icon="git-branch"
        />
      ) : null}

      {comparisons && assignments.length >= 2 && comparisons.length === 0 ? (
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
                <Td>{titleOf(comparison.left)}</Td>
                <Td>{titleOf(comparison.right)}</Td>
                <Td>
                  {comparison.verdict === "tie" ? (
                    <Badge icon="minus">Tie</Badge>
                  ) : (
                    <Status icon="circle-check">
                      {comparison.verdict === "left"
                        ? titleOf(comparison.left)
                        : titleOf(comparison.right)}
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
  const { slug, error: eventsError, retry: retryEvents } = useEventSlug();
  const [rubric, setRubric] = useState<Rubric | null>(null);
  const [unassigned, setUnassigned] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    if (!slug) return;
    let live = true;
    setError(null);
    setUnassigned(false);
    void (async () => {
      try {
        const assignments = await api.myAssignments(slug);
        if (!live) return;
        const eventID = assignments.data[0]?.event_id;
        // Without an assignment there is no rubric to score against, which is a
        // state to show rather than a load to wait on.
        if (!eventID) {
          setUnassigned(true);
          return;
        }
        const response = await api.rubric(eventID);
        if (live) setRubric(response.data);
      } catch (caught) {
        if (live) setError(describe(caught));
      }
    })();
    return () => {
      live = false;
    };
  }, [slug, reload]);

  const head = (
    <PageHead
      title="Rubric"
      lede="The weighted criteria you are scoring against, and how they are combined."
    />
  );

  if (eventsError) {
    return (
      <div className="stack stack-6">
        {head}
        <ErrorState
          title="The rubric could not be loaded"
          body={eventsError}
          retry={<RetryButton onRetry={retryEvents} />}
        />
      </div>
    );
  }

  if (error) {
    return (
      <div className="stack stack-6">
        {head}
        <ErrorState
          title="The rubric could not be loaded"
          body={error}
          retry={<RetryButton onRetry={() => setReload((value) => value + 1)} />}
        />
      </div>
    );
  }

  if (unassigned) {
    return (
      <div className="stack stack-6">
        {head}
        <EmptyState
          title="Nothing assigned for this event"
          body="The rubric is shown for the event you are judging, and you have no assignment there. An organizer assigns projects before there is anything to score."
          icon="scale"
        />
      </div>
    );
  }

  if (!rubric) return <Skeleton lines={3} />;

  const scale = rubric.criteria[0];

  return (
    <div className="stack stack-6">
      {head}
      <Card>
        <CardTitle>Criteria and weights</CardTitle>
        <CardBody>
          Weights total 100%. The published version is immutable, so a review can be explained
          later.
        </CardBody>
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
            {scale ? `${scale.min_score} to ${scale.max_score} per criterion` : "Not published"}
          </DescriptionValue>
        </DescriptionList>
      </Card>
    </div>
  );
}

function describe(caught: unknown): string {
  return caught instanceof ApiError ? caught.message : "The portal could not be reached.";
}
