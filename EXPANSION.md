# CodeCeremony — Next Expansion Roadmap

<p align="center">
  <img src="docs/assets/codeceremony-logo.svg" alt="CodeCeremony" width="320" />
</p>

> **Superseded.** This is the pre-implementation roadmap, kept as a record of the
> planning. For what is actually built, read [`README.md`](README.md) and
> [`ARCHITECTURE.md`](ARCHITECTURE.md).

> **Status:** Active backend-first expansion plan. Frontend implementation is intentionally deferred.
>
> **Product goal:** Move CodeCeremony from a working hackathon submission/judging backend to a self-hostable event operating system with a polished operator console, participant workspace, judge console, public gallery, and first-party My Account center.
>
> **Read first:** `rules.md`, `FEATURES.md`, `ACCOUNT-MANAGEMENT.md`, `STRUCTURE.md`, and `design.md`.

## 1. North star

CodeCeremony should be more than a Devpost alternative. It should be the system a small organizer can run end to end:

- create an event without a database administrator;
- invite participants and judges;
- collect messy real-world submissions safely;
- make judging transparent, isolated, and auditable;
- publish results with confidence;
- preserve the record for future organizers;
- let the platform leave cleanly through export and documented APIs.

The product is successful when an organizer can go from an empty directory to a working event with one command, and when a judge can complete an assigned review without asking for help.

## 2. Expansion pillars

### Pillar A — Event operating system

- event templates and cloning;
- event lifecycle state machine;
- configurable tracks, prizes, deadlines, and custom questions;
- eligibility rules and reviewer notes;
- organizer branding and public microsite;
- announcements and activity timeline;
- reusable event archive and migration bundle;
- backup, restore, and retention controls.

### Pillar B — Participant workspace

- first-party My Account center;
- team invitations with explicit roles;
- captain, leader, and member permissions;
- submission drafts with autosave and version history;
- media handling and link validation;
- deadline countdown and eligibility explanation;
- duplicate detection and organizer resolution;
- submission status timeline;
- personal data export and deletion lifecycle.

### Pillar C — Judge console

- assignment inbox with balanced workload;
- conflict declaration before review;
- rubric versions and track-specific rubrics;
- keyboard-first review form;
- save, resume, validate, and lock review states;
- peer-score isolation enforced by the API;
- pairwise comparison mode using a Bradley–Terry style estimator;
- normalization explanation and low-information judge warnings;
- participation records and signed evidence;
- reviewer feedback after publication.

### Pillar D — Judging integrity

- transparent raw and normalized scores;
- deterministic tie-breaking;
- judge calibration diagnostics;
- outlier and disagreement detection;
- appeals and organizer resolution;
- immutable audit trail;
- threat model covering sybil votes, ballot stuffing, scraping, collusion, and deadline gaming;
- exportable calculation evidence.

### Pillar E — Public experience

- fast searchable gallery;
- project detail pages with media and links;
- track and tag filters;
- comments with moderation;
- authenticated, open-link, or token-based voting;
- randomized ballots and anti-abuse controls;
- hidden results until publication;
- public certificates and verifiable records;
- embeddable gallery widget.

### Pillar F — Platform and operations

- PostgreSQL persistence and migrations;
- documented REST API and OpenAPI contract;
- scoped API tokens;
- webhooks with signed deliveries;
- JSON/CSV import and export;
- structured logs, health, readiness, and backup instructions;
- import preview, validation, retry, and rollback;
- accessibility and performance budgets.

## 3. Devpost-level capability comparison

| Capability | Devpost-level expectation | CodeCeremony target |
|---|---|---|
| Registration | Email, profile, eligibility | Local account, state, recovery, sessions |
| Teams | Invite and membership | Member/leader/captain roles and audited promotion |
| Submissions | Rich project form and media | Drafts, versions, deadline lock, duplicate detection |
| Gallery | Search and project pages | Fast filters, accessible states, public/private boundaries |
| Judging | Rubric and reviewer workflow | Weighted rubrics, isolation, calibration, evidence |
| Results | Published ranking | Normalized, explainable, snapshot-based results |
| Voting | Community vote | Configurable, abuse-aware, hidden-result voting |
| Organizer ops | Dashboard and export | Progress, audit, imports, webhooks, archive |
| Portability | CSV and integrations | OpenAPI, scoped tokens, JSON/CSV, migration bundle |
| Adoption | Hosted platform | One-command self-hosting and no hosted dependency |

## 4. Prioritized expansion plan

### Tranche 1 — Product surface now

1. Next.js SPA shell with the locked CodeCeremony design system.
2. Public gallery connected to the Go API.
3. Organizer overview connected to progress/results endpoints.
4. Judge review entry connected to own-score and review endpoints.
5. My Account entry connected to profile, sessions, notifications, export, and deletion endpoints.
6. Responsive sidebar, top bar, loading, empty, error, and permission-denied states.
7. API base URL configuration that works locally and in Docker.

### Tranche 2 — Persistence and operability

1. PostgreSQL repository implementation behind the existing store boundary.
2. Migration runner and schema verification.
3. Session, account, team, notification, and audit persistence.
4. Deterministic fixture import.
5. Backup/restore documentation and smoke script.
6. Docker Compose with API, web, and database services.

### Tranche 3 — Organizer operations

1. Event creation wizard.
2. Track, prize, deadline, and custom-question management.
3. Team moderation and eligibility queue.
4. Assignment builder with workload preview.
5. Progress dashboard and missing-review views.
6. Results preview, publication, and snapshot history.
7. Import/export jobs with row-level errors.

### Tranche 4 — Judge and integrity depth

1. Full rubric builder and versioning UI.
2. Pairwise mode and Bradley–Terry estimator.
3. Normalization proof report.
4. Conflict and appeal workflow.
5. Signed participation records.
6. Threat-model document and abuse test suite.

### Tranche 5 — Public platform

1. Voting campaign builder.
2. Randomized ballots and rate limits.
3. Comment moderation console.
4. Certificates and public verification.
5. Embeddable gallery widget.
6. Public API documentation and webhook catalog.

## 5. Best-in-class differentiators

These are the features that make CodeCeremony more than a feature checklist:

- **Explainable results:** every result shows raw score, normalized score, rubric version, exclusions, and calculation timestamp.
- **Integrity by construction:** judges cannot see peers, participants cannot see results early, and every protected mutation is audited.
- **Real-world data tolerance:** duplicate submissions, incomplete reviews, low-variance judges, empty comments, and uneven workloads are first-class cases.
- **Operator portability:** an organizer can import existing data, export everything, and leave without a lock-in conversation.
- **Calm interface:** dense information is organized through hierarchy, spacing, and progressive disclosure rather than visual noise.
- **Fast judging:** keyboard navigation, saved drafts, batch assignment, and a review queue designed for thirty projects in one sitting.
- **One-command adoption:** no cloud account, hosted database, external auth provider, or mandatory API key.

## 6. Current frontend contract

The first frontend tranche must use only the component inventory and tokens in `design.md`.

Required surfaces:

- `AppShell`;
- `Sidebar`;
- `TopBar`;
- `PageHeader`;
- `Card`;
- `StatCard`;
- `Button`;
- `Badge`;
- `Status`;
- `Table`;
- `List`;
- `EmptyState`;
- `ErrorState`;
- `Skeleton`;
- `Input`;
- `Select`;
- `Tabs`;
- `Progress`;
- `Tooltip`;
- `Modal`;
- `Drawer`;
- `Toast`.

Forbidden in the first tranche:

- new component families;
- new colors or gradients;
- dark mode;
- invented logo or icon assets;
- hardcoded production data that hides API state;
- a mockup that replaces a working API path.

## 7. Quality bar

Every shipped surface must have:

- loading state;
- empty state;
- error state;
- permission-denied state;
- keyboard focus;
- mobile layout;
- readable contrast;
- no raw secret or token in the browser URL;
- API request correlation and consistent error handling;
- a documented data source.

Every major workflow must be demonstrable in the five-minute video:

1. start locally;
2. open the event;
3. browse the gallery;
4. submit or inspect a project;
5. score as a judge;
6. show isolation;
7. show organizer progress and results;
8. open My Account and manage a session.

## 8. Definition of “massive” without chaos

A large feature surface is healthy only when it is:

- **vertical:** one complete path from UI to API to persistence;
- **honest:** incomplete work is labeled and not claimed as finished;
- **composable:** shared primitives and contracts are reused;
- **observable:** failures, actions, and decisions are visible to operators;
- **portable:** no hidden hosted dependency;
- **accessible:** keyboard and contrast are part of done;
- **documented:** behavior is written down as it is built.

The expansion is not an excuse to abandon the acceptance path. T1/T2 correctness remains the foundation while the product surface grows around it.

## 9. Current build order

The current phase is backend-only. The frontend is deferred until the API, persistence, and acceptance paths are stable.

1. PostgreSQL persistence and migration execution.
2. Event, team, submission, and eligibility operations.
3. Assignment builder, rubric versioning, and judging console depth.
4. Public participation, voting, comments, and integrity features.
5. OpenAPI, import/export, webhooks, certificates, and archive tooling.
6. Frontend shell and API-connected surfaces.

The next code change should deepen the backend rather than create frontend files.

## 10. Shipped in the first backend tranche

Work completed against the in-memory store, with a matching relational migration where the schema needed to change.

### Assignment builder

- judge profiles with track scopes, capacity, and active flags;
- conflict declarations that revoke the underlying assignment and block reassignment;
- bulk balanced assignment that re-sorts judges by current load per project;
- `manual`, `balanced`, and `batch` strategies with per-assignment skip reasons;
- organizer assignment list that hides revoked rows, and judge-scoped assignment inbox with a pending count;
- assignment revocation with an audit entry.

### Rubric versioning

- rubrics as versioned, track-scoped, publishable units with draft, published, and archived states;
- weights validated to total 100, per-criterion score ranges, and required criteria;
- newest published version wins, with a track-specific rubric overriding the event default;
- reviews record the rubric and version they were scored against, plus a normalized 0–100 projection;
- submitted reviews are locked; an identical resubmit stays idempotent;
- results and CSV read weights from the published rubric instead of a hardcoded map.

### Submission lifecycle

- captain-only revision that returns the project to `draft` and records a version snapshot;
- explicit resubmit, withdraw, and organizer status transitions ending at `locked`;
- eligibility decisions with a required note for `ineligible`, and ineligible projects drop out of the public gallery and eligible counts;
- duplicate detection over repository, live demo, and normalized title, with a resolve queue that requires a note;
- organizer-created events can now define tracks and prizes, which previously made a new event unable to accept any submission.

### Endpoints added

- `GET|POST /v1/organizer/assignments`, `DELETE /v1/organizer/assignments/{assignmentID}`
- `GET /v1/judge/assignments`, `POST /v1/judge/assignments/{assignmentID}/conflict`
- `GET|POST /v1/organizer/rubrics`, `PUT /v1/organizer/rubrics/{rubricID}`, `POST .../publish`, `POST .../archive`
- `GET /v1/judging/rubric`
- `GET /v1/submissions/{projectID}`, `PATCH ...`, `POST .../submit`, `POST .../withdraw`
- `PUT /v1/organizer/submissions/{projectID}/eligibility`, `PUT .../status`
- `GET /v1/organizer/duplicates`, `POST /v1/organizer/duplicates/scan`, `PUT /v1/organizer/duplicates/{duplicateID}`
- `GET|POST /v1/events/{slug}/tracks`, `GET|POST /v1/events/{slug}/prizes`
- `backend/migrations/0003_judging_expansion.sql`

### Still pending

PostgreSQL adapter and migration execution, public voting and comments, notifications preferences and delivery, import/export, webhooks, and certificates.

## 11. Shipped in the hackathon hosting tranche

### Hackathon hosting

- `GET /v1/events/{slug}` is now the full hackathon resource: tracks, prizes, hosts, milestones, custom questions, team policy, active rubric, and judge count, with the judge roster for staff.
- `PATCH /v1/events/{slug}` for summary, lifecycle state, judging mode, submission and judging deadlines, team size policy, global teams, reviews per project, and leaderboard visibility.
- Lifecycle states: `draft`, `registration_open`, `submissions_open`, `submissions_closed`, `judging`, `results_published`, `archived`.
- Milestones and hosts as first-class records, both readable publicly.
- `allow_global_teams` plus `min_team_size` and `max_team_size` define the team policy per hackathon.

### Massive submission forms

- Typed custom questions per hackathon: `short_text`, `long_text`, `url`, `select`, `checkbox`, `number`.
- Audience scoping so submission, team, and participant forms stay separate.
- Per-type validation with length limits, option allowlists, and number checks; required questions are enforced on submit.
- Duplicate question prompts are rejected per audience; question keys are derived from the prompt.
- `GET|POST /v1/events/{slug}/questions`, `PUT|DELETE /v1/organizer/questions/{questionID}`.
- Submission create and update now carry `story`, `thumbnail_url`, and validated `custom_answers`, and `repo_url` must resolve to a host plus repository path.

### Profiles and team formation

- User profiles with headline, bio, location, skills, and validated links for `github`, `linkedin`, `portfolio`, `website`, `x`, `bluesky`, `devto`, `youtube`, and `discord`; each link must match its expected scheme.
- Availability: `solo`, `looking_for_team`, `teamed`, with `seeking_team`, `open_to_invites`, `seeking_role`, and a per-hackathon `seeking_event_id`. Availability is the source of truth so the fields can never disagree.
- `GET /v1/profiles/{userID}` includes teams, links, and hackathon appearances; `PATCH /v1/profile` updates it.
- `GET /v1/discover` lists participants who are seeking a team or open to invites, excluding the caller.
- Teams carry a scope (`hackathon` or `global`), availability (`open`, `invite_only`, `closed`, `full`), `max_size`, and `open_roles`; an open team must publish at least one open role.
- Team invites by user id or email, with sender, message, expiry, accept, decline, and revoke; only captains and leaders can send them, and invitees who are not open to invites are refused.
- `GET /v1/discover/teams` lists teams open for members with their members and open roles.

### Judge rosters and assignment modes

- Per-hackathon judge roster with headline, expertise, scope, and active flag, plus a global reviewer pool.
- Adding a judge to a roster can also set track scopes and capacity in one call.
- The assignment builder now only considers judges on that hackathon roster or the global pool, so an organizer cannot assign a judge who is not staff for the event.
- `judging_mode` of `automatic` or `manual`; manual mode requires an explicit judge list, and `reviews_per_project` on the hackathon supplies the default reviewer count.

### Leaderboards and result notifications

- `GET /v1/events/{slug}/leaderboard` is public only after results are published and marked public; organizers always see it, and ineligible projects are hidden from the public board.
- `POST /v1/organizer/events/{slug}/publish-results` publishes, moves the hackathon to `results_published`, records pending review counts, and notifies every active team member with their placement plus the whole judge roster.
- `POST /v1/organizer/events/{slug}/unpublish-results` requires a reason, records an audit entry, and closes the public board.

### Hackathon appearances

- Participation history per user and hackathon with role, team, and result.
- `GET /v1/users/{userID}/appearances` and `GET /v1/users/me/appearances`, also embedded in the profile view.
- Participations are recorded when a team is created and when an invite is accepted.

### Endpoints added

- `PATCH /v1/events/{slug}`, `GET|POST /v1/events/{slug}/milestones`, `GET|POST /v1/events/{slug}/hosts`
- `GET|POST /v1/events/{slug}/questions`, `PUT|DELETE /v1/organizer/questions/{questionID}`
- `GET|POST /v1/events/{slug}/judges`, `DELETE /v1/events/{slug}/judges/{judgeID}`
- `GET /v1/events/{slug}/leaderboard`, `POST /v1/organizer/events/{slug}/publish-results`, `POST /v1/organizer/events/{slug}/unpublish-results`
- `GET /v1/profiles/{userID}`, `PATCH /v1/profile`, `GET /v1/discover`, `GET /v1/discover/teams`
- `POST /v1/teams/{teamID}/invites`, `DELETE /v1/teams/{teamID}/invites/{inviteID}`, `GET /v1/invites`, `POST /v1/invites/{inviteID}`
- `GET /v1/users/{userID}/appearances`, `PATCH /v1/teams/{teamID}`
- `backend/migrations/0004_hackathon_hosting.sql`

## 12. Shipped in the visibility and communication tranche

### Event directory, roles, and permissions

- `GET /v1/directory` returns a catalog of every hackathon with counts for teams, submissions, judges, tracks, milestones, custom questions, open invites, and participation, plus deadline countdowns, state, and search by `state`, `q`, or `slug`.
- Per-hackathon staff roles: `owner`, `co_organizer`, `judge_liaison`, `viewer`, each with its own permission set and rank. Adding a role at or below an existing one is refused, and only organizers and admins can hold event staff.
- `GET|POST /v1/events/{slug}/staff` and `DELETE /v1/events/{slug}/staff/{userID}`.
- `GET /v1/permissions` publishes the full matrix: every permission, its description and category, and the platform roles and event roles that hold it.

### Activity log

- A first-class activity stream separate from the audit log, with categories `event`, `team`, `submission`, `judging`, `results`, `account`, and `communication`.
- Visibility levels `public`, `participants`, `judges`, and `organizers`, enforced per viewer role and event membership.
- Recorded for team creation, invites, membership, submission create/revise/submit/withdraw, eligibility and status changes, duplicate scans, assignment creation and revocation, conflict declarations, judge roster changes, profile and preference updates, and result publication.
- `GET /v1/activity` and `GET /v1/events/{slug}/activity` support category, action, actor, target type, visibility, limit, offset, and a `counts_only` rollup with per-category and per-visibility totals.

### Mail

- `internal/mailer` with a template registry, an SMTP sender, and a log sink used automatically when SMTP is not configured, so development and tests never require a mail server.
- Templates: `welcome`, `verify_email`, `password_reset`, `account_deletion_scheduled`, `team_invite`, `submission_received`, `review_reminder`, `results_published`, `hackathon_announcement`, and `weekly_digest`.
- Transactional mail is tied to account and event actions; promotional mail is opt-in per topic.
- Preferences per user across transactional, account security, account lifecycle, team activity, event activity, judging, results, marketing, and weekly digest, with a full opt-out that also blocks account security mail.
- Outbox with statuses `queued`, `sending`, `sent`, `failed`, `skipped`, dedupe keys, exponential backoff capped at 30 minutes, and an attempt budget before a message is marked failed.
- Single-use unsubscribe tokens with expiry, consumed on use, driving a full opt-out.
- Organizer announcements to `all`, `seeking`, `team_captains`, or `judges`, with per-recipient exclusion reasons for people who never opted into marketing.
- Judging reminder and weekly digest jobs, plus a manual queue flush.
- Endpoints: `GET|PATCH /v1/email/preferences`, `POST /v1/email/verify`, `POST /v1/email/password-reset`, `GET /v1/unsubscribe/{token}`, `POST /v1/organizer/events/{slug}/announcements`, `POST /v1/organizer/events/{slug}/review-reminders`, `POST /v1/organizer/mail/flush`, `POST /v1/organizer/mail/weekly-digest`, `GET /v1/organizer/mail/outbox`, `GET /v1/organizer/mail/templates`.

### Configuration

- `APP_BASE_URL` for links in mail, `SMTP_FROM_NAME`, `MAIL_INTERVAL_SECONDS`, `MAIL_BATCH_SIZE`, `MAIL_MAX_ATTEMPTS`, all documented in `.env.example`.
- `backend/migrations/0005_activity_and_mail.sql` covers the activity log, event staff, mail preferences, mail outbox, and unsubscribe tokens.

## 13. Shipped in the community and integration tranche

### Comments and moderation

- Threaded comments on any project with `visible`, `hidden`, and `deleted` states, a 2000 character limit, and a five-comments-per-author-per-project cap.
- Public read for visible comments; organizers and admins also see hidden ones plus the moderation note and moderator.
- Authors can remove their own comments; organizers hide, restore, or delete with a required note.
- Reporting with duplicate-report protection, an organizer queue filtered by status, and actioned or dismissed resolution that requires a note.
- Endpoints: `GET /v1/events/{slug}/comments`, `POST /v1/events/{slug}/projects/{projectID}/comments`, `DELETE /v1/comments/{commentID}`, `PUT /v1/organizer/comments/{commentID}/moderate`, `POST /v1/comments/{commentID}/report`, `GET /v1/organizer/reports`, `PUT /v1/organizer/reports/{reportID}`.
- Comment posting is recorded in the activity log and emitted as a `comment.posted` webhook.

### Community voting

- Vote campaigns per hackathon with a choice budget between one and ten per participant, optional windows, an optional eligible-only rule, and a strict `draft` to `open` to `closed` lifecycle.
- One ballot per project per participant, budget enforced across calls, duplicate and unknown projects rejected, and no voting before opening or after closing.
- Tallies with deterministic tie-breaking, public results only after closing unless the hackathon board is public, and per-participant vote history.
- Endpoints: `GET /v1/events/{slug}/vote-campaigns`, `POST /v1/organizer/events/{slug}/vote-campaigns`, `PUT /v1/organizer/vote-campaigns/{campaignID}/{status}`, `POST /v1/events/{slug}/vote`, `GET /v1/events/{slug}/vote`, `GET /v1/vote/mine`.
- Closing a campaign records public activity and emits `vote.closed`.

### Webhooks

- Per-hackathon webhooks over https only, subscribing to a documented set of events, with a write-only signing secret of at least sixteen characters.
- Deliveries carry `X-CodeCeremony-Event`, `X-CodeCeremony-Delivery`, `X-CodeCeremony-Timestamp`, and an HMAC-SHA256 `X-CodeCeremony-Signature` over `timestamp.body`, with a documented verification helper and replay tolerance.
- Outbox with attempts, response code and body capture, exponential backoff capped at an hour, an attempt budget, and per-webhook delivery and failure counters.
- A background worker runs alongside the mail worker and is driven by the same interval and batch settings.
- Endpoints: `GET /v1/events/{slug}/webhooks`, `POST /v1/organizer/events/{slug}/webhooks`, `DELETE /v1/organizer/webhooks/{webhookID}`, `POST /v1/organizer/webhooks/{webhookID}/test`, `GET /v1/organizer/webhooks/deliveries`, `POST /v1/organizer/webhooks/flush`.
- Events emitted today: `submission.created`, `results.published`, `results.unpublished`, `vote.closed`, and `comment.posted`.

### Portability

- `GET /v1/organizer/export` returns a versioned bundle covering the hackathon, tracks, prizes, questions, milestones, hosts, rubrics, roster, teams, submissions, reviews, assignments, comments, activity, participations, vote results, and stats. Password hashes, session tokens, mail secrets, and webhook signing secrets are never included.
- `POST /v1/organizer/import` recreates a hackathon from a bundle with a dry-run mode that reports per-record warnings before anything is written, remaps ids for tracks and questions, and starts imported rubrics as drafts so nothing is scored until an organizer publishes one.
- The existing reviews CSV export is unchanged.

### API contract

- `GET /v1/openapi.json` serves an OpenAPI 3.1 document with security schemes, shared schemas, and per-route auth and public flags.
- `GET /v1/endpoints` serves the live route catalog generated during router construction, grouped by resource with auth classification, so the published surface can never drift from the running service.

### Fix found in this tranche

`manage_integrations` was only granted to platform admins, so organizers could not configure webhooks for the hackathons they host. It is now granted to organizers as well.


---

## 13. Shipped in the authorization and accountability tranche

### Action-centric authorization

- Routes are gated on an **action** from a closed vocabulary (`internal/authz`), not on a role. A role is a bundle of actions; an action is the thing that can be allowed, granted or denied.
- A fixed resolution order: account state, target constraints, explicit deny, explicit grant, event role, global role, ownership. Every decision reports the rule that produced it.
- `GET /v1/permissions` publishes the matrix by reading the real maps, so the documentation cannot drift from the enforcement.

### Per-event scoping, enforced

- Every route declares its event by `{slug}` or by `?event_id=`, and the gate resolves it before deciding. This closes the cross-event gap that `THREAT-MODEL.md` A7 previously described as open.
- Event roles are a **union** with global roles, strictly scoped by event. An organizer of one event is refused on another.
- Event creators are automatically `EventRoleOwner`.

### Explicit grants

- `GET|POST /v1/grants` and `DELETE /v1/grants/{id}`: per-user, per-action, per-event allows and denies with a mandatory reason and optional expiry.
- Deny is evaluated before allow, so revocation is immediate. Expired grants stop applying on their own.
- Grants are persisted in the snapshot.

### Hash-chained action audit

- Every authorization decision, including refusals, is written to a hash-chained log with the actor, action, target, event, outcome, reason and originating rule.
- `GET /v1/audit/actions` (filterable, paged backwards by sequence), `GET /v1/audit/actions.csv` (with `prev_hash`, so a third party can check the log is internally consistent), and `GET /v1/audit/verify`.
- Retention trims the oldest entries; the retained window verifies as a **suffix** and the dropped count is reported. The `head` is published so a verifier can anchor against a copy they hold.
- Keyed from `AUDIT_SECRET`, falling back to `SESSION_SECRET`, with a boot warning when derived.

### Write safety

- `Idempotency-Key` on any unsafe method. The response is remembered and replayed byte for byte, so a retry cannot double-apply. A key reused for a different body is a 422. Keys are scoped per actor. A 5xx is not remembered.
- `ETag` on reads and `If-Match` on writes for the two contended paths: editing a submission, and saving a review. A 412 carries the current ETag. Unconditional requests still work.

### Recorded pairwise comparisons

- `domain.Comparison`: an attributed, timestamped, reversible head-to-head verdict, as distinct from a comparison inferred from rubric scores.
- `POST /v1/events/{slug}/comparisons` (both projects must be assigned to the judge), `GET` for a judge and for an organizer, and `DELETE /v1/organizer/comparisons/{id}`.
- One verdict per judge per unordered pair; re-answering updates the row, so one judge cannot contribute two verdicts to the same match.
- The pairwise view uses recorded verdicts when the panel answered enough to stand alone, and reports `source`, `recorded_comparisons` and `derived_comparisons` either way. A partial recorded set is never blended into a derived one.

### Fixes found by the new tests

- The decisive-panel warning in the pairwise view fired on **every** response: both sentinels for the strength spread were seeded so that neither could update. It is now detected structurally.
- A trimmed audit chain could never verify, because verification was anchored on genesis and the oldest entries had been discarded.
- Four load-bearing handlers — submission create, review save, staff add and remove, sign-in and sign-out — recorded nothing, because the audit helper wrote to a different table than the chain reads.
- The audit chain was keyed with the empty string in every real run: `AuditSecret` existed on the seed struct and was set by nothing outside tests, and a restored boot skips seeding entirely.
- Organizer routes addressed by `?event_id=` could never match a per-event grant or deny, because the event was only resolved from the path.
- An all-ties record reported a 100% win rate, because a tie contributes half a win and the rate was computed from the weighted totals.
