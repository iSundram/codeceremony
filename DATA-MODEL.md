# DATA-MODEL.md

The schema CodeCeremony actually runs, how it is stored, and how data moves in
and out.

> **This is the whole schema.** There is no SQL in this repository. An early set
> of PostgreSQL migrations existed and was removed: nothing ever read, embedded
> or executed it, it had drifted from the code, and it contained defects that
> would abort a real migration. `backend/migrations/README.md` records why.
> What runs is the Go types in `internal/domain`, persisted as the snapshot
> format described in section 3.

---

## 1. Entities and relationships

The tenant key is `event_id`. It is present on most, but deliberately not all,
tables — the reasoning is in section 2.

```
                          ┌──────────────┐
   users ────┬───────────▶│   events     │  tenant root
             │            │   evt_01     │
             │            └──────┬───────┘
             │                   │
             ├── user_profiles   ├── tracks ──────────────┐
             ├── judge_profiles ─┤                          │
             │        │          ├── prizes                 │
             │        └── judge_track_scopes ───────────────┤
             │                   │                          │
             ├── team_members ── teams                      │
             │                   │      ├── team_invites    │
             │                   │      ├── duplicate_flags │
             │                   │      └── submissions ────┘
             │                   │            │
             │                   │            ├── submission_versions
             │                   │            └── submission_answers
             │                   │
             ├── participations  ├── rubrics ── rubric_versions ── criteria
             │                   │                      │
             │                   ├── assignments ───────┘
             │                   │          │
             │                   │          └── reviews ── review_scores
             │                   │                  │
             │                   │                  └── normalized_scores
             │                   │                          │
             │                   │                   normalization_runs
             │                   │
             │                   ├── comments ── comment_reports
             │                   ├── vote_campaigns ── ballots ── votes
             │                   ├── certificates
             │                   ├── webhooks ── webhook_deliveries
             │                   ├── activity_entries
             │                   ├── audit_events
             │                   └── result_publications
             │
             └── sessions, notifications, mail_preferences, security_events
```

### The core tables

| Table | Purpose | Key invariants |
|---|---|---|
| `users` | accounts | one global `role`; `email` unique; soft-deleted via `state` |
| `events` | the tenant root | 8 lifecycle timestamps; `state`; `results_visible` |
| `tracks` | competition categories | unique `(event, slug)` |
| `prizes` | awards | `rank`; optionally track-scoped |
| `teams` | participant groups | `captain_id` set-null on user delete; `eligibility_status` |
| `team_members` | membership | PK `(team_id, user_id)`; `membership_status` |
| `submissions` | projects | `team_id` and `track_id` both `ON DELETE RESTRICT`; versioned |
| `submission_versions` | append-only history | unique `(submission_id, version)` |
| `rubrics` / `rubric_versions` / `criteria` | the scoring scheme | weights sum to exactly 100; versions immutable once published |
| `assignments` | who judges what | unique `(event, judge, project)`; revoked frees the slot |
| `reviews` | scores | unique `(event, judge, project)` — one review per judge per project |
| `review_scores` | per-criterion marks | `CHECK (score BETWEEN min AND max)` |
| `normalization_runs` | reproducibility | `method_version`, `weights`, `input_hash` |
| `normalized_scores` | the computed result | `raw_score`, `normalized_score`, `low_information`, `details` |
| `result_snapshots` | published standings | immutable |
| `comparisons` | recorded head-to-head verdicts | one per `(judge, {left, right})` unordered; `verdict` in {left, right, tie} |
| `grants` | explicit allows and denies | `(user, action, event, object)`; mandatory `reason`; optional expiry |
| `action_audit_entries` | the accountability record | append-only, hash-chained, `seq` contiguous within the retained window |
| `audit_events` / `activity_entries` | the trail | actor set-null on delete, so history survives |

Two of these deserve a note.

**`comparisons` is keyed on the unordered pair.** One judge has one answer per
pair of projects, and re-answering — including with the two sides swapped —
updates the row rather than adding one. Two rows for one match would let a single
judge contribute what the estimator reads as two independent verdicts. The row
stores the orientation the judge last used, so a change of mind is visible in the
data rather than hidden in a normalisation step.

**`action_audit_entries` is a chain, not a log.** Each entry carries `seq`,
`prev_hash` and `hash`, where the hash is an HMAC over the entry's own canonical
contents. An edit or a removal after the fact breaks the chain from that point
on, which is what makes the trail evidence rather than a record. Retention trims
the oldest entries; the retained window verifies as a **suffix**, and `dropped`
is reported so a reader knows the chain they hold is not the whole history. A
suffix cannot rule out a rewrite of the discarded prefix, because that data is
gone. The published `head` exists so a verifier can anchor against a copy they
hold independently.

### The two tables worth defending

**`normalization_runs`.** Versioned method, the exact weights used, a hash of
the input, and start/completion timestamps. Without it a published leaderboard
cannot be reproduced or explained six months later. The Go implementation
reports `method` and `method_version` in every results response for the same
reason, and the bootstrap is seeded from a fixed constant so repeated calls
return identical numbers.

**`review_scores` as a child of `reviews`, not a column on it.** A criterion is
organizer-defined data, not a schema change. Adding "accessibility" to a rubric
must not require a migration, and must not change what "5" means for any other
criterion.

---

## 2. Why `event_id` is not on every table

`event_id` appears on three tiers, and the split is a decision rather than an
oversight:

1. **Directly scoped** — an `event_id` column, `ON DELETE CASCADE`. Most tables.
2. **Conditionally scoped** — a nullable `event_id` for platform-level records
   that *may* attach to an event: `audit_events`, `security_events`,
   `permission_overrides`, `mail_messages`.
3. **Global, scoped transitively** — `users`, `sessions`, `user_profiles`,
   `judge_profiles`, `team_members`, `rubric_versions`, `criteria`,
   `review_scores`, `normalized_scores`. These have no `event_id`; they reach an
   event through a parent.

The consequence is that **there is no database-level tenant isolation for
scores.** `review_scores` is reachable from any event via `reviews`. The
isolation is the application layer's job, and the application enforces it by
never joining across an event boundary — concretely, by refusing to create a
review whose event is not the event its project belongs to.

That check is not optional. Without it a caller could stamp any event id onto a
review, and because every read of reviews filters on the event id, the row would
disappear from the leaderboard of the event it belongs to and appear in the
results of one it has nothing to do with. It is enforced in
`Store.SaveReview` and again in `Store.CreateComment`.

---

## 3. The snapshot format

All durable state is one JSON document. It is versioned, and a mismatch is
refused rather than silently coerced.

```jsonc
{
  "version": 1,
  "saved_at": "2026-09-29T04:12:03Z",

  "users": [ /* … */ ],
  "events": [ /* … */ ],
  "tracks": [ /* … */ ],
  "teams": [ /* … */ ],
  "team_memberships": [ /* … */ ],
  "submissions": [ /* … */ ],
  "assignments": [ /* … */ ],
  "reviews": [ /* … */ ],
  "rubrics": [ /* … */ ],
  "duplicate_flags": [ /* … */ ],
  "comments": [ /* … */ ],
  "ballots": [ /* … */ ],
  "comparisons": [ /* … */ ],
  "grants": [ /* … */ ],
  "activity_entries": [ /* … */ ]
  // … 32 collections in total
}
```

### Rules the format guarantees

- **Every collection is sorted by its own key.** Two snapshots of identical state
  are byte-identical. This is what makes the journal's digest comparison
  meaningful, and it is asserted in a test.
- **`saved_at` describes the file, not the state.** Stamping it at capture time
  made every poll look like a change and rewrote the file every two seconds.
- **Password hashes are never written.** `Snapshot` zeroes them, and `Restore`
  refuses a document that contains one, so a hand-edited or tampered file cannot
  smuggle a credential into an account. A backup is the thing most likely to be
  copied around, and a hash is a credential.
- **Sessions are absent.** A restart signs everyone out. See `ARCHITECTURE.md`
  section 4 for why that is the right trade.
- **Composite keys are stored as objects, not concatenated strings**, so the
  format stays readable and a key cannot be forged by putting a colon in an id.
- **Grants are persisted, the action audit chain is not.** An explicit allow or
  deny is a decision someone made and relies on, so losing it on a restart would
  silently change who can do what. The chain is evidence about the past, and it
  is rebuilt empty on boot; restoring it from a snapshot would mean a snapshot
  could rewrite history, which is the opposite of what a chain is for. The same
  reasoning puts idempotency records in memory: a forgotten key risks a
  duplicate, a persisted one would make a key mean different things before and
  after a restart.

### Operational use

```bash
# back up
cp data/portal.json backup/portal-$(date +%F).json

# restore
docker compose down && cp backup/portal-2026-09-29.json data/portal.json
docker compose up

# reset a demo portal
docker compose down -v && docker compose up
```

### Moving to a real database

The snapshot is the migration path in. Every collection is a table, every field
is already named, and `Restore` is a working reference implementation of the
insert order. Moving to PostgreSQL is writing a loader that walks the snapshot
and issues the inserts, plus applying a real schema. The reverse direction is the
export endpoint, which emits the same document shape.

`internal/httpapi/portability.go` implements export and import over the same
structure, and `GET /v1/organizer/export` is the canonical way to get data out
of a running portal.

---

## 4. Import and export paths

| Path | Shape | Who |
|---|---|---|
| `GET /v1/organizer/export` | full event as JSON | organizer, admin |
| `GET /v1/organizer/export.csv` | one row per review | organizer, admin |
| `POST /v1/organizer/import` | accepts the export shape | organizer, admin |
| `GET /v1/organizer/duplicates` | duplicate flags | organizer, admin |
| `POST /v1/organizer/duplicates/scan` | rescans the event | organizer, admin |
| `fixtures.json` via `FIXTURES_PATH` | the shared DOGFOOD file | any seed |

### The fixture file is input, not a schema

`fixtures.json` uses its own flat shape — an `event` object plus arrays of
`tracks`, `judges`, `teams`, `projects`, `scores`, with judges and team members
identified by email and no rubric anywhere. The loader in `internal/fixtures`
translates it:

| Fixture shape | Becomes |
|---|---|
| `event` | the `Event`, with `submissions_close` driving the lifecycle state |
| `tracks[]` | `Track` rows; the loader is authoritative for the track list |
| `judges[]` | `User` (role `judge`) + `JudgeProfile` + `JudgeRosterEntry` |
| `teams[].members[]` (emails) | `User` (role `participant`) with an id derived from the address + `TeamMembership` + `Participation` |
| `projects[]` | `Submission`, status `submitted`, eligibility `eligible` |
| `scores[]` | `Review` **and** the `Assignment` that authorises it |
| — | a `Rubric` derived from the criteria actually present, weights totalling 100 |
| — | `DuplicateFlag`s for any repeated team/title/repo |

Two things about that mapping are worth stating:

**Score rows become assignments.** The store refuses a review from a judge who
was not assigned the project, so the loader synthesises the assignment rows.
Without them the fixture panel would be internally inconsistent and the judge
console would show an empty batch.

**The fixture supersedes the built-in demo panel.** The built-in seed carries two
demo judges so a clone with no fixture file still works. When fixtures load,
those two are retired along with their profiles, roster entries, assignments and
reviews. Leaving them in place is worse than having no demo at all: they share
the demo email addresses, so signing in as the documented judge would
authenticate a judge with an empty batch while the real panel sat unreachable.
That bug existed and is why the loader filters.

**Team member ids are derived, not assigned.** Fixtures identify members by
email only, so the id is `usr_` plus the first twelve hex characters of
SHA-256(address). Deterministic, collision-resistant, and identical on every
machine — which matters because those ids end up in membership rows and in the
public gallery.

---

## 5. The awkward cases, on purpose

The shared fixture deliberately contains cases a hand-written demo seed always
smooths away. The portal is expected to handle them, and does:

| Case | In the fixture | How the portal handles it |
|---|---|---|
| A judge who rated every project identically | `jdg_07` | flagged low-information, their scores land on exactly neutral rather than dividing by ~zero |
| Judges with too few reviews to have a scale | 8 of 30 have < 3 | shrunk by `n/(n+k)`, flagged, counted rather than discarded |
| Ragged review counts | 2 to 5 reviews per project | per-project means; no project dropped |
| A duplicate submission | `prj_41` repeats `prj_07`'s team, title and repo | flagged at seed time, visible in the organizer queue on first boot |
| A partially finished panel | 2 of 30 judges wrote nothing | no error; they contribute nothing |
| Empty review comments | 51 of 126 | not a required field |

The rubric is derived from the criteria the fixture actually uses, so a fixture
with a different criterion set still produces a valid rubric with weights
totalling exactly 100. A hand-rolled distribution produced 40/70/15 — a total of
125, which the rubric validator rejects — before it was replaced with
largest-remainder apportionment.

---

## 6. Deletion

There is no hard delete anywhere in the system.

- **Users** are soft-deleted: `state` moves to `deletion_pending` or `suspended`.
  Account state is the **first** thing the resolver checks, so a pending-deletion
  account is refused every action except the one that cancels the deletion, and
  that exception is expressed as an allow in the same table rather than as a
  special case in the middleware.
- **Teams** are archived, with `deleted_at` set and memberships cascaded.
- **Events** are not deleted at all.
- **Comparisons** are withdrawable rather than deletable, and a withdrawal is
  written to the audit chain with the verdict it withdrew. A judge who recorded
  the wrong answer should be able to take it back; the record that they were
  wrong, and when they corrected it, should outlive the mistake.
- **Grants** are revoked rather than deleted, and an expired grant stops applying
  on its own, so a lapsed delegation withdraws itself without anyone having to
  remember it existed.

This is deliberate for a judging platform. A submitted project, a written review
and a published result are the evidentiary record of a decision that may be
disputed weeks later. Hard-deleting any of them would make the portal unable to
answer the question it exists to answer.

Activity and audit entries keep a denormalised `actor_name` and `actor_role`
specifically so the trail survives the deletion of the account that generated
it.
