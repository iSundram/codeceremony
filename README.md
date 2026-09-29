# CodeCeremony

> An open source, self-hostable submission and judging platform for hackathons.
> Built during DOGFOOD 2026. MIT licensed.

**Tagline: build the platform that will judge you.**

Most hackathon platforms converged on the same nine features and stopped. Three
of the things organisers actually need are missing: you cannot weight judging
criteria, no platform documents its score normalization, and community voting is
treated as unfixable. CodeCeremony is an attempt at the thing organisers want
rather than the thing that is easy to build.

```bash
docker compose up
```

That is the whole setup. A single static binary, one file of state, no database
container, no cloud account, no external API. It comes up on
`http://localhost:8080` seeded with the shared DOGFOOD fixtures, so this portal
and every other submission are looking at the same invented hackathon.

Verified with the organisers' own acceptance checker — 7 of 7, claimed T1–T3:

```
T1  gallery is public ................. PASS
T1  project from fixtures shown ....... PASS
T1  closed event refuses submissions .. PASS
T2  judge sees own scores ............. PASS
T2  judge cannot see peer scores ...... PASS
T2  participant blocked ............... PASS
T2  csv export works .................. PASS
```

(`acceptance-report.txt`, produced by `python3 run.py .dogfood.toml`.)

---

## What is actually here

**T1 — core.** Authentication with opaque revocable sessions; five roles
(`visitor`, `participant`, `judge`, `organizer`, `admin`); event creation with
configurable dates, tracks and prizes; team formation with invite links;
submissions with draft, edit, submit and withdraw; deadline enforcement that
actually holds; a public gallery with search and track filter.

**T2 — judging.** Judge invitation and assignment with three strategies; weighted
versioned rubrics; **role isolation enforced in the backend**; a live organizer
progress dashboard; documented cross-judge normalization; CSV export.

**T3 — public.** Community voting with per-user caps and randomized ballot order;
comments with moderation and reporting; results withheld until published;
rate limits, duplicate detection and a readable audit trail.

**T4 — partial.** A documented REST API with a generated OpenAPI document,
outbound webhooks, and bulk import/export. *Not* implemented: certificate
generation, signed participation records, and the embeddable widget.

Claimed in `.dogfood.toml` as T1–T3. The acceptance checker has no T3 tests, so
its report says `verified T1 T2`; that is the checker's coverage, not a failure.
T4 is deliberately not claimed.

---

## The thing worth looking at

**The judging engine does not print a confident leaderboard it cannot support.**

On the shared fixture data — 41 projects, 30 judges, 126 reviews — the projects
are packed into a raw-mean range of 2.90 to 4.38, the median gap between adjacent
ranks is **0.025 on a five-point scale**, and dropping 25% of the reviews at
random reproduces the same rank order only **62%** of the time.

A portal that prints `1. prj_34  2. prj_37  3. prj_33` from that is asserting a
precision the panel never produced. So the engine reports, for every project, a
confidence interval obtained by bootstrapping the **panel**, not the reviews —
because reviews by one judge are correlated by construction, which is exactly
what the normalization removes.

On the fixture data, **1 of 41 projects is separated from the project above it.**
The full method, the arithmetic, the evidence and — at some length — what it
cannot do are in [`JUDGING.md`](JUDGING.md). A second Bradley-Terry pairwise
estimator is there too, and it reports `"unbounded": true` when a panel is too
decisive for the maximum likelihood estimate to exist, rather than printing a
number decided by where the iteration happened to stop.

Second worth looking at: **role isolation, three times over.** `judgeScores`
refuses a `?judge=` naming anyone but the caller, the store call is
`ReviewsForJudge(eventID, principal.UserID)` so the judge identity is never read
from the request at all, and every all-judges endpoint requires a permission the
judge role does not hold. There is no `/judges/{id}/scores` route. Seven
score-bearing routes are probed as a non-owning judge in
`internal/httpapi/isolation_test.go`.

---

## Running it

### Docker (the supported path)

```bash
docker compose up
```

- Portal: <http://localhost:8080>
- Data: one file in the `codeceremony-data` volume
- Healthcheck: runs the binary's own `-healthcheck`, because the image is
  distroless and has no shell
- Override the host port with `CODE_CEREMONY_PORT=8090 docker compose up`

Back up is `cp` the data file. Reset is `docker compose down -v`.

### Locally

```bash
cd backend
FIXTURES_PATH=./fixtures.json go run ./cmd/codeceremony
```

Go 1.25. One non-stdlib dependency (`golang.org/x/crypto`, for bcrypt).

### Sign in

One shared password, `codeceremony-dev`, for every seeded account. The portal
prints the full credential table — including fixed auth headers for tools that
never log in — on every boot.

| Role | Email |
|---|---|
| Organizer | `organizer@example.org` |
| Admin | `admin@example.org` |
| Judge (11 reviews) | `diego.herrera@example.org` |
| Judge (flat scores — the low-information case) | `iva.petrova@example.org` |
| Participant | `participant@example.org` |

The portal seeds two events. **Sample Hack 2026** is the fixture event: closed,
judged, 41 projects, results withheld. **Open Call 2026** is left deliberately
open, with one draft project and two unscored assignments, so the whole
lifecycle can be walked through without touching a clock.

### The API

`GET /v1/endpoints` lists every route from the live router.
`GET /v1/openapi.json` returns a generated OpenAPI document.
`GET /v1/permissions` publishes the role matrix from the code that enforces it.

---

## Documentation

| Document | What it covers |
|---|---|
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | How it is built, and why each decision was made. Includes the constraint that closed most doors. |
| [`DATA-MODEL.md`](DATA-MODEL.md) | The schema that actually runs, the snapshot format, import and export paths. |
| [`JUDGING.md`](JUDGING.md) | Assignment, the scoring math, the normalization method defended, and what it cannot do. |
| [`THREAT-MODEL.md`](THREAT-MODEL.md) | Sybil votes, ballot stuffing, collusion, and the two gaps that are stated rather than hidden. |
| [`design.md`](design.md) | The visual system the frontend follows. |

---

## Honest limitations

Stated here and in the code rather than left to be discovered.

1. **Cross-event isolation is not enforced between organizers.** Any user with
   the global `organizer` role can act on any event by supplying its slug. A
   per-event role model exists in the domain types and is used for display, but no
   route enforces it. This is correct for one organisation self-hosting its own
   events and a real gap for multi-tenant hosting. See `THREAT-MODEL.md` A7.
2. **Storage is a single snapshot file, not a database.** Reads are linear scans
   and writes are whole-file. That is the right shape for a few thousand projects
   in one event and the wrong shape for a SaaS. The format is versioned and
   documented as a migration path; `Restore` is a reference implementation of the
   insert order.
3. **There is no SQL schema in the repository, on purpose.** About a thousand
   lines of PostgreSQL DDL were written early on and never executed by anything.
   They had drifted from the real code and contained defects that would abort a
   real migration, so they were removed. `backend/migrations/README.md` explains
   why; `DATA-MODEL.md` documents the schema that actually runs.
4. **No account lockout, deliberately.** An attacker who knows a judge's email
   could lock that judge out of the event they are meant to judge, which is worse
   than the brute-force it prevents.
5. **T4 is partial.** No certificates, no signed participation records, no
   embeddable widget. The REST API, webhooks and bulk import/export are real.
6. **No password reset flow completes.** Tokens are generated and stored; there
   is no route that redeems one.
7. **HTTP only.** TLS is expected to terminate in front of the portal.
8. **`docker compose up` was verified by rehearsal, not by Docker.** The image is
   distroless, which is not installed in the build environment used here. The
   binary was built with the same flags, run under the same environment
   variables, on a network namespace where port 8080 was genuinely free, and
   driven through the same checks. Treat the container itself as untested.

---

## Development

```bash
cd backend
go build ./... && go vet ./... && go test ./...
```

Eight test packages, standard library only. The tests are not coverage for its
own sake — each one exists because something was wrong at some point during the
build. A snapshot that carried users but not events. Float sums differing in the
last bit because criteria were iterated in randomized map order. A rubric weight
distribution that produced a total of 125 for three criteria. They are the record
of what went wrong.

To regenerate the acceptance report:

```bash
cd backend && go build -o /tmp/codeceremony ./cmd/codeceremony
FIXTURES_PATH=./fixtures.json /tmp/codeceremony &
cd .. && python3 run.py .dogfood.toml > acceptance-report.txt
```

---

## License

MIT. See [`LICENSE`](LICENSE).

Built during DOGFOOD 2026 by Hackathon Raptors' entrants. You keep ownership;
nothing is assigned and no CLA is signed.
