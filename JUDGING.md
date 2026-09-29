# JUDGING.md — assignment, scoring, and the normalization method

<p align="center">
  <img src="docs/assets/codeceremony-logo.svg" alt="CodeCeremony" width="320" />
</p>

This document is the argument for how CodeCeremony turns raw review scores into a
standing. It covers the rubric, how judges get work, the arithmetic, the evidence
that the arithmetic is trustworthy, and — at some length — what the method
cannot do.

Everything here is verified against the shared DOGFOOD fixture data: 41
projects, 30 judges, 126 reviews, 8 tracks. The numbers quoted are produced by
the portal, not estimated.

---

## 1. What problem this solves

Three platforms claim to normalize scores. None of them, as far as we could find,
publishes the formula, and none of them report whether the resulting order is
distinguishable from noise.

The fixture data makes the problem concrete. Across 126 reviews:

| Criterion | Min | Max | Mean | Std dev |
|---|---|---|---|---|
| functionality | 2 | 5 | 3.5714 | 1.0796 |
| quality | 2 | 5 | 3.5794 | 1.1364 |
| innovation | 2 | 5 | 3.5476 | 1.0955 |

Judges are not calibrated. Of 30 judges, **8 wrote fewer than three reviews**, so
their personal scale is barely estimable. **13 of 90 (judge, criterion) pairs have
zero variance** — that judge gave every project the same score for that
criterion. Project review counts range from 2 to 5. Seven judges are flagged
low-information by the method described below.

Most importantly: **41 projects are packed into a raw-mean range of 2.90 to
4.38, and the median gap between adjacent ranks is 0.025 on a five-point scale.**
Drop 25% of the reviews at random and the rank order agrees with the original
only 62% of the time.

A leaderboard printed from this data without an uncertainty statement is
asserting a precision the panel did not produce. That is the gap this method is
built to close.

---

## 2. The rubric

An organizer publishes a rubric: a set of criteria, each with a score range, a
weight, and whether it is required. Weights must be positive integers summing to
exactly **100**; the store rejects anything else.

The rubric is **versioned and event-scoped**. A review records the rubric id and
version it was written against, so a rubric change mid-event does not silently
reinterpret reviews already collected. Published rubrics are immutable; changing
one means publishing a new version.

Active rubric selection (`Store.activeRubricLocked`) prefers a track-specific
rubric over an event-wide one, and the higher version between equals.

> **A bug worth recording.** The fixture loader originally created the rubric
> with an empty `EventID`. Because active-rubric lookup matches on event, that
> rubric was never active — and because `eventWeights` falls back to a hardcoded
> 40/35/25 when no rubric is found, every result was silently computed against
> hardcoded weights while the API reported `weight_source: published_rubric`.
> An organizer could publish a 90/10 rubric and see no change in the standings.
> The rubric now carries its event, and the results response reports which
> rubric produced the numbers so this cannot be invisible again.

---

## 3. Assignment

An assignment is the record that a judge was given a project. It is what makes
`IsAssigned(event, judge, project)` true, and that check gates the only route
that writes a review.

Three strategies:

- **manual** — the organizer names the pairs.
- **balanced** — spread judges across tracks by profile, then round-robin by
  current workload.
- **batch** — assign whole batches at once, which is how a real event runs:
  everyone gets a set, and the set is the unit of reminder email.

Constraints enforced by the store, not by the handler:

- A judge must be on the event roster.
- A judge must be active.
- The judge must be scoped to the project's track.
- `(event, judge, project)` is unique, and a revoked assignment frees the slot
  for reassignment.
- A declared conflict revokes the matching assignment in the same locked
  operation, so a judge cannot be left holding a project they declared a
  conflict on.

---

## 4. The normalization method

`internal/judging/judging.go`. Reported method id: `judge-zscore-shrink-logistic`,
version `1`. The version is published in every results response so an archived
result set can be recomputed under the arithmetic that produced it.

### Step 1 — calibrate each judge against themselves

For each judge *j* and criterion *c*, take that judge's own scores across every
project they reviewed and compute the mean μ and population standard deviation
σ. Express each score as

```
z = (s − μ) / σ
```

which reads as: how far does this project sit from where **this judge** normally
lands on **this criterion**?

This is the step that removes the judge effect. A judge who awards 5s generously
and one who awards 3s generously are placed on the same footing, so aggregation
measures projects rather than temperaments.

### Step 2 — shrink thin evidence

A judge with one review, or whose scores barely vary, produces a z-score that is
mostly noise. Multiply by a reliability factor

```
λ = n / (n + k),   k = 2
```

where *n* is that judge's review count on that criterion.

- n = 1 → λ = 0.33
- n = 2 → λ = 0.50
- n = 3 → λ = 0.60
- n = 11 → λ = 0.85

**Why shrinkage instead of a minimum-review cutoff.** The obvious alternative is
"ignore any judge with fewer than three reviews". It is worse in two ways: it
throws away a judge's entire contribution at a threshold, creating a cliff in the
ranking where one extra review changes the result discontinuously; and it is
arbitrary at the boundary. Shrinkage is smooth, it is one parameter, and a judge
who writes four reviews contributes 67% rather than 0% then 60% then 100%.

A judge with **zero variance** on a criterion is a special case: there is no
spread to divide by. That criterion is set to the neutral value and the judge is
flagged. Seven of the thirty fixture judges are flagged this way or for having a
single review.

### Step 3 — squash to a bounded 0–100 scale

```
value = 100 / (1 + exp(−λz))
```

The logistic is strictly increasing, so **it never reorders anything within a
single judge**. It is included for two reasons:

- **Boundedness.** A judge using a 1–5 range with σ = 0.2 would otherwise
  produce |z| up to 20 and dominate the panel through magnitude alone. The
  squash caps that at 0–100.
- **Interpretability.** 50 is exactly this judge's own average on this
  criterion. A judge scoring a project at their own mean contributes exactly
  neutral, and the console can say so.

### Step 4 — weight and aggregate

Per review, combine criteria with the rubric weights. Per project, average
reviews with **equal weight per judge**: every judge is one voice regardless of
how many projects they happened to be assigned.

### Step 5 — missing criteria

A review that omits a criterion is **imputed at that judge's own mean**, which is
z = 0, which is exactly 50 — neutral. The review is flagged `partial` with the
missing keys listed.

> An earlier version returned an error for any review missing a weighted
> criterion. That meant one judge skipping one criterion made the entire results
> endpoint fail closed. Partial reviews are the normal state of a live panel:
> the fixture deliberately includes judges who never finished their batch.

### Step 6 — uncertainty

Judges, not reviews, are the independent unit. Reviews written by one judge are
correlated **by construction** — that correlation is precisely what step 1
removes — so resampling individual reviews would report intervals far too narrow
and would be measuring the wrong thing.

So the interval is a **bootstrap over judges**: resample the panel with
replacement, rerun steps 1–5, repeat 2000 times, and take the 5th and 95th
percentiles of each project's normalized mean.

A judge drawn *k* times contributes *k* copies of their whole review block, not
*k* random rows, so the judge effect stays inside the resample.

The bootstrap is seeded from a fixed constant and the PRNG is a hand-written
splitmix64, not `math/rand`, whose stream is not contractually stable across Go
releases. A results endpoint whose intervals moved on every refresh could not be
audited, and a recomputation could not reproduce a published number.

### Step 7 — report separability, not just rank

Two projects are **separated** when the lower one's interval lies entirely above
the upper one's. A two-sided interval overlap test, so a project far below its
neighbour is separable even if its own interval is wide.

```
separable(i)  ⇔  not ( low(i) ≤ high(i−1)  ∧  low(i−1) ≤ high(i) )
```

**Separability is a statement about one adjacent pair.** It is not a clique. A
chain of overlapping neighbours can still contain a project that is plainly
ahead of the bottom of the table, and reading a long chain of equal `tie_group`
values as "these are all equivalent" would overstate the data. The API therefore
publishes both the pairwise flag and `previous_rank`, the rank the project failed
to beat, and the console words it that way.

---

## 5. The result on the fixture data

Method `judge-zscore-shrink-logistic` v1, 90% interval, 2000 resamples:

| Rank | Project | Normalized | 90% interval | Raw | Reviews | Separated |
|---|---|---|---|---|---|---|
| 1 | prj_34 | 63.7 | [58.9, 69.1] | 4.38 | 3 | yes |
| 2 | prj_37 | 58.7 | [36.4, 68.8] | 4.06 | 4 | no |
| 3 | prj_33 | 58.5 | [57.6, 60.5] | 3.98 | 3 | no |
| 4 | prj_16 | 58.3 | [51.2, 70.0] | 3.93 | 3 | no |
| 5 | prj_11 | 58.0 | [53.7, 65.6] | 4.41 | 4 | no |
| 6 | prj_10 | 57.7 | [55.3, 61.9] | 4.15 | 2 | no |
| … | | | | | | |
| 40 | prj_23 | 38.1 | [25.8, 45.8] | 2.88 | 3 | no |
| 41 | prj_05 | 37.4 | [29.2, 43.7] | 2.97 | 3 | no |

**1 of 41 projects is separated from the project above it.**

Two things are worth noticing. First, prj_34 at rank 1 is genuinely ahead — its
interval clears prj_37's. Second, note prj_11: it has the **highest raw mean of
any project at 4.41** and sits fifth, because its reviewers were severe on
average. That is the method doing its job. The raw column is still reported
alongside, because an organizer who wants to argue about the raw ranking should
be able to see it and argue about it.

Judge calibration, as the console shows it:

| Judge | Reviews | Reliability | Low information |
|---|---|---|---|
| jdg_24 | 11 | 0.85 | no |
| jdg_26 | 10 | 0.83 | no |
| jdg_29 | 9 | 0.82 | no |
| jdg_02 | 6 | 0.75 | no |
| jdg_07 | 3 | 0.60 | **yes** — same score for every project |
| jdg_01 | 1 | 0.33 | **yes** — one review |

---

## 6. What this method does not claim

Stated plainly, because the absence of these claims is the point.

1. **It does not correct for genuine disagreement.** If the panel splits because
   two judges value different things, normalization does not resolve that. It
   removes scale differences, not taste differences.
2. **It does not manufacture signal.** A project with two reviews from two
   lenient judges has a wide interval, and that is the honest answer.
3. **It is not a significance test.** Separability is an interval-overlap
   heuristic, not a hypothesis test with a p-value. It is conservative by
   construction: it under-claims separation rather than over-claiming it.
4. **It does not weight judges by expertise.** A judge scoped to a track they
   know and one scoped to a track they do not get the same λ. Weighting by
   declared expertise is defensible and is not implemented; the field exists
   (`judge_profiles`, roster `expertise`) for it.
5. **Shrinkage assumes panel members are exchangeable.** That is a modelling
   choice, not a fact. It is reasonable for a panel of peers and questionable for
   a panel with a deliberate seniority mix.
6. **The 90% interval is not calibrated against ground truth.** There is no
   ground truth for "which project is better". The interval describes the panel's
   internal consistency, nothing more.

---

## 7. Reproducing the numbers

```bash
docker compose up
# then, as the organizer (see .dogfood.toml for the auth header)
curl -s -H "Cookie: session=cc_org_7f2a1b9d4e6c8a0f" \
  "http://localhost:8080/v1/organizer/results?event_id=evt_01" | python3 -m json.tool
```

The response carries `method`, `method_version`, `confidence`, `resamples`, a
per-judge `judges` array with reliability and reasons, `criterion_stats`, and a
per-project `separable` flag with its interval. The organizer console at
`/organizer/<event>/results` renders the same data with the interval bars.

The bootstrap is deterministic, so repeated calls return identical numbers. There
is a test asserting exactly that
(`TestNormalizeUncertaintyIsDeterministic`).

---

## 8. Pairwise comparison (Bradley-Terry)

`internal/judging/pairwise.go`. Method id `bradley-terry-mm`.
`GET /v1/organizer/pairwise?event_id=…`

### Why a second estimator

Asking a judge to score forty projects on three criteria, independently and
consistently, is a demanding thing. Asking "which of these two do you prefer?"
is not. The pairwise question needs no scale, no calibration between criteria,
and no memory of what you said about the last project — and it is what people
actually do when forced to rank things.

So the portal offers a second view built only on head-to-head verdicts.

### A second bug worth recording

The decisive-panel warning fired on **every** response, and its test passed the
whole time it was wrong.

The check compares the fitted strength spread against a threshold. Both sentinels
for that spread were seeded wrong: the maximum started at `+Inf` and the minimum
at `0`. A Bradley-Terry strength is always positive, so neither could ever
update — the maximum never moved because nothing is above infinity, and the
minimum never moved because nothing is below zero. The ratio was therefore always
infinite and the warning was always true.

The existing test asserted that a genuinely decisive panel sets the flag, and it
did — for the wrong reason. A warning that fires unconditionally is worse than no
warning, because a reader learns to ignore it within a day. The condition is now
detected **structurally**, from whether any project never lost and any never won,
which is exact rather than a proxy; and there is now a second test asserting that
a panel *with a cycle* is not flagged. A threshold that does not fire for a
genuinely decisive panel whose truncated fit happens to stay small is the same
error in the other direction.

### Recorded verdicts, and derived comparisons

A comparison can arrive two ways, and the response says which it used.

**Recorded.** A judge can answer the question directly:

```
POST /v1/events/{slug}/comparisons
{ "left": "prj_07", "right": "prj_19", "verdict": "left" }
```

`verdict` is `left`, `right` or `tie`. Both projects must be assigned to the
judge, so a judge cannot rank a field they were not given. The verdict is
attributed, timestamped, and withdrawable, and a withdrawal is in the audit trail
with the verdict it withdrew — the record of the mistake survives even though the
verdict does not.

One judge has one answer per pair. Re-answering, including with the two sides
swapped, **updates the same row**. Two rows for one match would let a single
judge contribute what the estimator reads as two independent verdicts, and would
weight that match as if two judges had played it. The stored row keeps the
orientation the judge last used, so a change of mind is visible rather than a
silently different record.

**Derived.** The fixture file contains rubric scores, not explicit verdicts, so
where the panel has not answered enough, comparisons are derived: for each judge,
every pair of projects that judge reviewed becomes one comparison, won by the
higher weighted score. A judge who scored A at 4 and B at 3 has, on their own
scale, said A beats B. A review missing a weighted criterion is excluded from
head-to-head comparison rather than compared on the criteria it happens to carry,
because that would be a different question from the one being asked.

The view reports `source` (`recorded` or `derived`) and splits the total into
`recorded_comparisons` and `derived_comparisons`, because a ranking built from
answers and one built from inference are different claims.

A recorded set is used only when it covers **the same projects** the derived fit
did. A partial set is never blended into a derived one: dropping the derived
comparisons it does not cover would change the question being asked, and the
resulting ordering would no longer be comparable with the rubric pipeline. In
that case the view falls back to derived and says so.

**Why the split matters.** A derived comparison is a chain of inferences: a
4-versus-3 becomes a win, and that win is read as a preference the judge
actually expressed. It is usually right. It is not the same kind of evidence as
being asked which of two you prefer, and a panel that disagrees with itself across
the two is telling you something about how absolute scales are being used.

### The model and the fit

P(i beats j) = π_i / (π_i + π_j), fitted by the MM algorithm of Hunter (2004),
which is the standard estimator: each update is the maximiser of a concave
minorant of the negative log-likelihood, so the sequence decreases the objective
monotonically and cannot diverge the way gradient ascent on a log-ratio
parameterisation can.

Two details that matter:

- **Scale.** The model is only identified up to a common factor. Strengths are
  normalised so their geometric mean is 1, which makes them readable as "times
  as strong as the average project".
- **Ties.** A tied comparison contributes half a win to each side. Without that,
  an undecided comparison would systematically treat the two projects as if it
  had never happened, and the estimator's total weight would no longer equal the
  number of games played. The half-win is **not** counted toward the reported win
  rate, which is a proportion of decided comparisons. A record of nothing but
  draws has no decided comparisons, so its `win_rate` is null rather than 1.
- **When the estimate does not exist.** If some project never lost and some never
  won, the strength ratio between them grows without limit and there is no finite
  maximum likelihood. That is detected from the tally — structurally — rather
  than from the size of the fitted spread, because a spread threshold misses a
  decisive panel whose truncated fit happens to stay small, and would fire for a
  well-determined panel whose fit ran long. The response sets `unbounded: true`
  and says in `note` that the numbers are a truncated fit rather than estimates.
  The ranking is still reported, because a decisive panel is a common and
  informative outcome; it is the *strengths* that stop being measurements.

### The result on the fixture data

184 derived comparisons across 41 projects. Top of the pairwise table:

| Rank | Project | Strength | Wins | Losses | Comparisons | Win rate |
|---|---|---|---|---|---|---|
| 1 | prj_34 | 143.7 | 17 | 1 | 18 | 0.94 |
| 2 | prj_09 | 52.5 | 7 | 4 | 11 | 0.64 |
| 3 | prj_10 | 42.0 | 9 | 4 | 13 | 0.69 |
| 4 | prj_11 | 33.6 | 13 | 4 | 17 | 0.76 |

**prj_34 is first under both estimators.** That is a real cross-check: the two
methods share only the underlying reviews, and they disagree about the middle of
the table (prj_11 has the highest raw mean of any project in the fixture and sits
fifth on the rubric pipeline, but fourth here). prj_34 being agreed on is
genuine corroboration; the middle of the table being reordered is a statement
that 41 projects packed into a 1.5-point band are not separable, which is
exactly what the confidence intervals in section 5 already said.

The response also attributes comparisons per judge, so an organizer can see
whether one judge is driving an ordering. On this fixture, jdg_02 contributed 15
of 184 comparisons with a 0.50 win rate — unremarkable, which is itself worth
seeing.

### Where this estimator does not work, and says so

**This panel is decisive and the API reports it as unbounded.** At least one
project never lost and at least one never won, so the maximum likelihood
estimate does not exist: π is unbounded at the top and zero at the bottom. The
response carries `"unbounded": true`, a note explaining it, and `converged:
false` where the iteration budget ran out.

That matters more than the numbers. A large finite strength in that position is
decided by where the iteration happened to stop, not by the data, and a portal
that printed it without qualification would be reporting an artefact as a
measurement. So the fit says what is wrong and points at the rubric pipeline,
which is bounded by construction.

A related boundary case is handled explicitly: a project that wins nothing has a
maximum likelihood estimate of zero, so its strength is floored at `MinStrength`
rather than allowed to divide by zero downstream. Convergence is judged only over
projects the model can actually estimate, because a pinned project would
otherwise make the iteration count meaningless and hide a genuine failure to
converge on everything else.

### Which one should an organizer use?

**The rubric pipeline by default.** It is bounded, it reports confidence, and it
degrades gracefully. The pairwise view is the more interesting analysis and the
weaker estimator on a panel this small, and it is offered as a second opinion
with its own honesty flags attached rather than as an alternative ranking to
choose between at random.
