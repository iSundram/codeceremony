# CodeCeremony — Next Expansion Roadmap

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

PostgreSQL adapter and migration execution, team invites, public voting and comments, notifications preferences and delivery, import/export, webhooks, and certificates.
