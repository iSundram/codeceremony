# tests/

Two suites, different jobs.

## `e2e.py` — this one

A cross-process suite. It builds the binary, boots it on a scratch data
directory seeded from the shared DOGFOOD fixtures, drives the whole product
lifecycle, and tears it down.

```bash
python3 tests/e2e.py                  # build, boot, run, tear down
python3 tests/e2e.py --keep           # leave the portal running
python3 tests/e2e.py --base-url URL   # test an already-running portal
```

Standard library only. 66 assertions covering:

- **the public surface** — gallery, search, track filter, a closed event
  refusing submissions
- **role isolation** — a judge reading their own scores and nobody else's,
  through all seven score-bearing routes; a participant not being a judge; the
  assignment list narrowing to the caller
- **privacy** — judge addresses withheld from anonymous callers, the webhook
  list not public
- **organizer tooling** — CSV export, the results method and version, the
  published rubric actually being the one used, low-information judges reported,
  the fixture's duplicate detected
- **the lifecycle** — edit a draft, submit it, score it, publish, withdraw,
  with a submitted review refusing to change in between
- **voting** — ballot ordering randomized per voter but stable for one, the
  per-voter choice cap enforced
- **abuse controls** — login rate limiting, a review refused against a foreign
  event
- **durability** — the snapshot is versioned, carries the data, and contains no
  password hash

## `backend/**/*_test.go` — the unit suite

Go's standard library only, discovered by `go test ./...`. Tests live beside the
code they cover.

```bash
cd backend && go test ./...
```

These are not coverage for its own sake. Each one exists because something was
wrong at some point during the build:

- a snapshot that carried users but not events, reporting a successful restore
  and booting with no data at all
- float sums differing in the last bit because criteria were iterated in
  randomized map order
- a neutral imputation left at Go's zero value
- a separability flag assigned to a struct copy instead of the slice element
- a weight distribution producing a total of 125 for three criteria
- a fixture loader that assigned the roster from the wrong source and left the
  organizer with an empty judge panel

## The DOGFOOD acceptance checker

Neither of the above is the acceptance checker. That is the organisers' own
`run.py`, and its output is committed as `../acceptance-report.txt`.
