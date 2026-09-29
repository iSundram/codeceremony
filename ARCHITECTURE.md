# ARCHITECTURE.md

<p align="center">
  <img src="docs/assets/codeceremony-logo.svg" alt="CodeCeremony" width="320" />
</p>

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
| A SPA with a Node server | a second runtime, a second process | a Vite build, embedded with `go:embed` |
| OAuth / hosted auth | a hosted dependency | local accounts, opaque sessions |
| Redis | a second service | in-process fixed windows |
| A migration runner + SQL | nothing to apply it against | a documented, versioned snapshot format |
| A CSS framework | a package install | the design tokens already in `design.md` |

The result is a single static binary with one non-stdlib dependency
(`golang.org/x/crypto`, for bcrypt), and a frontend that is built in a separate
stage and embedded in it.

The SPA row is the one that changed under me, and it is worth being precise
about why rather than quietly editing the table. A frontend was originally
rejected outright on the reasoning that it means a Node toolchain in the
container. That reasoning turned out to be half right: a **Node server** at
runtime is genuinely unaffordable, but a Node **build** is not, as long as it
happens in a stage that never reaches the image. So the frontend exists, built by
Vite, with no Node in the runtime. Section 7 has the full reasoning.

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
    authz/              the action vocabulary, the resolver, and its ordering
    httpapi/            routing, authorization, JSON API, and the embedded app
    ratelimit/          fixed-window budgets
    mailer/             templates, queue, dispatch, webhooks
    config/             environment parsing and validation
    webassets/          brand assets, the legacy templates, and the built SPA
web/
  src/
    components/         the design.md section 7 inventory, as React
    apps/               one directory per application surface
    lib/                the typed API client, the session, the generated icons
    styles/             tokens, base, components, shell, apps
  public/               the brand assets, served from the build output
scripts/                the icon generator and the frontend build
tests/                  the e2e suite and the acceptance wrapper
```

Dependencies run one way: `domain` knows nothing, `store` and `judging` know
`domain`, `authz` knows `domain`, and `httpapi` knows everything. There is no
cycle, and `domain` has no imports from the project at all.

The frontend has no dependency on Go and Go has no dependency on the frontend
source: the build copies `web/dist` into `webassets/spa` and `go:embed` takes it
from there. That is what lets the Go tests run on a checkout with no Node
toolchain installed.

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

One place authenticates a request: `Server.requireAction`. It resolves the token,
loads the user, checks the account state, and re-reads the role **from the store
on every request** rather than from the token. Only the subject and session id
are trusted from the token, so a role change takes effect on the very next
request and a stale token can never be used to escalate.

```
route → requireAction(authz.Action…) → resolve target → decide → idempotent(handler)
                │                            │            │
                │                            │            └── denial is audited
                │                            └── from {slug} or ?event_id=
                └── principal onto the request context
```

### Actions, not permissions

Routes are gated on an **action** from a closed vocabulary in
`internal/authz/action.go`, not on a role. `results.read` is a thing to be
allowed; `organizer` is a label that happens to imply a bundle of them. Keeping
the two apart is what makes an explicit grant possible: you cannot grant a role,
only things.

The vocabulary is closed, and `authz` refuses an action it does not know rather
than defaulting to allow. `GET /v1/permissions` publishes the matrix by reading
the real maps, so the documentation cannot drift from the enforcement.

### Per-event scoping, enforced

An earlier version of this document described a known limitation: any user with
the global `organizer` role could act on any event by supplying its slug. That
was true when it was written and is not true now.

Every route declares the event it acts on, by `{slug}` in the path or by
`?event_id=`, and `routeTarget` resolves it into the authorization target
before any decision is made. Event roles are a **union** with global roles,
strictly scoped by `EventRoleFor == Target.EventID` — an organizer of one event
is refused on another, rather than being allowed by a global role and left for
each handler to fence. The event's creator is automatically
`EventRoleOwner`.

The resolver applies a fixed order, and the order is the design:

```
account state → target constraints → explicit deny → explicit grant
              → event role → global role → ownership
```

Deny before allow is the load-bearing part. A grant can add a permission; only a
deny can take one away, which is what makes an incident response a single API
call. Every decision carries a **reason** naming the rule that decided it, and
that reason is what lands in the audit trail.

Handlers still re-check against the object they load, because the route gate
resolved an event and not a row. `requireRouteEvent` catches a handler acting on
a different event than the path named.

### Explicit grants

`GET/POST /v1/grants` and `DELETE /v1/grants/{id}` manage per-user, per-action,
per-event allows and denies, with a mandatory reason and optional expiry.
Expired grants stop applying without anyone having to remember to revoke them,
and a deny is evaluated before any allow, so revoking is immediate.

An organizer can manage grants and audit exports for their own events; an admin
can across the portal. Both are fenced by the event on the target, so neither
reaches an event they do not hold.

### The audit trail is hash-chained

Every authorization decision is recorded: the action, the actor, the target, the
event, the outcome, the reason, and the rule that produced it. Entries are
chained — each carries the hash of the previous entry and an HMAC over its own
canonical contents — so an edit or a removal after the fact is detectable.

`GET /v1/audit/verify` reconciles the chain. `GET /v1/audit/actions.csv` exports
it, `prev_hash` included, so a third party can check that the log is internally
consistent without holding the key.

**The honest limit**: the chain is keyed with HMAC, so the export proves the log
is intact, and only the key holder can prove its *contents* are unedited.
Publishing the key would detect server tampering but not compromise of the
server itself. Retention trims the oldest entries; a retained chain verifies as
a **suffix**, and the response reports how many entries were dropped, so an
operator cannot present a truncated log as a complete one. A suffix cannot rule
out a rewrite of the discarded prefix — that data is gone. Anchoring the head
against an independently held copy is the mitigation, and that is what the
published head hash is for.

The key comes from `AUDIT_SECRET`, falling back to `SESSION_SECRET`. It is set
where the store is built rather than in the seed, because a restored boot skips
seeding entirely.

### Judge score isolation

The requirement is that one judge cannot read another's scores, enforced in the
backend. It is enforced **three times over**, deliberately:

1. `GET /v1/judge/scores` refuses a `?judge=` naming anyone but the caller, with
   an explicit 403. Defence in depth — the next line would already scope
   correctly.
2. The store call is `ReviewsForJudge(eventID, principal.UserID)`. The judge
   identity is **never read from the request**; it comes from the verified
   session. There is no code path where a request chooses whose scores it sees.
3. The organizer routes over reviews, results and exports require
   `results.read` or `export.full`, which the judge role does not hold.

There is no `/judges/{id}/scores` route and no per-judge resource of any kind.
`internal/httpapi/isolation_test.go` probes all of these, plus the neighbouring
leaks a `curl` would find: cross-judge assignment reads, judge address
disclosure, and the participant-as-judge case.

### Write safety

Two mechanisms, both opt-in, because a protection nobody uses protects nothing.

**Idempotency.** An `Idempotency-Key` on any unsafe method makes a retry safe.
The response is remembered and replayed byte for byte, so a retry cannot add a
second ballot, a second submission version or a second round of mail. A key
reused for a *different* body is a 422 rather than the first response, because
returning that would be a lie about the second request. Keys are scoped per
actor, so one caller cannot read another's cached response. A 5xx is not
remembered, since it may have partially applied.

**Optimistic concurrency.** `ETag` on reads, `If-Match` on writes, for the two
contended paths: editing a submission and saving a review. A 412 carries the
current ETag so a client can re-read and merge without a second round trip.
Unconditional requests still work.

The review ETag deliberately excludes `UpdatedAt`: including it would invalidate
the token even when a judge saved identical scores twice, and a client that
saves then saves again would be refused against its own last write.

---

## 6. Judging

See `JUDGING.md` for the method. Architecturally, `internal/judging` is a pure
package: it takes reviews and weights and returns a summary. It has no store
access, no clock, and no randomness — the bootstrap PRNG is a seeded
splitmix64 written out longhand precisely so the package stays pure and
reproducible.

Two estimators live side by side: the rubric pipeline (bounded, with confidence
intervals) and Bradley-Terry pairwise, which reports `unbounded: true` when the
panel is decisive enough that the maximum likelihood estimate does not exist.
That condition is detected structurally — some project never lost and some never
won — rather than from a threshold on the fitted spread, which does not fire for
a decisive panel whose truncated fit stays small and would fire for a
well-determined panel whose fit ran long.

Comparisons reach the estimator from two places. A judge can record a
**head-to-head verdict** directly (`POST /v1/events/{slug}/comparisons`), which is
attributed, timestamped and reversible. Where the panel answered enough verdicts
to stand on its own, the fit uses those; otherwise it derives comparisons from
rubric scores. The response reports which, in `source`, because a ranking built
from answers and one built from inference are different claims. A partial
recorded set is never blended into a derived one: that would change the question
being asked and make the ordering incomparable with the rubric pipeline.

One verdict per judge per pair. Re-answering updates the row rather than adding
one, because two rows for the same match would let a single judge contribute what
the estimator would read as two independent verdicts.

---

## 7. The frontend

A React and TypeScript single-page app, built by Vite and **embedded in the Go
binary** with `go:embed`, served from the same port as the API.

This is a reversal of an earlier decision, and the reason is worth recording
because the first answer was not wrong, just incomplete. The original choice was
server-rendered Go templates, on the reasoning that a separate SPA would put a
Node toolchain and a build step inside the container, which the one-command rule
cannot afford. That reasoning still holds — and it is why there is **no Node
runtime in the image**. What changed is where the build happens: the Node stage
exists in the Dockerfile, produces static assets, and the runtime stage is still
one static Go binary. `docker compose up` needs a network while the *image* is
built, never to *run* the portal.

### Why Vite and not Next

Next needs a Node **server** at runtime. That is two runtimes to operate, a
second process to supervise, and a `Content-Security-Policy` that could no
longer be `default-src 'none'`. Vite produces static files, so the Go binary
embeds them and stays the only process.

The cost is honest and worth stating: the public gallery is client-rendered, so
it is worse for search engines than a server-rendered page would be. For a
self-hosted portal whose audience arrives from a link an organizer sends rather
than from a search, that is the right trade — but it is a trade, not a free win.

### The applications

One directory per surface under `web/src/apps`: `auth`, `dashboard`, `events`
(public), `account`, `judge`, `organizer`, `admin`. Hiding a group in the
sidebar is a courtesy to the reader and **never** the control: every route
authorizes independently in the backend, so a viewer who types a URL they were
not offered is refused by the resolver rather than by the navigation. The judge
app in particular reads a session-scoped assignments endpoint rather than
fetching everything and filtering in the browser, which would ship a judge's
peers' work to their machine.

The API client and the server are checked against each other by a test that
reads both sources. That test exists because the client was written against the
documented routes and six of the paths it used did not exist; a wrong path is a
404 in production and nothing at all in development.

### design.md is enforced, not just referenced

The styles are the token layer transcribed from `design.md` clause by clause,
and 40 component tests assert the accessibility rules a unit test can actually
prove — each test naming the clause it comes from. Three of them failed on first
run and found real gaps. A source scan rejects any icon name outside the approved
Lucide set, which catches a glyph that would render as nothing at all.

### The legacy pages

Nine server-rendered templates remain and are still routed, so the portal is
useful on a checkout where the frontend has not been built and `go test ./...`
works with no Node toolchain installed. They are being retired app by app; the
build output wins where both exist.

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

Three suites, because they catch different things.

- **Go**, standard library only. `go test ./...` runs nine packages that carry tests; the other three are type-only.
- **Vitest**, 40 tests over the component library and the design contract.
- **e2e**, `tests/e2e.py` builds the binary, boots it on a scratch port and runs
  142 assertions against a real process, because several of the things below only
  exist or only fail across a real connection.

The frontend and the backend are additionally checked against each other: a test
reads the Go router and the TypeScript client and fails if the client asks for a
path the server does not register, or uses the wrong method on one it does. That
test exists because the client was written against the documented routes and six
of the paths it used did not exist.

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
- `internal/httpapi/audit_coverage_test.go` — four load-bearing handlers
  (submission create, review save, staff add and remove, sign-in and sign-out)
  that recorded nothing, because the audit helper wrote to a different table than
  the one the chain reads.
- `internal/store/action_audit_test.go` — a chain that could never verify after
  retention trimming, because verification was anchored on genesis and the oldest
  entries were gone.
- `internal/judging/pairwise_test.go` — the decisive-panel warning firing on
  every response, because both sentinels for the strength spread were seeded so
  that neither could ever update. The existing test for that warning had been
  passing for that reason and was not testing the heuristic.
- `internal/httpapi/concurrency_test.go` — a review ETag that included
  `UpdatedAt`, so a judge saving identical scores twice invalidated their own
  token and a save-then-save client was refused against its last write.
- `web/src/test/components.test.tsx` — a field error that was visible but never
  announced, a skeleton whose bars were hidden by a wrapper rather than
  individually, and two icon names that are not in the approved Lucide set and
  therefore rendered as nothing at all.

`AUDIT_SECRET` is the newest configuration variable and has a fallback worth
knowing about; see §12.

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

### AUDIT_SECRET

Keys the HMAC chain on the action audit. It defaults to `SESSION_SECRET` so a
working portal does not need a second variable, and it is separate so an operator
can rotate sessions without invalidating the audit. Boot logs a warning when it
is derived, because the consequence of not knowing is that rotating
`SESSION_SECRET` later silently invalidates every historical audit entry, and
that is discovered by watching verifications fail rather than by reading a
release note.

### The rest

| Variable | Default | What it does |
|---|---|---|
| `APP_ENV` | `development` | `production` turns on the guard rails below |
| `HTTP_ADDR` | `:8080` | listen address |
| `PUBLIC_PORT` | `8080` | host-side published port, read by compose |
| `DATA_DIR` | `./data` | the one state file lives here |
| `PERSIST_INTERVAL_SECONDS` | `5` | how often the journal rewrites it |
| `FIXTURES_PATH` | empty | shared fixture file; absent means the built-in seed |
| `SEED_DEMO_DATA` | `true` | mint the fixed demo tokens |
| `SEED_PASSWORD` | `codeceremony-dev` | shared password for every seeded account |
| `SESSION_TTL_HOURS` | `12` | session lifetime |
| `ALLOWED_ORIGIN` | `http://localhost:3000` | CORS origin, for a split dev frontend |
| `APP_BASE_URL` | — | absolute links in mail |
| `SMTP_*` | empty | `HOST`, `PORT`, `USERNAME`, `PASSWORD`, `ENCRYPTION`, `FROM`, `FROM_NAME`; mail falls back to a log sender when unset |
| `MAIL_MAX_ATTEMPTS` | `4` | delivery retries |
| `MAIL_INTERVAL_SECONDS` | `10` | dispatch interval |
| `MAIL_BATCH_SIZE` | `25` | deliveries per tick |

Idempotency records are **not** configurable. The retention window is a day,
which is long enough to cover a client retrying after a weekend and short enough
that the table stays small. `IdempotencyStats` reports the tracked and pending
counts for an operator who needs to reason about it.

### Build order, and what a checkout without Node produces

`scripts/build_frontend.sh` installs, typechecks, tests and builds the frontend,
then stages the output into `backend/internal/httpapi/webassets/spa` for the
`go:embed`. The Dockerfile does the same in a separate stage, so the image is
built from the repository root.

With no build staged, the Go binary still serves the JSON API and the nine
server-rendered pages, and `go test ./...` passes with no Node toolchain
installed. The API must not depend on a frontend build to be testable, and a
developer reading `THREAT-MODEL.md` should be able to reproduce a finding without
installing a package manager.
