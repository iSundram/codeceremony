# CodeCeremony — Comprehensive Product Feature Specification
> **Superseded.** This is the pre-implementation feature specification, kept as a
> record of what was planned. For what is actually built and what is honestly
> missing, read [`README.md`](README.md), which lists the real tier claims and the
> known limitations.

> **Status:** Feature decisions locked; implementation is in progress using the P0–P2 order below.
>
> **Product:** CodeCeremony — an open-source, self-hostable hackathon submission and judging platform.
>
> **Scope rule:** This document defines the product surface. Implementation follows the priorities and freeze rules; scope must not expand silently.
>
> **Read first:** `rules.md`, `structure.md`, `design.md`, and `ACCOUNT-MANAGEMENT.md`.

## 1. Product decision

CodeCeremony is a modern alternative to hackathon platforms such as Devpost. It is not a clone, rebrand, or thin wrapper around an existing platform. It is an organizer-operated system for running a complete event lifecycle:

1. register participants;
2. form teams;
3. collect and validate submissions;
4. publish a searchable gallery;
5. assign judges;
6. collect weighted, isolated reviews;
7. normalize and aggregate results;
8. publish outcomes;
9. preserve records, exports, and certificates.

The product is designed to be operated by a volunteer organizer on a laptop or a small self-hosted server, without cloud accounts or hosted service dependencies.

## 2. Scope priorities

The feature set is large, but the implementation order is deliberate.

| Priority | Meaning | Commitment |
|---|---|---|
| P0 | Foundation required to operate the product | Must work |
| P1 | Official T1 Core | Must be complete and verified |
| P2 | Official T2 Judging | Primary target; must be correct |
| P3 | Official T3 Public | Build selectively after P2 is stable |
| P4 | Official T4 Stretch | Build only if time and correctness allow |
| P5 | Future expansion | Document the boundary; do not build for the current submission |

**Freeze rule:** The official code freeze is 28 September 2026 at 18:00 UTC. A feature not working and documented by the freeze is not a success. Do not start a large P3/P4 area while P1/P2 correctness is incomplete.

**Honesty rule:** A working, documented T2 is better than a partially broken T4. Never claim a feature merely because it appears in this document.

## 3. Personas and roles

### 3.1 Visitor

A visitor can:

- view a public event page;
- browse the public gallery;
- search and filter published submissions;
- view published results;
- use a public voting link when the organizer explicitly enables it;
- view public certificates or participation records when enabled.

A visitor cannot:

- see private drafts;
- see judge scores;
- see another user's private team data;
- see results before publication;
- access organizer or administrative data.

### 3.2 Participant

A participant can:

- maintain a profile;
- create or join a team;
- accept or leave team invitations;
- create and edit a submission draft;
- submit a project before the deadline;
- view the team's own submission status;
- upload permitted project media;
- comment where commenting is enabled;
- vote where voting is enabled;
- view published results and certificates;
- export their own records where permitted.

A participant cannot:

- read another team's private submission data;
- read judge scores;
- read aggregate results before publication;
- change event deadlines;
- assign judges;
- export organizer-wide data without permission.

### 3.3 Judge

A judge can:

- view assigned events and tracks;
- declare a conflict of interest;
- view the projects assigned to them;
- open the scoring form;
- save a draft review;
- edit a review until the review deadline;
- submit a final review;
- read only their own scores and comments;
- view their own progress;
- export their own review data where permitted.

A judge cannot:

- view another judge's scores or ballot;
- view an unassigned track;
- view organizer-only progress;
- view aggregate results before the judge is authorized;
- modify rubric definitions;
- modify assignments;
- change another review.

Isolation is enforced in the backend and API, not only in the UI.

### 3.4 Organizer

An organizer can:

- create and configure events;
- manage event tracks, prizes, deadlines, and rubric;
- manage teams and submissions;
- invite and assign judges;
- view judge progress and missing reviews;
- view aggregate and normalized results;
- configure voting and comments;
- publish or unpublish results;
- moderate comments and votes;
- export CSV and JSON;
- read the audit trail;
- issue certificates and records;
- import existing event data;
- archive an event.

An organizer cannot bypass the audit trail or silently alter submitted reviews.

### 3.5 Admin

An admin has all organizer capabilities plus:

- manage platform users;
- assign platform roles;
- manage event templates;
- inspect cross-event operational data;
- manage API tokens and webhooks;
- view platform-wide audit records;
- suspend or recover accounts;
- run migrations and maintenance operations.

Admin access is still logged and does not make hidden UI-only behavior acceptable.

### 3.6 Seeded system identities

The local seed process must create working test identities for:

- organizer;
- judge A;
- judge B;
- participant;
- admin.

The seed output must provide the exact authentication headers or tokens that a participant can place in `.dogfood.toml`. The official checker never logs in.

## 4. Event lifecycle

An event is a stateful record with explicit transitions.

### 4.1 Event states

| State | Meaning |
|---|---|
| `draft` | Configuration is being built; not publicly visible. |
| `registration_open` | Participants can register and form teams. |
| `submissions_open` | Teams can create, edit, and submit projects. |
| `submissions_closed` | New submissions and edits are refused. |
| `judging` | Assignments and reviews are active. |
| `voting` | Optional public voting window is active. |
| `results_published` | Results are visible according to policy. |
| `archived` | Read-only historical record. |

### 4.2 Event configuration

Each event supports:

- name and slug;
- short and long description;
- timezone-aware display settings;
- start and end timestamps;
- registration deadline;
- submission deadline;
- judging window;
- optional voting window;
- results-publication timestamp;
- announcement text;
- logo and brand references;
- contact email;
- team size limits;
- submission eligibility rules;
- gallery visibility;
- comment policy;
- voting policy;
- result-visibility policy;
- custom submission questions;
- tracks;
- prizes;
- rubric.

All deadlines are stored in UTC and displayed in the event's configured timezone.

### 4.3 Event templates

An organizer can save an event configuration as a reusable template. A template copies configuration only; it never copies participants, teams, submissions, reviews, or results.

Template operations are P5 unless time permits after T2.

## 5. Identity, sessions, and access control

### 5.1 Authentication

The system provides:

- local registration;
- login and logout;
- password hashing with a modern adaptive algorithm;
- server-side sessions;
- session expiry;
- logout of one session and all sessions;
- optional invite-based account activation;
- password change;
- account recovery instructions that do not require an external email provider.

No hosted authentication provider may be required.

### 5.2 Authorization model

Authorization is enforced in the service layer and repeated at the data-access boundary where necessary.

Every protected operation checks:

1. authenticated identity;
2. role;
3. event membership;
4. track scope;
5. assignment scope;
6. record ownership;
7. event state;
8. deadline.

A UI route being hidden is never considered authorization.

### 5.3 Permission matrix

**The authoritative matrix is `GET /v1/permissions`**, generated from the same
maps the resolver reads. A hand-maintained table like this one is a second source
of truth and will drift; it is here to be readable, not to be trusted over the
endpoint. Authorization is by **action**, not by role, and an event role unions
with the global role within its own event only — so "Organizer: Yes" below means
for events they organize, not for every event on the portal.

| Capability | Visitor | Participant | Judge | Organizer | Admin |
|---|:---:|:---:|:---:|:---:|:---:|
| View public event | Yes | Yes | Yes | Yes | Yes |
| View public gallery | Yes | Yes | Yes | Yes | Yes |
| View published results | Yes | Yes | Yes | Yes | Yes |
| Create account | Yes | Yes | Yes | Yes | Yes |
| Create/join team | No | Yes | Optional | Yes | Yes |
| Edit own draft | No | Yes | No | Yes | Yes |
| Edit another submission | No | No | No | Yes | Yes |
| View judge assignments | No | No | Own only | Yes | Yes |
| View another judge's scores | No | No | No | Yes | Yes |
| View aggregate results early | No | No | No | Yes | Yes |
| Configure rubric | No | No | No | Yes | Yes |
| Record head-to-head comparison | No | No | Assigned only | Yes | Yes |
| View the action audit | No | No | No | Own events | Yes |
| Verify the audit chain | No | No | No | Own events | Yes |
| Grant or deny an action | No | No | No | Own events | Yes |
| Publish results | No | No | No | Yes | Yes |
| Moderate comments/votes | No | No | No | Yes | Yes |
| Manage platform users | No | No | No | No | Yes |
| Read audit trail | No | No | No | Yes | Yes |

“Own only” and “Yes” are not substitutes for backend checks. Organizer access to judge scores is allowed by the product model and must still be audited.

## 6. Teams and participation

### 6.1 Team lifecycle

- A participant can create a team.
- A team has a name, optional description, owner/captain, members, and creation time.
- Default team size is 1–4, configurable per event.
- A participant may belong to one active team per event by default.
- A captain can invite, remove, or transfer ownership according to event policy.
- A team can be marked ineligible by an organizer with a recorded reason.
- Team history is retained after a member leaves.

### 6.2 Invitations

Invitation links contain an opaque, single-purpose token with:

- event binding;
- team binding;
- expiry;
- maximum uses;
- creator;
- revocation state;
- audit record.

Invitation behavior:

- expired links are refused;
- revoked links are refused;
- used links are refused unless explicitly reusable;
- an invitation cannot silently place a user in a different event or team;
- acceptance is idempotent and race-safe.

### 6.3 Eligibility

Eligibility rules can require:

- participant account;
- team membership;
- track selection;
- required custom answers;
- no conflicting judge assignment;
- submission completeness;
- event-specific organizer rules.

An eligibility decision is stored with a reason and timestamp. Eligibility failures are explainable, not opaque.

## 7. Submission lifecycle

### 7.1 Submission states

| State | Meaning |
|---|---|
| `draft` | Editable; not visible to judges or the public. |
| `submitted` | Complete and included in eligibility checks. |
| `needs_changes` | Organizer returned it for correction before the deadline. |
| `withdrawn` | Team or organizer withdrew it before final judging. |
| `locked` | Immutable after the deadline or administrative lock. |
| `disqualified` | Excluded with a recorded reason. |

### 7.2 Submission fields

Core fields:

- title;
- tagline or one-line summary;
- long description;
- thumbnail;
- image gallery;
- hosted demo-video URL;
- repository URL;
- live link;
- technology tags;
- track;
- team;
- custom organizer questions.

A submission stores the event and track it belonged to at the time of submission so later event changes do not rewrite history.

### 7.3 Draft and editing

- Drafts save locally through the application database.
- A participant sees only their team's drafts.
- Drafts are not indexed publicly.
- Autosave is optional and must not create confusing duplicate records.
- Edit permission is checked on every write, not only when the page loads.
- A submitted project can be edited until the deadline if event policy allows it.
- Every meaningful edit creates a version or audit entry.

### 7.4 Deadline enforcement

The backend compares the current time to the event's submission deadline using UTC.

A closed event refuses:

- new submissions;
- edits to submitted projects;
- replacement of media;
- changes that bypass the submission lock.

The official fixture event closes on 1 March 2026, so a correctly seeded portal is already closed when the checker runs. A participant submission probe must receive a 4xx response.

### 7.5 Validation and duplicate detection

Validation covers:

- required fields;
- title and summary length;
- URL format;
- supported media type;
- file size;
- track membership;
- team membership;
- custom question responses;
- deadline state.

Duplicate detection can compare:

- normalized title;
- repository URL;
- live URL;
- team and track;
- content fingerprint;
- near-duplicate text.

A duplicate is flagged for organizer review; it is never silently deleted. The fixture deliberately contains a duplicate project record.

### 7.6 Version history

A version record can contain:

- version number;
- author;
- timestamp;
- changed fields;
- optional reason;
- integrity hash.

The version history is organizer-visible and exportable. Participants see their own team's history.

## 8. Gallery and discovery

### 8.1 Public gallery

The gallery supports:

- grid and list presentations;
- search by title, tagline, tags, and team;
- track filter;
- tag filter;
- status filter for authorized users;
- sorting by published or submission time;
- pagination;
- empty, loading, and error states;
- responsive card layout;
- accessible project links.

The gallery must show real seeded fixture projects, not hardcoded marketing data.

### 8.2 Project detail

A public project page can show:

- title and tagline;
- description;
- media;
- team name;
- track;
- tags;
- repository and live links;
- demo video;
- custom public answers;
- published result, if policy allows;
- comments, if enabled.

Private drafts, internal eligibility notes, judge reviews, and private custom answers never appear publicly.

### 8.3 Search and filtering

Search behavior is:

- case-insensitive;
- whitespace-tolerant;
- scoped to the current event;
- safe against arbitrary query injection;
- paginated;
- consistent between grid and list views.

The same filter state is reflected in the URL when practical so a view can be bookmarked without creating a new component.

## 9. Judge management and assignment

### 9.1 Judge invitations

An organizer can invite a judge by:

- creating an invitation record;
- generating a shareable link;
- optionally recording an email address for display;
- setting track scope;
- setting assignment capacity;
- setting an expiry.

The invitation is not an external email dependency. The organizer can copy or share the link through any channel.

### 9.2 Assignment strategies

Supported strategies:

1. **Manual assignment** — organizer selects specific projects.
2. **Batch assignment** — organizer creates a batch and assigns its projects.
3. **Balanced assignment** — distribute workload by current assignment count.
4. **Track-aware assignment** — only assign judges authorized for the project track.
5. **Conflict-aware assignment** — exclude declared conflicts.
6. **Round-robin assignment** — deterministic distribution for equal workloads.

Default strategy: deterministic balanced, track-aware assignment with conflict exclusion. The algorithm and seed must be documented so the result is reproducible.

### 9.3 Assignment invariants

- A judge is not assigned their own team or declared conflict.
- A track judge does not receive projects outside their scope unless explicitly authorized.
- Assignment counts never exceed a configured capacity without an explicit override.
- A project receives the configured number of reviews where capacity allows.
- Unassigned projects are visible to organizers as an exception.
- Reassignment is audited and does not delete prior review history.

### 9.4 Judge progress

The organizer can see:

- assigned count;
- started count;
- completed count;
- draft count;
- missing count;
- overdue count;
- workload distribution;
- conflict declarations;
- unassigned projects.

Judges see only their own progress.

## 10. Rubric and review workflow

### 10.1 Rubric builder

An organizer can define:

- criterion name;
- description;
- scoring scale;
- minimum and maximum score;
- weight;
- required flag;
- whether comments are required;
- track-specific override;
- rubric version.

Default scale is 1–5. Criterion weights must total 100% before the rubric can be activated.

### 10.2 Rubric versioning

- A rubric is immutable once reviews begin.
- Editing an active rubric creates a new version.
- Existing reviews remain attached to the version they used.
- Results record the rubric version used for aggregation.
- Organizers can compare rubric versions.

### 10.3 Review form

A review can contain:

- one score per criterion;
- criterion-level comment;
- overall comment;
- private judge note;
- recommendation, if the organizer enables one;
- conflict declaration;
- final-submission flag;
- saved-at and submitted-at timestamps.

The review form supports:

- keyboard navigation;
- autosave or explicit save;
- clear unsaved-state indication;
- validation before final submission;
- read-only mode after final submission;
- draft recovery;
- accessible error messages.

### 10.4 Score visibility

- A judge sees their own scores.
- A judge never sees a peer's scores.
- A participant never sees review scores before results publication.
- An organizer and admin can see authorized review data.
- Aggregates are not exposed through judge routes.
- Track restrictions apply to project details, review queues, exports, and API responses.

## 11. Normalization and results

### 11.1 Raw score

For a completed review, calculate the weighted raw score:

```text
raw = sum(criterion_score * criterion_weight) / 100
```

Keep the raw score, criterion values, weights, and rubric version for auditability.

### 11.2 Default normalization method

The default method is documented and reproducible:

1. Group completed reviews by judge and criterion.
2. Calculate a within-judge z-score for each criterion when the judge has enough variation.
3. Combine normalized criterion values using the rubric weights.
4. Convert the combined value to a stable 0–100 presentation scale.
5. If a judge has too few reviews or effectively zero variance, mark the judge as low-information and use a documented fallback rather than dividing by an unstable value.
6. Store the normalization method version, input values, fallback decisions, and output values.

The implementation must show what happens to a judge who rates everything identically. It must not silently discard that judge's data without an audit reason.

### 11.3 Aggregation

Default project result:

```text
result = mean(normalized project score across eligible completed reviews)
```

Aggregation excludes:

- withdrawn submissions;
- disqualified submissions;
- invalid or incomplete reviews;
- reviews from judges with unresolved conflicts, according to policy.

The result record stores:

- raw scores;
- normalized scores;
- reviewer count;
- excluded review reasons;
- rubric version;
- normalization version;
- calculation timestamp.

### 11.4 Ranking and ties

Rank by normalized result descending. Deterministic tie-breakers are applied in this order:

1. higher raw mean;
2. more completed eligible reviews;
3. earlier first submission time;
4. stable project identifier as the final deterministic fallback.

Every tie-break is recorded in the result explanation.

### 11.5 Results publication

Organizers can:

- preview results;
- publish results;
- unpublish results;
- freeze a result snapshot;
- publish written feedback;
- export the result set;
- archive the event.

Before publication, visitors and participants cannot see aggregate results. During a voting window, results remain hidden from non-organizers even if reviews are complete.

## 12. Public participation

### 12.1 Community voting

Voting is configurable per event:

| Mode | Behavior |
|---|---|
| `disabled` | No public voting. |
| `open_link` | A shareable link allows voting without an account. |
| `email_token` | A one-time email-bound token is required; no external mail service is assumed. |
| `authenticated` | A signed-in participant votes. |

Default mode: `authenticated`. Public-link mode is opt-in and carries a higher abuse risk.

### 12.2 Voting methods

Supported methods:

- one-person-one-vote;
- approval voting;
- quadratic or budget-based voting as an optional T3 method.

The selected method, budget, eligibility, and aggregation rule are visible to voters.

### 12.3 Ballot integrity

- Randomize project order per ballot.
- Prevent duplicate votes from the same verified identity or token.
- Rate-limit repeated requests.
- Record vote events in an audit trail.
- Allow organizers to invalidate suspicious votes with a reason.
- Do not expose live results to voters during the voting window when hidden-result mode is enabled.
- Do not use a position-dependent ballot order.

### 12.4 Comments

When enabled, comments support:

- authenticated or token-based identity according to event policy;
- plain text and approved formatting;
- edit/delete history for the author or moderator;
- moderation queue;
- spam and rate-limit controls;
- organizer removal with a recorded reason;
- clear distinction between public and private moderation notes.

Comments never expose judge-only information.

## 13. Organizer dashboard and administration

### 13.1 Dashboard metrics

Organizer views include:

- registration count;
- team count;
- eligible and ineligible submissions;
- submission completion;
- draft versus submitted counts;
- judge invitations;
- assignment coverage;
- review completion;
- missing and overdue reviews;
- score variance;
- normalization warnings;
- votes and moderation counts;
- comments awaiting review;
- audit events.

Every metric respects the organizer's event scope and permission.

### 13.2 Data management

Organizers can:

- edit event configuration;
- archive events;
- correct eligibility state;
- lock or unlock submissions;
- resend or regenerate invite links;
- revoke access;
- export data;
- import data;
- view validation errors;
- retry failed import jobs.

Destructive operations require confirmation and an audit record.

### 13.3 Audit trail

Audit events record:

- actor;
- action;
- target type and identifier;
- event;
- timestamp;
- before/after metadata where safe;
- request or correlation identifier;
- reason for manual overrides.

Audit records are append-only from the application's perspective and are not editable through normal organizer UI.

## 14. Import, export, API, and integrations

### 14.1 Import

Supported import formats:

- JSON fixtures;
- CSV projects;
- CSV teams and participants;
- rubric configuration;
- assignment batches.

An import is:

- validated before commit;
- mapped through an explicit field mapping;
- idempotent where possible;
- reported with row-level errors;
- recorded as an import job;
- reversible or compensating according to the import type.

The organizer's supplied `fixtures.json` must load successfully at startup.

### 14.2 Export

Supported export formats:

- CSV for spreadsheet use;
- JSON for migration and backup;
- event archive bundle;
- results snapshot;
- review dataset for authorized organizers;
- judge participation records.

Exports respect role and track scope. An export must not accidentally bypass peer isolation.

### 14.3 REST API

A documented REST API covers every action available in the UI, including:

- authentication-independent public reads where allowed;
- event management;
- team operations;
- submission lifecycle;
- assignments;
- reviews;
- aggregates;
- voting;
- comments;
- exports;
- audit reads.

API requirements:

- OpenAPI specification;
- consistent error envelope;
- pagination and filtering;
- explicit authorization errors;
- request correlation;
- safe retry behavior;
- versioned contract strategy;
- no undocumented admin backdoor.

### 14.4 Webhooks

Optional webhook events include:

- event published;
- results published;
- submission submitted;
- review completed;
- vote cast or invalidated;
- import completed;
- export completed.

Webhook deliveries include a signature, timestamp, event ID, and retry metadata. Delivery failures are visible to organizers.

### 14.5 API tokens

Organizers and admins can create scoped API tokens with:

- explicit permissions;
- expiry;
- last-used timestamp;
- revocation;
- audit history.

Tokens are never displayed in plaintext after creation.

## 15. Certificates, records, and archive

### 15.1 Certificates

An organizer can generate certificates for:

- participants;
- teams;
- judges;
- volunteers or organizers, if enabled.

Certificate data includes:

- recipient;
- event;
- role;
- issue date;
- verification identifier;
- optional public verification URL.

### 15.2 Participation records

A participation record can prove:

- that a person registered;
- that a person belonged to a team;
- that a judge was assigned;
- that a review was completed;
- that a project was submitted.

Records can be signed and independently verifiable when the T4 signed-record feature is implemented.

### 15.3 Archive

Archived events are read-only but retain:

- configuration;
- teams;
- submissions;
- reviews;
- normalized results;
- votes;
- comments;
- certificates;
- audit history;
- exports.

Archive is a first-class state, not a deletion mechanism.

## 16. Notifications and communication

Because external email and hosted messaging services are not allowed as required dependencies, the first release uses:

- in-app notifications;
- visible invite links;
- notification inbox;
- unread state;
- event announcements;
- copyable links;
- optional digest view;
- webhook delivery for external systems that choose to integrate.

A notification never contains private judge data or unpublished results.

## 17. Search, analytics, and reporting

### 17.1 Search

Search is available for:

- projects;
- teams;
- participants, for authorized organizers;
- reviews, for authorized organizers;
- audit events, for admins.

### 17.2 Analytics

The platform records operational and quality metrics:

- funnel counts;
- review latency;
- assignment balance;
- score distributions;
- judge variance;
- duplicate and conflict flags;
- voting abuse signals;
- import/export outcomes.

Analytics must be explainable and exportable. A chart never replaces the underlying data.

### 17.3 Reporting

Reports can be filtered by:

- event;
- track;
- team;
- date;
- status;
- reviewer;
- completion state.

Reports are read-only snapshots unless an explicit administrative action is taken.

## 18. Operational and non-functional requirements

### 18.1 Startup

- `docker compose up` starts the complete local environment.
- The database is created and migrated automatically.
- Fixture data seeds automatically on first boot.
- Seeded identities and authentication headers are printed or written to a local development file.
- No cloud account, external API, hosted database, or API key is required.
- Repeated startup is safe and does not create unbounded duplicate fixture records.

### 18.2 Configuration

- Configuration comes from environment variables or local files.
- `.env.example` contains only safe placeholders.
- Secrets are never committed.
- Timezone storage is UTC.
- Event timezone is used for display only.

### 18.3 Security

- Passwords use an adaptive password hash.
- Cookies are HttpOnly and SameSite by default.
- CSRF protection applies to state-changing browser requests.
- Inputs are validated at the boundary and in the application layer.
- Database access uses parameterized queries or safe query builders.
- Authorization is enforced server-side.
- Rate limits protect login, invites, voting, comments, and exports.
- Sessions can be revoked.
- Sensitive values are not written to logs.
- Error responses do not leak internal details.

### 18.4 Reliability

- Database migrations are versioned and reversible where practical.
- Import jobs are resumable or clearly restartable.
- Export jobs have a status and failure reason.
- Health and readiness endpoints are available locally.
- Logs are structured enough to diagnose failures.
- Backup and restore instructions are documented.

### 18.5 Accessibility and design

All UI follows `design.md`:

- light theme only;
- approved palette and accent roles;
- approved component inventory;
- visible keyboard focus;
- WCAG AA target;
- responsive sidebar and app shell;
- no unapproved component or visual pattern.

If a feature requires a component not listed in `design.md`, the design document must be updated first.

## 19. Logical data entities

These are logical entities for product planning, not final physical tables:

- `User`
- `Role`
- `Session`
- `Event`
- `EventSettings`
- `EventTemplate`
- `Track`
- `Prize`
- `Team`
- `TeamMembership`
- `TeamInvite`
- `Submission`
- `SubmissionVersion`
- `SubmissionAnswer`
- `CustomQuestion`
- `Rubric`
- `RubricVersion`
- `Criterion`
- `JudgeProfile`
- `JudgeTrackScope`
- `ConflictDeclaration`
- `Assignment`
- `Review`
- `ReviewScore`
- `ReviewComment`
- `NormalizationRun`
- `NormalizedScore`
- `ResultSnapshot`
- `VotingCampaign`
- `Ballot`
- `Vote`
- `Comment`
- `ModerationAction`
- `Certificate`
- `ParticipationRecord`
- `ImportJob`
- `ExportJob`
- `Webhook`
- `WebhookDelivery`
- `ApiToken`
- `Notification`
- `AuditEvent`

Key invariants:

- every mutable record belongs to an event or has an explicit global scope;
- every score references a rubric version and reviewer;
- every assignment references a project, judge, and event;
- published results reference a frozen calculation snapshot;
- audit events are append-only;
- soft deletion is preferred over destructive deletion where history matters.

## 20. Feature IDs and priority

### P0 — Foundation

| ID | Feature | Priority |
|---|---|---|
| F-001 | Local Docker startup and health checks | P0 |
| F-002 | Database migration and fixture seeding | P0 |
| F-003 | Local account authentication and sessions | P0 |
| F-004 | Role model and backend authorization | P0 |
| F-005 | Seeded organizer, judges, and participant identities | P0 |
| F-006 | Audit event recording | P0 |
| F-007 | UTC time handling and event state model | P0 |
| F-008 | Safe configuration and secret handling | P0 |
| F-009 | Shared API contract and error format | P0 |
| F-010 | Basic responsive application shell | P0 |

### P1 — T1 Core

| ID | Feature | Priority |
|---|---|---|
| F-101 | Event creation and configuration | P1 |
| F-102 | Tracks, prizes, deadlines, and custom questions | P1 |
| F-103 | Team creation and membership | P1 |
| F-104 | Invite links and acceptance | P1 |
| F-105 | Submission draft, validation, and editing | P1 |
| F-106 | Submission deadline enforcement | P1 |
| F-107 | Public gallery search and filtering | P1 |
| F-108 | Public project detail pages | P1 |
| F-109 | Eligibility state and reasons | P1 |
| F-110 | Submission version history | P1 |

### P2 — T2 Judging

| ID | Feature | Priority |
|---|---|---|
| F-201 | Judge invitation and track scope | P2 |
| F-202 | Manual and balanced assignment | P2 |
| F-203 | Conflict declaration and exclusion | P2 |
| F-204 | Weighted rubric builder and versioning | P2 |
| F-205 | Review form and draft saving | P2 |
| F-206 | Own-score access and peer-score isolation | P2 |
| F-207 | Organizer progress dashboard | P2 |
| F-208 | Raw score calculation | P2 |
| F-209 | Documented cross-judge normalization | P2 |
| F-210 | Result aggregation and ranking | P2 |
| F-211 | CSV export | P2 |
| F-212 | Review and normalization audit trail | P2 |

### P3 — T3 Public

| ID | Feature | Priority |
|---|---|---|
| F-301 | Configurable voting mode | P3 |
| F-302 | Random ballot ordering | P3 |
| F-303 | Duplicate and rate-limit controls | P3 |
| F-304 | Hidden results during voting | P3 |
| F-305 | Comments and moderation | P3 |
| F-306 | Vote audit and invalidation | P3 |
| F-307 | Quadratic or approval voting experiment | P3 |
| F-308 | Public result publication page | P3 |

### P4 — T4 Stretch

| ID | Feature | Priority |
|---|---|---|
| F-401 | Full REST API and OpenAPI document | P4 |
| F-402 | Webhooks and delivery retries | P4 |
| F-403 | Certificate generation | P4 |
| F-404 | Signed participation records | P4 |
| F-405 | Embeddable gallery widget | P4 |
| F-406 | Bulk import and export | P4 |
| F-407 | Pairwise Bradley–Terry judging mode | P4 |
| F-408 | Event archive and migration bundle | P4 |

### P5 — Future

- localization and multiple languages;
- advanced organizer analytics;
- reusable event templates;
- native mobile applications;
- external email/SMS providers;
- multi-organization SaaS billing;
- real-time collaborative editing;
- advanced anti-fraud machine learning;
- custom certificate artwork;
- additional voting mechanisms;
- third-party plugin marketplace.

These are documented as possible future directions, not current commitments.

## 21. Official acceptance mapping

The implementation must expose the following logical routes through `.dogfood.toml`; exact paths are chosen by the implementation.

| Official check | Required behavior |
|---|---|
| Gallery is public | Unauthenticated gallery request returns 200. |
| Fixture project is visible | A known fixture title appears in the gallery response. |
| Closed event refuses submission | Participant submission probe returns 4xx. |
| Judge sees own scores | Judge A receives 200. |
| Judge cannot see peer scores | Judge B receives 401 or 403. |
| Participant is blocked from judging | Participant receives 401 or 403. |
| Organizer exports CSV | Organizer receives 200 and a CSV body. |

The portal must not rely on a hosted database, hosted authentication, cloud account, or external API. `acceptance-report.txt` must be generated by the official `run.py` and committed, including any honest failures.

## 22. Definition of done for a feature

A feature is done only when:

1. its behavior is implemented in the backend, not only the UI;
2. its authorization rules are enforced;
3. its validation and error states exist;
4. its loading, empty, and failure states exist;
5. it works with seeded fixture data;
6. it survives a restart through local persistence;
7. it has focused automated tests where time permits;
8. it follows `design.md` for any UI;
9. it is documented in the appropriate markdown file;
10. it is reflected honestly in the tier claim.

A feature is not done merely because a screen exists.

## 23. Demo script

The five-minute demo should show one complete lifecycle:

1. start the local environment with one command;
2. show seeded data and roles;
3. create or open an event;
4. create a team and submit a project;
5. show the closed-deadline behavior;
6. assign a judge;
7. score with a weighted rubric;
8. demonstrate peer-score isolation;
9. show organizer progress;
10. show normalization and results;
11. export CSV;
12. show the published gallery and results.

The demo must use real application behavior, not a static mockup.

## 24. Locked decisions and open decisions

### Locked decisions

- Product name: CodeCeremony.
- Backend: Go service.
- Frontend: TypeScript and Next.js in SPA mode.
- Database: PostgreSQL, migration-driven.
- Local runtime: Docker Compose.
- Shared contract: OpenAPI plus shared TypeScript types.
- Theme: light only for the first release.
- Accent colors: supplied `#749DD0` and `#92AAD1`.
- Component policy: only the inventory in `design.md`.
- Default team size: 1–4, configurable per event.
- Default scoring scale: 1–5.
- Default rubric weights: total 100%.
- Default assignment: balanced, track-aware, conflict-excluding.
- Default normalization: documented within-judge normalization with low-information fallback.
- Default voting mode: authenticated and disabled until enabled.
- Default results policy: hidden from non-organizers until publication.
- Import/export: JSON and CSV.
- Notifications: in-app and webhook-based; no required external provider.
- Future dark theme: not in the current release.

### Open decisions requiring supplied assets or explicit approval

- final logo and wordmark files;
- final font hosting or bundled font files;
- final icon library;
- certificate signing format;
- public verification URL format;
- exact optional bonus selected;
- exact T3/T4 features attempted before freeze.

If one of these is needed during implementation and has not been decided, report it rather than inventing it.

## 25. Build order

The intended order is:

1. P0 runtime, persistence, seed, and auth foundation;
2. P1 event, team, submission, and gallery path;
3. P1 acceptance checks;
4. P2 rubric, assignment, review, and isolation;
5. P2 progress, normalization, results, and CSV;
6. P3 only after P2 is stable;
7. P4 only with remaining time and a working acceptance report;
8. documentation and demo video throughout, not at the end.

This is the comprehensive product surface. It is a decision map, not a promise to implement every P3/P4 item before the freeze.

## 26. Account management expansion

The complete first-party account, session, team-role, permission, notification, My Account, and SMTP decisions are maintained in [`ACCOUNT-MANAGEMENT.md`](ACCOUNT-MANAGEMENT.md). That document is normative for account behavior.

### Account feature IDs

| ID | Feature | Priority |
|---|---|---|
| F-501 | Local account states and lifecycle | P0 |
| F-502 | Revocable server-side sessions and device management | P0 |
| F-503 | Team member, leader, and captain roles | P1 |
| F-504 | Team invitations, promotion, demotion, and captain transfer | P1 |
| F-505 | Team removal, leaving, deletion request, and archive safeguards | P1 |
| F-506 | Event-scoped organizer and moderator assignments | P2 |
| F-507 | Admin account search, state changes, role assignment, and session revocation | P2 |
| F-508 | Permission catalog and deny-by-default policy enforcement | P0 |
| F-509 | Audit and security event viewer | P2 |
| F-510 | In-app notification inbox, preferences, and activity catalog | P2 |
| F-511 | Notification digests and mandatory security notices | P3 |
| F-512 | First-party My Account center and account data controls | P4 |
| F-513 | Optional SMTP outbox, templates, retries, and delivery status | P4 |
| F-514 | Email/webhook verification and signed action links | P4 |
| F-515 | Account export, merge, anonymization, and retention workflows | P4 |

### Account decisions

- Local authentication is the only identity system and remains mandatory.
- There is no Google, OIDC, OAuth, or other external identity provider.
- SMTP is optional mail transport only; absent SMTP leaves in-app notifications fully functional.
- Global account types are participant, judge, organizer, admin, and explicit service identities. Visitors are unauthenticated actors.
- Team roles are member, leader, and captain, scoped to one event team.
- Protected actions require backend authorization and an audit event.
- No notification or email payload may contain private judge data, unpublished results, session tokens, or secrets.

These features must not displace the T1/T2 acceptance path. The current backend has local bcrypt authentication, store-backed revocable sessions, profile/password/export/deletion endpoints, team membership roles, notifications, and admin account controls.
