/**
 * The typed API client.
 *
 * Two things are deliberate here.
 *
 * First, credentials. The portal authenticates with a session cookie, so every
 * request is sent with `credentials: "include"`. In development the Vite proxy
 * makes the API same-origin, so there is no cross-origin cookie problem to
 * configure, and in production everything is one origin anyway.
 *
 * Second, the error type. The backend returns a structured error body, and
 * `ApiError` keeps the code so a handler can distinguish "not permitted" from
 * "that does not exist" without matching on a message. A component that has to
 * parse an error string to decide what to show is a component that breaks when
 * the wording improves.
 */

export interface ApiErrorBody {
  error?: {
    code?: string;
    message?: string;
  };
}

/** An error the backend reported, with its code preserved. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }

  /** True when the caller lacks the permission, as distinct from it being absent. */
  get isForbidden(): boolean {
    return this.status === 401 || this.status === 403;
  }

  get isNotFound(): boolean {
    return this.status === 404;
  }

  /**
   * A 412 is the concurrency guard answering: the resource changed since the
   * client read it. It is distinguished from other failures because the fix is
   * different — re-read and merge, rather than retry.
   */
  get isConflict(): boolean {
    return this.status === 409 || this.status === 412;
  }
}

/**
 * The API root.
 *
 * Every route is registered under /v1 on the server and the client is written
 * without the prefix, so there is one place to change and one place for the
 * contract test to compare. The client and the server must not both know the
 * prefix independently, or a change to one silently breaks the other.
 */
const BASE = "/v1";

interface RequestOptions {
  method?: string;
  body?: unknown;
  /** An ETag to assert with a write, for the optimistic concurrency guard. */
  ifMatch?: string;
  /** An Idempotency-Key, so a retry cannot apply a write twice. */
  idempotencyKey?: string;
  signal?: AbortSignal;
  /**
   * Called with the ETag the response carried, if any.
   *
   * request() used to return only the parsed body, so the ETag the server sets
   * on every read and every successful write was discarded. The If-Match option
   * existed and was therefore always undefined, which made the server's 412
   * guard unreachable from the product: two tabs scoring the same project
   * silently overwrote each other, which is the exact loss the guard exists to
   * prevent. A callback rather than a second return value, so every existing
   * call site keeps its type.
   */
  onEtag?: (etag: string | undefined) => void;
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, ifMatch, idempotencyKey, signal, onEtag } = options;
  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (ifMatch) headers["If-Match"] = ifMatch;
  if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;

  const response = await fetch(`${BASE}${path}`, {
    method,
    headers,
    credentials: "include",
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
    ...(signal ? { signal } : {}),
  });

  if (!response.ok) {
    let code = "error";
    let message = response.statusText || "The request failed";
    try {
      const parsed = (await response.json()) as ApiErrorBody;
      code = parsed.error?.code ?? code;
      message = parsed.error?.message ?? message;
    } catch {
      // A non-JSON error body is still an error; the status carries the meaning.
    }
    throw new ApiError(response.status, code, message);
  }

  onEtag?.(response.headers.get("ETag") ?? undefined);

  if (response.status === 204) return undefined as T;

  // A 2xx that is not JSON is a contract violation, not a body. It used to be
  // handed straight to response.json(), which threw a bare SyntaxError that
  // carried no status and no code, so a request that reached the app's
  // client-route fallback instead of the API surfaced as "Unexpected token <
  // in <!doctype" with nothing to grep for.
  const contentType = response.headers.get("Content-Type") ?? "";
  if (!contentType.includes("json")) {
    throw new ApiError(
      response.status,
      "invalid_response",
      `the server returned ${contentType || "an unlabelled body"} where JSON was expected`,
    );
  }
  return (await response.json()) as T;
}




/** The portal's JSON API, grouped the way the applications consume it. */
export const api = {
  // ---- session and account
  // GET /v1/me is the session identity. It is deliberately not a "whoami"
  // profile endpoint: the answer is "which account is this cookie", and nothing
  // more, so it cannot be used to read another account.
  me: () => request<{ data: SessionUser }>("/me"),
  signIn: (email: string, password: string) =>
    request<{ data: { user: SessionUser } }>("/auth/login", {
      method: "POST",
      body: { email, password },
    }),

  // ---- events
  events: () => request<{ data: EventSummary[] }>("/events"),
  // The related collections are siblings of `data`, not fields inside it. They
  // were declared nested, so `event.tracks` was undefined and every read of it
  // threw on the public event page — the one page every visitor lands on.
  event: (slug: string) =>
    request<{
      data: EventDetail;
      tracks: Track[];
      prizes: Prize[];
      milestones: Milestone[];
      questions: HackathonQuestion[];
      hosts: HackathonHost[];
      rubric: Rubric | null;
      team_policy: TeamPolicy;
      judge_count: number;
    }>(`/events/${slug}`),
  projects: (slug: string, query: ProjectQuery = {}) => {
    const params = new URLSearchParams();
    if (query.track) params.set("track", query.track);
    if (query.q) params.set("q", query.q);
    if (query.page) params.set("page", String(query.page));
    const search = params.toString();
    // meta.total, not count. The count field did not exist, so the pagination
    // control had nothing to page against and every project past the first
    // page of 24 was unreachable.
    return request<{ data: Project[]; meta: { page: number; page_size: number; total: number } }>(
      `/events/${slug}/projects${search ? `?${search}` : ""}`,
    );
  },

  // ---- account
  profile: () => request<{ data: AccountRecord }>("/account/profile"),
  // Two separate bugs in one call. The body sent display_name, headline,
  // organization and bio, and the server's decoder sets DisallowUnknownFields,
  // so the whole request was rejected with a 400 naming "headline" — the profile
  // editor could not save at all. And the response was declared as an
  // AccountRecord while the handler returns the updated user, so the editor read
  // response.data.profile, got undefined, and stayed on its skeleton forever.
  updateProfile: (patch: ProfilePatch) =>
    request<{ data: SessionUser }>("/account/profile", { method: "PATCH", body: patch }),
  // The caller's own teams ride on the profile read rather than on a separate
  // listing. A "teams I am in" endpoint that returned a platform-wide list and
  // expected the client to filter it would ship every membership in the portal
  // to the browser, which is exactly the pattern the event rules call out.
  //
  sessions: () => request<{ data: SessionSummary[] }>("/account/sessions"),

  // ---- submissions
  // The submission is the project, and the team, versions and flags are
  // siblings of it. It was declared as {submission, versions, flags} inside
  // data, so data.submission did not exist.
  submission: (id: string) =>
    request<{ data: Submission; team: Team | null; versions: Version[]; flags: Flag[] }>(
      `/submissions/${id}`,
    ),
  updateSubmission: (id: string, patch: Partial<Submission>, ifMatch?: string) =>
    request<{ data: Submission }>(`/submissions/${id}`, {
      method: "PATCH",
      body: patch,
      ...(ifMatch ? { ifMatch } : {}),
    }),
  submitProject: (id: string, ifMatch?: string) =>
    request<{ data: Submission }>(`/submissions/${id}/submit`, {
      method: "POST",
      ...(ifMatch ? { ifMatch } : {}),
    }),

  // ---- judging
  myAssignments: (slug: string) =>
    request<{ data: Assignment[]; pending: number; count: number }>(
      `/judge/assignments?event_slug=${slug}`,
    ),
  rubric: (eventID: string) => request<{ data: Rubric }>(`/judging/rubric?event_id=${eventID}`),
  saveReview: (
    projectID: string,
    payload: { event_id: string; criteria: Record<string, number>; comment?: string; submitted?: boolean },
    ifMatch?: string,
  ) =>
    request<{ data: Review }>(`/judge/projects/${projectID}/review`, {
      method: "PUT",
      body: payload,
      ...(ifMatch ? { ifMatch } : {}),
    }),
  // Registered as PUT only, so this GET was answered by the app's client-route
  // fallback: 200, text/html, and response.json() threw a bare SyntaxError that
  // no catch treated as an ApiError. The judge scoring page had no error state
  // for it and showed a permanent skeleton. The server route now exists.
  myReview: (projectID: string, onEtag?: (etag: string | undefined) => void) =>
    request<{ data: Review | null }>(`/judge/projects/${projectID}/review`, { onEtag }),

  // ---- organizer
  // These three took a slug where the handler read an id, and results declared
  // a row array where the handler returns a judging summary. The event picker
  // was a no-op: every panel showed the default event, and rows.map threw.
  progress: (slug: string) => request<{ data: Progress }>(`/organizer/progress?event_slug=${slug}`),
  panel: (slug: string) => request<{ data: JudgeRow[] }>(`/organizer/panel?event_slug=${slug}`),
  results: (slug: string) =>
    request<{ data: ResultsSummary; rubric: Rubric | null }>(`/organizer/results?event_slug=${slug}`),
  // Without a body the handler reads public=false, so the leaderboard stayed
  // 403 to anonymous visitors while the button said it had published them.
  publishResults: (slug: string) =>
    request<{ data: Event }>(`/organizer/events/${slug}/publish-results`, {
      method: "POST",
      body: { public: true },
    }),

  // ---- audit and grants, the accountability surface
  audit: (query: AuditQuery = {}) => {
    const params = new URLSearchParams();
    if (query.eventID) params.set("event_id", query.eventID);
    if (query.action) params.set("action", query.action);
    if (query.actorID) params.set("actor_id", query.actorID);
    if (query.limit) params.set("limit", String(query.limit));
    const search = params.toString();
    return request<AuditResponse>(`/audit/actions${search ? `?${search}` : ""}`);
  },
  verifyAudit: () => request<{ data: AuditVerification }>("/audit/verify"),
  grants: (query: GrantQuery = {}) => {
    const params = new URLSearchParams();
    if (query.userID) params.set("user_id", query.userID);
    if (query.eventID) params.set("event_id", query.eventID);
    const search = params.toString();
    return request<GrantResponse>(`/grants${search ? `?${search}` : ""}`);
  },
  createGrant: (grant: NewGrant) => request<{ data: Grant }>("/grants", { method: "POST", body: grant }),
  revokeGrant: (id: string) => request<{ data: { id: string; revoked: boolean } }>(`/grants/${id}`, { method: "DELETE" }),

  // ---- administration of accounts
  //
  // These used to be a bare `fetch("/v1/admin/users")` in the admin app, which
  // re-declared the API prefix the client exists to keep in exactly one place,
  // and returned a plain Error that the shared error copy never saw. They are
  // here so the response shape, the credentials and the error type are the same
  // as every other call.
  adminUsers: () => request<{ data: AdminUser[] }>("/admin/users"),
  adminSetUserState: (userID: string, state: AdminUserState, reason: string) =>
    request<{ data: AdminUser }>(`/admin/users/${encodeURIComponent(userID)}/state`, {
      method: "PATCH",
      body: { state, reason },
    }),
  adminSetUserRole: (userID: string, role: string, reason: string) =>
    request<{ data: AdminUser }>(`/admin/users/${encodeURIComponent(userID)}/role`, {
      method: "PUT",
      body: { role, reason },
    }),
  adminRevokeUserSessions: (userID: string, reason: string) =>
    request<{ data: { revoked: number } }>(
      `/admin/users/${encodeURIComponent(userID)}/sessions/revoke`,
      { method: "POST", body: { reason } },
    ),

  // ---- comparisons, the first-class pairwise question
  comparisons: (slug: string) => request<ComparisonResponse>(`/events/${slug}/comparisons`),
  recordComparison: (slug: string, comparison: NewComparison) =>
    request<{ data: Comparison }>(`/events/${slug}/comparisons`, { method: "POST", body: comparison }),
};

// ---- the shapes the UI reads ---------------------------------------------

export interface SessionUser {
  id: string;
  email: string;
  display_name: string;
  role: "visitor" | "participant" | "judge" | "organizer" | "admin";
  state?: string;
}

export interface Membership {
  team_id: string;
  event_id: string;
  role: string;
}

/** The caller's own account record, as GET /v1/account/profile returns it. */
export interface AccountRecord {
  user: SessionUser;
  profile: Profile;
  memberships: Membership[];
  participations: { event_id: string; scope: string; registered_at: string }[];
}

export interface EventSummary {
  id: string;
  slug: string;
  name: string;
  summary?: string;
  state: string;
  submissions_open: boolean;
  submissions_close: string;
  project_count?: number;
  judge_count?: number;
}

export interface EventDetail extends EventSummary {
  description: string;
  timezone: string;
  prizes: { id: string; name: string; description?: string }[];
  milestones: { id: string; title: string; detail?: string; due_at?: string }[];
  tracks: { id: string; name: string; slug: string }[];
}

/**
 * A project, which on the wire is a domain.Submission.
 *
 * This had two interfaces — Project and Submission — describing the same object,
 * and they drifted: one declared track_name, rank and repository_url, which the
 * server never sends, so every badge derived from them rendered permanently
 * blank while the compiler agreed with both.
 */
export interface Project {
  id: string;
  event_id: string;
  team_id: string;
  track_id: string;
  title: string;
  summary: string;
  description: string;
  story: string;
  repo_url: string;
  live_url: string;
  video_url: string;
  thumbnail_url: string;
  tags: string[];
  status: string;
  eligibility: string;
  submitted_at: string;
  updated_at: string;
  version: number;
}

export interface ProjectQuery {
  track?: string;
  q?: string;
  page?: number;
}

export type Submission = Project;

// ---- shapes the server sends, transcribed from backend/internal/domain
//
// These were absent while being referenced, which is how the mismatches above
// compiled at all. Each field name below is the JSON tag on the corresponding Go
// struct.

export interface Track {
  id: string;
  event_id: string;
  name: string;
  slug: string;
  summary?: string;
  order: number;
}

export interface Prize {
  id: string;
  event_id: string;
  track_id?: string;
  name: string;
  description: string;
  rank: number;
}

export interface Milestone {
  id: string;
  event_id: string;
  title: string;
  detail?: string;
  due_at: string;
  position: number;
}

export interface HackathonQuestion {
  id: string;
  event_id: string;
  key: string;
  prompt: string;
  help_text?: string;
  type: string;
  audience: string;
  required: boolean;
  options?: string[];
}

export interface HackathonHost {
  id: string;
  event_id: string;
  name: string;
  url?: string;
  logo_url?: string;
}

export interface TeamPolicy {
  min_size: number;
  max_size: number;
  allow_solo: boolean;
  requires_approval: boolean;
}

/** The fields PATCH /v1/account/profile accepts. Nothing else is accepted. */
export interface ProfilePatch {
  display_name?: string;
  avatar_url?: string;
  bio?: string;
  organization?: string;
  timezone?: string;
  locale?: string;
}

export interface Version {
  id: string;
  submission_id: string;
  version: number;
  title: string;
  summary: string;
  reason: string;
  created_at: string;
}

export interface Flag {
  id: string;
  event_id: string;
  project_id: string;
  duplicate_of_project_id: string;
  kind: string;
  note?: string;
  created_at: string;
}

export interface SubmissionDetail {
  submission: Submission;
  versions: Version[];
  flags: Flag[];
}

export interface Team {
  id: string;
  name: string;
  event_id: string;
  role: string;
  member_count: number;
}

/**
 * The account's own profile, as GET /v1/account/profile returns it.
 *
 * This was declared with a `headline` field, which does not exist. `headline`
 * belongs to a judge's UserProfile; an account has a display name, which is what
 * appears next to a review. The mismatch is why the profile editor had to be
 * reworked around display_name.
 */
export interface Profile {
  user_id: string;
  display_name?: string;
  bio?: string;
  organization?: string;
  timezone?: string;
  locale?: string;
  avatar_url?: string;
  headline?: string;
  participations?: Membership[];
  teams?: Membership[];
}

export interface SessionSummary {
  id: string;
  created_at: string;
  last_seen_at: string;
  current?: boolean;
}

export interface Assignment {
  id: string;
  project_id: string;
  project_title: string;
  event_id: string;
  event_slug: string;
  review_id?: string;
  submitted?: boolean;
  started?: boolean;
  has_conflict?: boolean;
}

export interface Rubric {
  id: string;
  version: number;
  // min_score/max_score, not min/max. Array.from({length: NaN}) produced no
  // options, so every criterion rendered an empty select and a judge could not
  // enter a single score.
  criteria: {
    key: string;
    label: string;
    weight: number;
    min_score: number;
    max_score: number;
    required: boolean;
    description?: string;
  }[];
}

export interface Review {
  id: string;
  project_id: string;
  judge_id: string;
  criteria: Record<string, number>;
  comment: string;
  submitted: boolean;
  updated_at: string;
}

export interface Progress {
  event_id: string;
  submissions: number;
  eligible_submissions: number;
  assignments: number;
  reviews_started: number;
  reviews_completed: number;
  reviews_pending: number;
}

export interface JudgeRow {
  id: string;
  display_name: string;
  email: string;
  assigned: number;
  started: number;
  completed: number;
  has_conflict: boolean;
  active: boolean;
}

// The wire names. normalized_score/raw_score/reviews did not exist: the
// server sends normalized_mean, raw_mean and review_count, plus a confidence
// interval on low and high. A component reading the old names rendered blank
// cells for every project, which is what a rankings table looks like when it is
// quietly wrong rather than loudly broken.
export interface ResultRow {
  project_id: string;
  raw_mean: number;
  normalized_mean: number;
  low: number;
  high: number;
  review_count: number;
  low_information_reviews: number;
  rank: number;
  separable: boolean;
  tie_group: number;
  previous_rank: number;
}

/** The whole fit, which is what the results endpoint returns under `data`. */
export interface ResultsSummary {
  method: string;
  method_version: number;
  confidence: number;
  resamples: number;
  reviews: number;
  projects: ResultRow[];
  low_information_judges: string[];
  judges: unknown[];
  criterion_stats: unknown[];
}

export interface AuditEntry {
  seq: number;
  created_at: string;
  actor_id: string;
  actor_role: string;
  action: string;
  event_id?: string;
  target_type?: string;
  target_id?: string;
  allowed: boolean;
  source: string;
  reason: string;
  method: string;
  path: string;
  status: number;
  request_id?: string;
  hash: string;
  prev_hash: string;
}

/** The states the backend will accept for an account, from domain.AccountState. */
export type AdminUserState =
  | "pending"
  | "active"
  | "suspended"
  | "deactivated"
  | "locked"
  | "deletion_pending";

export interface AdminUser {
  id: string;
  email: string;
  display_name: string;
  role: string;
  state: AdminUserState | string;
  created_at: string;
}

export interface AuditResponse {
  data: AuditEntry[];
  count: number;
  total: number;
  head: string;
  dropped?: number;
}

export interface AuditVerification {
  entries: number;
  valid: boolean;
  broken_at_seq?: number;
  detail?: string;
  head: string;
  dropped?: number;
}

export interface AuditQuery {
  eventID?: string;
  action?: string;
  actorID?: string;
  limit?: number;
}

export interface Grant {
  id: string;
  user_id: string;
  action: string;
  event_id?: string;
  object_id?: string;
  allow: boolean;
  reason: string;
  expires_at?: string;
  granted_by: string;
  created_at: string;
}

export interface GrantResponse {
  data: Grant[];
  count: number;
}

export interface GrantQuery {
  userID?: string;
  eventID?: string;
}

export interface NewGrant {
  user_id: string;
  action: string;
  event_id?: string;
  allow: boolean;
  reason: string;
  expires_at?: string;
}

export interface Comparison {
  id: string;
  event_id: string;
  judge_id: string;
  left: string;
  right: string;
  verdict: "left" | "right" | "tie";
  comment?: string;
  created_at: string;
  updated_at: string;
}

export interface ComparisonResponse {
  data: Comparison[];
  count: number;
}

export interface NewComparison {
  left: string;
  right: string;
  verdict: "left" | "right" | "tie";
  comment?: string;
}
