# ARCHITECTURE.md

How CodeCeremony is put together, and why each decision was made that way.

---

## 1. The constraint that decided everything

The event's one-command rule is that a laptop with the network off comes up with
a working, seeded portal. No cloud accounts, no hosted databases, no external
APIs, no authentication as a service.

That single constraint rules out most of what a web application normally reaches
for, and it is worth being explicit about which doors it closed:

| Normal choice | Closed by | What we did instead |
|---|---|---|
| Postgres or MySQL | a second service to run | single-file snapshot store |
| Next.js / Vite SPA | Node toolchain, `npm install`, a build step in the image | server-rendered Go templates, `go:embed` |
| OAuth / hosted auth | a hosted dependency | local accounts, opaque sessions |
| Redis | a second service | in-process fixed windows |
| A migration runner + SQL | nothing to apply it against | a documented, versioned snapshot format |
| A CSS framework | a package install | the design tokens already in `design.md` |

The result is a single static binary with one non-stdlib dependency
(`golang.org/x/crypto`, for bcrypt) and one embedded stylesheet.

---

## 2. Process shape

```
docker compose up
      │
      └── codeceremony (distroless, nonroot, read-only rootfs)
            ├── /codeceremony            the binary
            ├── /app/fixtures.json       the shared DOGFOOD fixture file
            └── /var/lib/codeceremony    volume: portal.json
```

One process, one port, one file of state. There is no message queue, no cache
tier, and no background worker other than two in-process tickers (the mail
dispatcher and the webhook dispatcher), both of which degrade to a log sink when
unconfigured.

The container runs unprivileged as uid 65532 with a read-only root filesystem, no
Linux capabilities, and `no-new-privileges`. The data directory is `COPY`'d into
the image owned by that uid so that Docker's named-volume initialisation
inherits the ownership — without that the volume is created root-owned and the
portal cannot write to it on a first boot. This was a real failure mode, not a
hypothetical one.

The healthcheck runs the binary's own `-healthcheck` flag rather than `curl`,
because distroless has no shell. The Dockerfile also runs `-version` during the
build, so a binary that cannot execute fails the build rather than someone
else's machine.

---

## 3. Package layout

```
backend/
  cmd/codeceremony/     composition root: flags, boot order, wiring
  internal/
    domain/             types and invariants, no dependencies
    store/              in-process durable state, snapshot and restore
    persistence/        the journal that writes state to disk
    judging/            normalization and the Bradley-Terry estimator
    fixtures/           the shared fixture loader
    seed/               the built-in demo data and the fixed credential table
    auth/               sessions, tokens, password hashing
    httpapi/            routing, authorization, JSON API, HTML frontend
    ratelimit/          fixed-window budgets
    mailer/             templates, queue, dispatch, webhooks
    config/             environment parsing and validation
    webassets/          embedded templates and stylesheet
```

Dependencies run one way: `domain` knows nothing, `store` and `judging` know
`domain`, and `httpapi` knows everything. There is no cycle, and `domain` has no
imports from the project at all.

### The composition root

`cmd/codeceremony` is the only package that knows about all the others. It owns
the boot order, which matters more than it sounds:

1. hash the seed password
2. create an **empty** store
3. construct the journal and try to restore
4. if there was nothing to restore, apply the seed
5. if seeding, mint the fixed credential sessions
6. build the API and start listening
7. start the persistence ticker

Restore happens **before** seeding on purpose. A data directory that already
holds an organizer's submissions wins, because those are not demo artefacts. And
seeding after a restore is what guarantees that accounts needing a password have
one, since a snapshot never contains password hashes.

---

## 4. Storage

`internal/store` is 38 maps behind one `sync.RWMutex`. That sounds like a
liability and here it is the right call, for three reasons:

- **One process.** There is no second node to keep in sync, so the usual reasons
  to reach for a database — concurrency across nodes, query planning, indexes —
  do not apply.
- **The data is small.** One event is a few thousand projects and tens of
  thousands of reviews. A linear scan over a few thousand entries is
  microseconds.
- **The durability requirement is one file.** `internal/persistence` writes a
  snapshot and reads it back. Backup is `cp`, restore is `cp`, and a reviewer can
  read the whole database in a text editor.

### Change detection

The journal does not instrument 39 mutating methods to raise a dirty flag. It
**polls**: every two seconds it takes a snapshot, encodes it, and compares a
SHA-256 against the last one written. Identical state means no write.

That is deliberately the boring choice. A missed dirty flag is silent data loss
with no symptom; a redundant snapshot costs microseconds and can never lose
anything. The cost is that a hard kill loses up to two seconds of writes, and a
clean shutdown loses nothing.

Two properties depend on this, so both are asserted in tests:

- `Snapshot` is **byte-identical** for identical state. Every collection is
  sorted by its own key, and criteria are iterated in sorted order because
  floating-point addition is not associative and Go's map iteration order is
  randomized. Without that, every tick would look like a change.
- `SavedAt` is stamped on the **file**, never on the captured state. Stamping it
  at capture time made every poll look like a change.

### Atomic writes

Write to a temporary file in the same directory, `fsync` it, `rename` it over
the target, then `fsync` the directory. Rename within a directory is atomic, so
a portal killed mid-write leaves a valid data file rather than a truncated one.
The directory sync is what makes the rename itself durable across a power cut.

### Sessions are deliberately not persisted

Token hashes are not serialised anywhere in the codebase, so **a restart signs
everyone out**. That is the correct trade for a judging platform: a stolen
session cookie should not survive a redeploy, and there is no session store to
steal from. Seeded identities are re-minted from the fixed token table at every
boot, so automation that attaches a documented header keeps working.

### The honest limitation

Reads are linear scans and writes are whole-file. That is the wrong shape for a
multi-tenant SaaS and the right shape for a few thousand projects in one event.
`DATA-MODEL.md` documents the snapshot format as a stable, importable schema, so
moving to a real database is writing a loader, not reconstructing a migration
story nobody can verify.

---

## 5. Authorization

One place authenticates a request: `Server.requirePermission`. It resolves the
token, loads the user, checks the account state, and re-reads the role **from
the store on every request** rather than from the token. Only the subject and
session id are trusted from the token, so a role change takes effect on the very
next request and a stale token can never be used to escalate.

```
route → requirePermission(permission) → [per-handler object checks] → handler
                │
                └── resolves principal, puts it on the request context
```

Coarse role gating lives in the middleware. Object-level checks live in the
handler, because they need the object. The list of object-level guards is
short and deliberate: team membership and captaincy, submission visibility,
assignment, comment authorship, event scoping, and the ballot.

### Judge score isolation

The requirement is that one judge cannot read another's scores, enforced in the
backend. It is enforced **three times over**, deliberately:

1. `GET /v1/judge/scores` refuses a `?judge=` naming anyone but the caller, with
   an explicit 403. Defence in depth — the next line would already scope
   correctly.
2. The store call is `ReviewsForJudge(eventID, principal.UserID)`. The judge
   identity is **never read from the request**; it comes from the verified
   session. There is no code path where a request chooses whose scores it sees.
3. `GET /v1/organizer/reviews`, `/results`, `/export` and `/export.csv` all
   require `ViewPeerScores` or `ExportData`, which the judge role does not hold.

There is no `/judges/{id}/scores` route and no per-judge resource of any kind.
`internal/httpapi/isolation_test.go` probes all of these, plus the neighbouring
leaks a `curl` would find: cross-judge assignment reads, judge address
disclosure, and the participant-as-judge case.

### Two role systems, consolidated to one enforced

The codebase grew a per-event role model (`owner`, `co_organizer`,
`judge_liaison`, `viewer`) that nothing enforced — it was rendered into a
permission-matrix endpoint and consulted nowhere. That is the kind of thing that
reads as depth and is actually dead weight.

The enforced model is the five global roles: `visitor`, `participant`, `judge`,
`organizer`, `admin`, with a `map[Role]map[Permission]` in `domain/roles.go` and
`Role.Can()`. The per-event model is documented as advisory and is used for
display only. `GET /v1/permissions` publishes the real matrix from the real
source, so it cannot drift.

**This is a known, deliberate limitation**: any user holding the global
`organizer` role can act on any event by supplying its slug. For a
single-organisation self-hosted portal, where the organizer role is trusted
staff, that is a reasonable trade. For multi-tenant hosting it is not, and the
fix is to enforce `EventRole.Can` on the `/v1/organizer/...` routes. It is stated
here rather than left to be discovered.

---

## 6. Judging

See `JUDGING.md` for the method. Architecturally, `internal/judging` is a pure
package: it takes reviews and weights and returns a summary. It has no store
access, no clock, and no randomness — the bootstrap PRNG is a seeded
splitmix64 written out longhand precisely so the package stays pure and
reproducible.

Two estimators live side by side: the rubric pipeline (bounded, with confidence
intervals) and Bradley-Terry pairwise (which reports `unbounded: true` when the
panel is decisive enough that the maximum likelihood estimate does not exist).

---

## 7. The frontend

Server-rendered Go templates, embedded with `go:embed`, served from the same
binary on the same port as the API.

A separate SPA was rejected for a concrete reason rather than a stylistic one: it
would put a Node toolchain, a package install and a build step inside the
container, which is exactly what the one-command rule cannot afford. The
templates compile into the binary and the stylesheet is embedded, so there is
nothing to fetch and nothing to build.

The HTML pages read the same store through the same authorization primitives as
the JSON routes, and the form handlers re-derive their own checks rather than
delegating to the API handlers, so the two surfaces cannot drift into disagreeing
about who may do what. The styling uses the approved palette and spacing scale
from `design.md`; no component uses a raw colour.

Responses carry a restrictive `Content-Security-Policy`
(`default-src 'none'; style-src 'self'`), which costs nothing for a same-origin
server-rendered app and rules out the obvious injection vectors.

---

## 8. Rate limiting

`internal/ratelimit` is a fixed-window counter keyed by `(policy, caller)`.

- **Authenticated routes key on the account, not the address.** This matters in
  practice: a hackathon venue, a university or a corporate network puts an entire
  panel behind one egress address, and a per-address budget there would let the
  busiest attendee lock everyone else out of commenting.
- **Anonymous routes key on the address.** The login budget is the one that
  matters, and it is the strictest at 10 per minute.
- **Only abuse-prone routes are budgeted.** There is no blanket limit on reads.
  A generous read cap would be a self-inflicted denial of service against the
  public gallery for no security benefit.

A fixed window lets a caller send 2x the limit across a boundary. That is
accepted deliberately: halving the budget to remove the artefact would halve
real throughput for every legitimate user.

---

## 9. What is deliberately absent

Listed because their absence is a decision, and a reviewer should not have to
guess whether it is an oversight:

- **No database.** One file, one process. Documented above.
- **No refresh tokens.** Sessions are short-lived opaque tokens; there is nothing
  to refresh.
- **No multi-tenant isolation between organizers.** Stated in section 5.
- **No email delivery by default.** With no `SMTP_HOST`, mail is written to a log
  sink. Verification, password reset, announcements and reminders all work; none
  of them send anything until an organizer configures SMTP.
- **No password reset consumption route.** Tokens are generated and stored but
  there is no endpoint that redeems one, so a reset email cannot currently
  complete. Left in place rather than removed, and called out here.
- **There is no SQL schema in the repository.** See below.

---

## 10. Why there is no SQL in the repository

`backend/migrations/` held six PostgreSQL migration files, about a thousand lines
describing 41 tables. **No Go code read, embedded, or executed any of it** — there
is no database driver in the module and no migration runner. They predate the
snapshot store and drifted away from the code that actually runs.

They were also defective. One file re-adds a constraint name PostgreSQL had
already auto-assigned, which aborts the migration on a real database. Four files
use `CREATE TABLE IF NOT EXISTS` against tables an earlier file had already
created, so on a real database the *earlier* shape silently wins and the new
columns never appear. One file reintroduces a webhook table with the signing
secret in plaintext, undoing the `secret_hash` an earlier file had set — a
credential regression. And three incompatible designs existed for `comments`, two
for `team_invites` and `ballots`, and two for `webhooks`, with the code
implementing a fourth combination.

Shipping known-broken, unreferenced, misleading code is worse than shipping
nothing, so they were removed and `backend/migrations/README.md` left in their
place explaining why. `DATA-MODEL.md` documents the schema that actually runs.

The one thing worth keeping from that exercise is the reasoning: three
incompatible designs for `comments` is what happens when a feature is designed
twice, and `webhooks` in particular shows how a secret hash can quietly become a
plaintext secret through a "harmless" additive change. Both are worth guarding
against on the real schema.

---

## 11. Testing

Go's standard library only. `go test ./...` runs eight packages.

The tests are not coverage for its own sake. Each one exists because something
was wrong at some point during the build:

- `internal/persistence` — a snapshot that carried users but not events, because
  the collection loops were written for four collections out of twenty-seven. It
  reported a successful restore and booted with no data at all.
- `internal/judging` — float sums differing in the last bit because criteria were
  iterated in randomized map order; a neutral imputation that was left at Go's
  zero value; a separability flag assigned to a struct copy instead of the slice
  element.
- `internal/httpapi/isolation_test.go` — the cross-judge probes, and the
  behaviour changes that came with removing address disclosure from public
  endpoints.
- `internal/fixtures` — a weight distribution that produced 40/70/15 for three
  criteria, a total of 125, for a rubric the validator would reject.

---

## 12. Configuration

Every setting is an environment variable, read once in `config.Load` and
validated there. Defaults are development-friendly and the production guard rails
are explicit: a default `SESSION_SECRET` is refused in production, and enabling
seeding in production additionally requires a private `SEED_PASSWORD` of at
least twelve characters, because seeding mints fixed publicly documented tokens
for accounts that hold organizer and admin rights.

That last guard is the important one. The seeded credentials are correct for a
self-hosted demo and indefensible in production, and the failure mode of getting
it wrong is a full authentication bypass. The configuration refuses the
combination rather than trusting the operator to notice.
