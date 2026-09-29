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

const BASE = "/v1";

interface RequestOptions {
  method?: string;
  body?: unknown;
  /** An ETag to assert with a write, for the optimistic concurrency guard. */
  ifMatch?: string;
  /** An Idempotency-Key, so a retry cannot apply a write twice. */
  idempotencyKey?: string;
  signal?: AbortSignal;
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, ifMatch, idempotencyKey, signal } = options;
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

  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

/** The portal's JSON API, grouped the way the applications consume it. */
export const api = {
  // ---- session and account
  whoami: () => request<{ data: { user: SessionUser; teams: SessionTeam[] } }>("/account/whoami"),
  signIn: (email: string, password: string) =>
    request<{ data: { user: SessionUser } }>("/auth/login", {
      method: "POST",
      body: { email, password },
    }),

  // ---- events
  events: () => request<{ data: EventSummary[] }>("/events"),
  event: (slug: string) => request<{ data: EventDetail }>(`/events/${slug}`),
  projects: (slug: string, query: ProjectQuery = {}) => {
    const params = new URLSearchParams();
    if (query.track) params.set("track", query.track);
    if (query.q) params.set("q", query.q);
    if (query.page) params.set("page", String(query.page));
    const search = params.toString();
    return request<{ data: Project[]; count: number }>(`/events/${slug}/projects${search ? `?${search}` : ""}`);
  },

  // ---- account
  profile: () => request<{ data: Profile }>("/account/profile"),
  updateProfile: (patch: Partial<Profile>) =>
    request<{ data: Profile }>("/account/profile", { method: "PATCH", body: patch }),
  teams: () => request<{ data: Team[] }>("/teams"),
  sessions: () => request<{ data: SessionSummary[] }>("/account/sessions"),

  // ---- submissions
  submission: (id: string) => request<{ data: SubmissionDetail }>(`/submissions/${id}`),
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
  myAssignments: (slug: string) => request<{ data: Assignment[] }>(`/judge/assignments?event_slug=${slug}`),
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
  myReview: (projectID: string) => request<{ data: Review | null }>(`/judge/projects/${projectID}/review`),

  // ---- organizer
  progress: (slug: string) => request<{ data: Progress }>(`/organizer/progress?event_slug=${slug}`),
  panel: (slug: string) => request<{ data: JudgeRow[] }>(`/organizer/panel?event_slug=${slug}`),
  results: (slug: string) => request<{ data: ResultRow[]; method: string }>(`/organizer/results?event_slug=${slug}`),
  publishResults: (slug: string) =>
    request<{ data: { published: boolean } }>("/organizer/publish", { method: "POST", body: { event_slug: slug } }),

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

export interface SessionTeam {
  id: string;
  name: string;
  role: string;
  event_id: string;
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

export interface Project {
  id: string;
  title: string;
  summary: string;
  track_id: string;
  track_name?: string;
  team_name?: string;
  status: string;
  repository_url?: string;
  live_url?: string;
  tags?: string[];
  score?: number | null;
  rank?: number | null;
  low_information?: boolean;
  submitted_at?: string;
}

export interface ProjectQuery {
  track?: string;
  q?: string;
  page?: number;
}

export interface Submission {
  id: string;
  title: string;
  summary: string;
  description: string;
  version: number;
  status: string;
  track_id: string;
  team_id: string;
}

export interface SubmissionDetail {
  submission: Submission;
  versions: { version: number; reason: string; created_at: string }[];
  flags: { id: string; kind: string; note?: string }[];
}

export interface Team {
  id: string;
  name: string;
  event_id: string;
  role: string;
  member_count: number;
}

export interface Profile {
  user_id: string;
  headline?: string;
  bio?: string;
  organization?: string;
  timezone?: string;
  locale?: string;
  avatar_url?: string;
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
  criteria: { key: string; label: string; weight: number; min: number; max: number; description?: string }[];
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

export interface ResultRow {
  project_id: string;
  title: string;
  track_name?: string;
  normalized_score: number;
  raw_score: number;
  rank: number;
  reviews: number;
  low_information: boolean;
  separable: boolean;
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
