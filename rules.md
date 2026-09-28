# DOGFOOD 2026 — Rules and Project Brief

> **Status:** Implementation authorized. Backend foundation is in progress; T1/T2 verification is not complete.
>
> **User instruction for this workspace:** Implementation may proceed during the event window. Keep the scope honest, do not commit build artifacts or secrets, and stop to report gaps.
>
> **Last checked:** 26 September 2026.

This document is the handoff brief for any AI or developer joining the project. Read the whole file before planning. Do not infer requirements that are not stated here or in the official sources.

## 1. AI handoff instructions

1. Treat the official DOGFOOD website, specification, checker, fixtures, and current Discord announcements as the source of truth.
2. Before continuing implementation, inspect the workspace and report:
   - the proposed stack;
   - the data model;
   - the route and authorization design;
   - the T1/T2 implementation order;
   - the Docker and seeding plan;
   - the acceptance-test plan;
   - known risks and unfinished work.
3. Continue only within the authorized event window and the committed plan.
4. Do not create a pre-existing-project rewrite, a design-only mockup, or a hardcoded frontend.
5. Do not claim a tier that has not been implemented and verified. Honest scope is explicitly rewarded.
6. Do not expose credentials, private data, or secrets. The supplied fixture data is synthetic.
7. If two official sources conflict, report the conflict instead of silently choosing an interpretation. The organizer's current Discord clarification should be preferred when it addresses the ambiguity.

## 2. Event snapshot

| Item | Details |
|---|---|
| Event | DOGFOOD 2026 |
| Organizer | Hackathon Raptors |
| Format | Online |
| Build period | 25–28 September 2026, 72 hours |
| Kickoff | 25 September 2026 at 18:00 UTC |
| Code freeze and submission deadline | 28 September 2026 at 18:00 UTC |
| Entry fee | Free |
| Team size | 1–4 people; solo entries welcome |
| Prize pool | USD 2,500 |
| Main product | An open-source, self-hostable hackathon submission and judging platform |
| Core promise | The winning project may be forked, self-hosted, and used by Hackathon Raptors |

The event is unusual: every team builds the same product. It is not a competition in which each team invents an unrelated product.

The public site describes Hackathon Raptors as having organized 35 hackathons since 2023 across more than 85 countries. The stated problem is that existing platforms generally provide registration, teams, submissions, galleries, judging, voting, dashboards, and CSV export, but often lack weighted rubrics, transparent score normalization, strong role isolation, abuse controls, and a public API.

## 3. What must be built

The product is a complete hackathon event lifecycle:

1. Registration and users.
2. Events with dates, tracks, and prizes.
3. Teams and invite links.
4. Project submissions and eligibility.
5. Judge invitations and assignments.
6. Weighted scoring.
7. Cross-judge normalization.
8. Results and public gallery.
9. Community voting and comments.
10. Certificates, records, exports, and archive.

The goal is not merely a CRUD demo. The organizers want software they could run on Monday for real events.

“Devpost alternative” is a useful shorthand, but it is not permission to clone, rebrand, or rewrite Devpost. The intended product is a modern, open-source, self-hostable, API-capable alternative with stronger judging integrity and operability.

The public brief says the winning project is the one the organizers intend to fork and run. They may also borrow ideas from another close entry, but they say they will disclose that publicly and credit both teams.

## 4. Timeline — all times UTC

### Before the event

| Date | Milestone |
|---|---|
| 24 August 2026 | Registration opens. |
| 4 September 2026 | Judging panel announced. |
| 21 September 2026 | Team formation; teams of 1–4. |
| 23 September 2026 | Raptors Conference; separate online event. |
| 24 September 2026 | Specification published. Reading, planning, stack selection, and schema sketching are allowed. Project code is not allowed before kickoff. |

### Build window

| Date/time | Milestone |
|---|---|
| 25 September 2026, 18:00 UTC | Kickoff. Building may begin. |
| 26 September 2026, 18:00 UTC | First 24 hours. |
| 27 September 2026, 18:00 UTC | 48-hour checkpoint. |
| 28 September 2026, 18:00 UTC | Code freeze. Repository, report, and required deliverables are due. |

### After the build

| Date | Milestone |
|---|---|
| 28 September–8 October 2026 | Judging window. |
| 5 October 2026, 18:00 UTC | Write Up Quest closes. |
| 9 October 2026 | Winners and adoption decision announced. |

The public site says the judging panel is already seated and assignments are issued. Each project is intended to receive multiple independent structured reviews.

## 5. Registration and team rules

- Teams contain one to four people.
- Solo participation is explicitly allowed.
- The organizers recommend two or three people because the work naturally divides across frontend, backend, and documentation.
- Teammates can be found through the Hackathon Raptors Discord before or during the event.
- The public website displays “Registration open” and directs people to Discord.
- The public pages do not expose a separate registration form or a clearly documented submission-upload form. The exact registration and final submission channel must be confirmed in Discord.
- Anonymous GitHub usernames are accepted if the team remains reachable for written follow-up during judging.
- No public age, citizenship, student-status, or geographic eligibility restriction is stated on the public pages. Do not invent one; ask in Discord if it matters.
- The public pages do not specify prize-payment, tax, banking, or age-verification procedures.

## 6. Non-negotiable rules

These are the official validity rules. A failure of the core requirements can make a submission invalid or unscorable.

### 6.1 Open source and OSI-approved license

- The repository must be public at submission.
- The project must use an OSI-approved open-source license.
- MIT and Apache-2.0 are preferred because they are easiest for the organizers to adopt.
- Copyleft licenses are allowed and do not inherently lose points.
- The team keeps ownership.
- No assignment, transfer, CLA, or exclusivity clause is requested.

### 6.2 One command to a running portal

The default command is:

```bash
docker compose up
```

It must produce a working, seeded portal on a laptop. It must not require:

- a cloud account;
- a hosted database;
- an external API;
- an authentication-as-a-service provider;
- an API key;
- a signup;
- a staging URL;
- a manually prepared remote service.

The FAQ allows a genuinely equivalent single command if it works locally without external services, but Docker Compose is the safest default.

The organizers describe the requirement as running offline on a laptop. A hosted staging deployment is not a substitute for a reproducible local deployment.

### 6.3 T1 is the minimum floor

A submission that does not clear T1 is not judged. T1 is not a stretch goal; it is the eligibility gate.

### 6.4 New code only

All project code must be written during the 72-hour event window.

Allowed before kickoff:

- reading the brief;
- choosing a stack;
- sketching a schema;
- planning routes;
- preparing prompts;
- forming a team;
- discussing architecture.

Not allowed before kickoff:

- an existing project of yours being submitted as the project;
- an existing open-source platform with a new name;
- project implementation committed before kickoff.

Frameworks, libraries, boilerplate generators, and AI-assisted coding are allowed.

### 6.5 Honest tier claims

Declare the tiers actually reached in `.dogfood.toml`. The acceptance report is the receipt.

- Claiming a tier that is not verified is penalized.
- A report with honest failures is better than a README that overclaims.
- A clean, correct T2 is preferable to a broken T4.
- Incomplete work should be described honestly in the README.

### 6.6 Backend-enforced authorization

Role checks must exist in the backend/API. Hiding a button or route in a template is not access control.

A judge must not be able to retrieve another judge's scores by changing a URL, query parameter, or request path. Track judges must not access other tracks unless the organizer's policy explicitly permits it.

### 6.7 Public source and reachable team

The GitHub repository must be public at submission. The team must be reachable for judge questions and written follow-up during the evaluation window.

### 6.8 AI tools

AI tools are expected and allowed. Examples named by the organizers include Claude Code, Cursor, Aider, Copilot, and local models.

The organizers do not score whether AI was used. They score whether:

- the portal actually runs;
- authorization survives direct HTTP requests;
- the data model is defensible;
- the documentation explains the decisions;
- a team member can explain the schema and scoring design.

## 7. Out of scope or likely to score poorly

The public brief lists these as ways to score nothing or lose substantial points:

1. Design mockups, Figma files, or a frontend with hardcoded data.
2. Anything requiring a cloud account, hosted database, or hosted authentication provider to start.
3. An authentication demo that stops at the login screen.
4. A gallery with no judging, or judging with no gallery.
5. Role checks implemented only in the frontend.
6. An LLM-generated dump with no architecture document and nobody able to defend the schema.
7. Closed-source code or a non-OSI-approved license.
8. Anything requiring custom hardware, GUI toolchains, or proprietary services.
9. A rewrite of an existing open-source platform with only the name changed.

## 8. Tier ladder

There are no competition tracks. There is one product and four implementation tiers.

### T1 — Core

Required features:

- authentication and sessions;
- real roles: visitor, participant, judge, organizer, and admin;
- event creation with configurable dates, tracks, and prizes;
- team formation through invite links;
- project submission with draft and edit capability until the deadline;
- deadline enforcement in the backend;
- public project gallery with search and filtering.

Suggested submission fields, based on the organizers' research:

- project name/title;
- tagline or short summary;
- long description;
- thumbnail;
- image gallery;
- hosted demo-video URL;
- repository URL;
- live link;
- technology tags;
- track;
- organizer-defined custom questions.

### T2 — Judging

Features:

- judge invitations;
- judge assignment by batches or algorithm;
- organizer-configurable weighted scoring rubric;
- backend-enforced role and track isolation;
- live organizer progress dashboard showing who has not started;
- cross-judge score normalization;
- a documented and defensible normalization method;
- CSV export at every stage.

The organizers specifically call out these problems:

- weighted criteria are missing or fixed on incumbent platforms;
- normalization is advertised but not explained;
- judges may be lenient, harsh, or inconsistent;
- role isolation is often only cosmetic;
- community voting is vulnerable to abuse.

### T3 — Public

Features:

- community voting with configurable access:
  - open link;
  - email-gated;
  - authenticated;
- a defensible alternative to one-person-one-vote, such as quadratic voting;
- comments on gallery projects;
- results hidden from everyone except organizers during the voting window;
- randomized project ordering on ballots to reduce position bias;
- meaningful anti-abuse controls:
  - rate limits;
  - duplicate detection;
  - readable audit trail.

### T4 — Stretch

Features:

- REST API covering every action available in the UI;
- webhooks;
- certificate and record generation;
- signed, publicly verifiable judge-participation records;
- embeddable gallery widget;
- bulk import and export so organizers can migrate data in and out.

A T3 or T4 feature is not automatically better than a correct T2. Correctness, operability, documentation, and integrity matter more than raw feature count.

## 9. Official acceptance suite

The organizer provides a Python 3 standard-library checker:

```bash
python3 run.py .dogfood.toml > acceptance-report.txt
```

Keep `fixtures.json` next to `run.py`, or pass its path with `--fixtures`.

The checker never logs in. It attaches the headers supplied in `.dogfood.toml`. Login pages may use any normal mechanism, but the seed process must provide working headers for the four test identities.

### Required checker configuration

The file belongs at the repository root and has this general shape:

```toml
[portal]
base_url = "http://localhost:8080"

[tiers]
claimed = ["T1", "T2"]
pitch = "One sentence describing what you built."

[auth]
organizer = "Cookie: session=organizer-token"
judge_a = "Cookie: session=judge-a-token"
judge_b = "Cookie: session=judge-b-token"
participant = "Cookie: session=participant-token"

[routes]
gallery = "/projects"
submit = "/projects/new"
judge_scores = "/api/judge/scores"
peer_scores = "/api/judge/scores?judge=judge_a"
csv_export = "/api/export.csv"
```

The route names are examples, not fixed requirements. The organizer explicitly allows any language, framework, database, schema, ORM, and route naming.

### The seven checks

| Tier | Check | Request identity | Expected result |
|---|---|---|---|
| T1 | Gallery is public | No authentication | `200` |
| T1 | Fixture project is visible | No authentication | A known fixture project title appears in the response body |
| T1 | Closed event refuses submission | Participant | Any `4xx` response |
| T2 | Judge reads own scores | `judge_a` | `200` |
| T2 | Judge cannot read peer scores | `judge_b` using `peer_scores` | `401` or `403` |
| T2 | Participant cannot use judge endpoint | Participant | `401` or `403` |
| T2 | Organizer can export CSV | Organizer | `200` and a response whose first line contains a comma |

Important implementation details:

- The fixture event's `submissions_close` is in the past. Seed that date honestly so the submission probe is rejected.
- The checker sends a JSON body to the submission route.
- The peer-score URL must be a URL that would expose judge A's scores if authorization were broken.
- The participant must be rejected at the API, not merely shown a different UI.
- The CSV check is shallow, but the organizers will still judge the quality and meaning of the export.
- The checker has explicit checks for T1 and T2 only. T3 and T4 are reviewed manually by judges; they are not automatically verified by this file.

### Acceptance report

Commit the checker output as `acceptance-report.txt`, even if it contains failures. The report should show:

- the portal URL;
- claimed tiers;
- fixture file used;
- one PASS/FAIL line per check;
- detailed failure diagnostics;
- claimed versus verified tiers.

Do not fabricate or manually rewrite a passing report.

## 10. Fixture data

The organizer's downloadable `fixtures.json` is synthetic input for every team. It is not a required database schema. Load it, transform it, and store it in a model that can be defended.

The current file contains:

- 1 event;
- 8 tracks;
- 30 judges;
- 40 teams;
- 41 project records, representing 40 unique project submissions;
- 126 score records;
- 3 criteria per score: `functionality`, `quality`, and `innovation`.

The event closes at:

```text
2026-03-01T18:00:00Z
```

Important edge cases:

- `prj_07` and `prj_41` are duplicate submissions for the same team, track, title, summary, and repository, with different IDs and timestamps.
- Some projects have 2 reviews, some 4, some 5, and most have 3.
- Judge `jdg_07` gives the same complete rubric score to all of their reviews, creating a zero-variance judge case.
- Judge workloads are uneven; some judges have one review and others have more than ten.
- Some comments are empty.
- Team names are not unique.
- All IDs are strings and timestamps are ISO 8601 UTC values.
- The data intentionally models incomplete and messy real-world event records.

Use the real downloaded JSON rather than relying only on the small example in the specification. The marketing copy rounds some of these figures; the actual file is the input that matters for implementation.

## 11. Required repository deliverables

The repository should contain:

```text
your-portal/
├── .dogfood.toml
├── acceptance-report.txt
├── docker-compose.yml
├── README.md
├── ARCHITECTURE.md
├── DATA-MODEL.md
├── JUDGING.md
├── LICENSE
├── src/
└── tests/
```

Also provide a five-minute demo video showing a complete lifecycle:

1. create an event;
2. create or join a team;
3. submit a project;
4. assign and score judges;
5. publish results.

### Documentation expectations

`README.md` must explain:

- what the project does;
- how to run it;
- how to use the seeded accounts or roles;
- what is incomplete;
- known limitations.

`ARCHITECTURE.md` must explain:

- the system shape;
- major components;
- important decisions;
- trust boundaries;
- why the design is suitable for self-hosting.

`DATA-MODEL.md` must explain:

- the schema;
- relationships and constraints;
- how fixtures are imported;
- how organizers import and export data;
- duplicate and missing-review handling.

`JUDGING.md` must explain and defend:

- assignment strategy;
- rubric weighting;
- score calculation;
- cross-judge normalization;
- treatment of lenient, harsh, or constant-score judges;
- role isolation;
- auditability;
- any pairwise or alternative judging mode.

`acceptance-report.txt` is generated output, not a hand-written claim.

## 12. Scoring

Judges rate each project on a 1–5 scale across four weighted criteria. The final result is the weighted average across the judges who evaluated the project.

| Criterion | Weight | What matters |
|---|---:|---|
| Tier Completion & Correctness | 40% | How far the implementation honestly reaches; verified behavior; correct T1/T2; complete rather than broken features. |
| Judging Integrity | 25% | Backend authorization, defensible normalization, assignment, audit trail, and abuse thinking. |
| Adoptability & Operability | 20% | One-command startup, local operation, seeded data, clear documentation, import/export, clean license. |
| Code Quality & Innovation | 15% | Idiomatic implementation, defensible schema, maintainability, and a genuinely valuable design decision. |

T1 is a gate rather than a normal scored tier. Above T1, correctness is more important than breadth.

The organizers explicitly say that “we averaged the scores” is a weak answer for normalization. A strong `JUDGING.md` explains what happens to a judge who rates everything low, high, or identically, and why the method is fair.

## 13. Optional bonus challenges

Bonuses do not increase the weighted main score. They break ties and help decide the Best Judging Engine prize. Do one properly rather than starting all four.

| Challenge | Difficulty | Tie-break value | Requirement |
|---|---|---:|---|
| Normalization Proof | Hard | +5 | Normalize fixture scores, show raw and normalized results, show ranking changes, and defend the method. |
| Pairwise Mode | Hard | +5 | Let judges compare two projects and recover a global ranking with a Bradley–Terry-style estimator. |
| Threat Model | Medium | +3 | Document Sybil votes, ballot stuffing, scraping, collusion, deadline gaming, mitigations, and unresolved risks. |
| API First | Medium | +3 | Make every UI action available through a documented API with a published OpenAPI specification. |

The maximum nominal bonus is 16 points, but the organizers advise against attempting all four. Bonuses are not added to the 40/25/20/15 score.

## 14. Prizes

Total pool: USD 2,500.

| Prize | Amount |
|---|---:|
| 1st place / Grand Prize | $800 |
| 2nd place / Runner-up | $500 |
| 3rd place | $350 |
| 4th place | $200 |
| 5th place | $150 |
| Best Judging Engine | $100 |
| Write Up Quest | $400 total, four awards of $100 |

### Write Up Quest

Publish a technical write-up of the build on X, LinkedIn, Dev.to, a personal blog, or another developer-focused platform. Tag Hackathon Raptors; the downloadable context also references `#DogfoodHackathon`.

Good topics include:

- a schema decision you would redo;
- normalization mathematics;
- a role-isolation bug;
- a feature you cut;
- a design you abandoned;
- a disappointing benchmark;
- a debugging story;
- the moment the spec became harder than expected.

Judging is based on technical insight and substance, not follower count. The write-up is optional and does not affect the main score.

## 15. Adoption terms for the winner

The organizers state that:

- the team keeps ownership of the repository;
- no assignment, transfer, CLA, or exclusivity agreement is required;
- the winning project may be forked, self-hosted, and put into production for Hackathon Raptors events;
- the team receives credit on event pages powered by the platform;
- future fixes, hardening, and features may be sent upstream as pull requests;
- credit remains with the team even if the original team is no longer active.

They also state that, if two entries are close, they may adopt one and borrow ideas from another while publicly identifying and crediting both sources.

## 16. Judging panel and process

The public page states:

- 36 panel seats;
- three reviews per project;
- an evaluation window from 29 September through 8 October 2026;
- independent structured reviews;
- judges do not see other judges' ballots;
- weighted scores and written feedback go to every team, including teams that do not place.

The named panel includes engineers, architects, product leaders, and infrastructure specialists from organizations including Microsoft, Amazon Web Services, Meta, Adobe, GoDaddy, Walmart, Yahoo, Avito, Wise, Lululemon, and others. The authoritative list and biographies are on the official event page.

The public page says the panel is seated and assignments are issued. Contact for judge nominations or event questions: `hello@raptors.dev`.

## 17. Official source files

Use these official files rather than copied summaries:

- Event site: <https://dogfoodhack.com/>
- Specification page: <https://dogfoodhack.com/spec>
- Full specification: <https://dogfoodhack.com/spec/spec.md>
- Acceptance checker: <https://dogfoodhack.com/spec/run.py>
- Fixture data: <https://dogfoodhack.com/spec/fixtures.json>
- Example configuration: <https://dogfoodhack.com/spec/example.dogfood.toml>
- Full context: <https://dogfoodhack.com/spec/context.txt>
- Discord: <https://discord.gg/xfYPDZYqeh>
- Organizer listing: <https://www.raptors.dev/project/dogfood-2026-build-the-platform-that-will-judge-you>

The public files are currently downloadable. The event timeline says fixtures and the acceptance suite are released at kickoff, but the specification page also says they are already available; use the current official files.

## 18. Known ambiguities and discrepancies

These are documented so a future AI does not guess:

1. The public site displays “Registration open,” but the public HTML has no separate registration form. Discord is the documented entry point.
2. The public pages do not clearly document the final repository-submission channel. Confirm it in Discord.
3. The marketing copy says roughly 40 projects and three reviews per project. The actual fixture has 41 project records, 40 unique submissions, and 2–5 reviews per project.
4. The public panel has 36 seats, while the synthetic fixture has 30 judges. These are different datasets.
5. The event has no competition tracks, but the platform and fixture model event tracks; the fixture contains eight tracks.
6. One description refers to a judge who rates everything a 3, while the current constant-score fixture judge rates complete criteria at 4. Use the actual fixture.
7. The checker has T1 and T2 checks only. T3/T4 completion is judged manually.
8. No public page specifies age, tax, payment, or regional eligibility rules. Ask before assuming an answer.

## 19. Recommended planning target

Unless a future team decision says otherwise, target a correct, documented T1/T2 implementation first:

1. Local Docker startup and deterministic fixture seeding.
2. Users, sessions, and role model.
3. Event, track, team, invite, and project-submission lifecycle.
4. Public gallery and search/filter.
5. Deadline enforcement.
6. Judge assignment and configurable weighted rubric.
7. Own-score access and backend peer-score denial.
8. Organizer progress view and CSV export.
9. Documented normalization and audit trail.
10. Acceptance report, architecture/data/judging documents, and demo video.

Only after T1/T2 is stable should the team consider T3/T4 or one bonus challenge.

## 20. Current handoff status

- Event rules and specification: captured in this file.
- Project implementation: backend foundation started under `backend/`.
- Workspace: `/root/dogfood`.
- User instruction: continue implementation during the event window; do not commit build artifacts or secrets.
- Next AI action: read this file, inspect the current backend, run its tests, and continue the next authorized T1/T2 slice.
