# THREAT-MODEL.md

A written defence against Sybil votes, ballot stuffing, collusion, and the rest.

The threat model of a hackathon judging platform is not a bank. Nobody is trying
to steal money. What an attacker wants is to **change who won**, and the
interesting attacks are all about manufacturing or suppressing a vote rather
than about exfiltrating data. That framing drives everything below.

Two things about the threat model are worth stating before the table, because
they change what "mitigated" means here:

- **The judge panel is the crown jewel, and it is authenticated.** A judge
  account is issued to a named person by an organizer. The primary attack is
  therefore not "become a judge" but "move a project's score after seeing the
  standings", or "read what other judges said and conform to it".
- **The public surface is large and anonymous.** The gallery, comments and
  community voting are reachable by anyone. That is where volume attacks live.

Trust boundaries: **anonymous internet → portal**, **signed-in participant →
portal**, **judge → own reviews**, **judge → other judges' reviews (forbidden)**,
**organizer → everything in their event**, **admin → the platform**.

---

## 1. The attacks, and what stops each

### A1 — Peer score disclosure (reading a rival's review)

*Threat.* Judge B reads judge A's scores before submitting, and conforms to them.
This is collusion by information rather than by communication, and it is the
single highest-value attack on judging integrity.

*Defence, three layers.* `GET /v1/judge/scores` refuses a `?judge=` naming
anyone but the caller with an explicit 403. The store call is
`ReviewsForJudge(eventID, principal.UserID)` — the judge identity is never read
from the request, it comes from the verified session, so there is no code path
where a request chooses whose scores it sees. And every all-judges endpoint
(`/organizer/reviews`, `/results`, `/export`, `/export.csv`) requires
`ViewPeerScores` or `ExportData`, which the judge role does not hold. There is no
`/judges/{id}/scores` route and no per-judge resource at all.

*Verified by.* `internal/httpapi/isolation_test.go` probes all seven score-bearing
routes as a non-owning judge, and asserts the response contains none of the peer's
review text.

*Residual.* An organizer can read everything, by design. And a judge who talks to
another judge in a hallway is outside any technical control; see D3.

### A2 — Score shopping (reading the standings, then moving a score)

*Threat.* Judge A scores a project poorly, sees the published standings, changes
the score, and re-submits.

*Defence.* **A submitted review is immutable.** `Store.SaveReview` refuses any
change to a review that is already submitted, and refuses a second submission
with different scores. Drafts are explicitly not locked, so the workflow of
draft-then-submit survives. The console shows a submitted review as locked rather
than as an editable form.

*Verified by.* `TestSubmittingTwiceWithDifferentScoresIsRefused` and the
end-to-end form path, which returns 409.

*Residual.* A judge who has not yet submitted can always see the current
standings. Mitigation is procedural: `results_published` is false until an
organizer publishes, and the default is that the panel is not published while
judging is open.

### A3 — Sybil judges

*Threat.* An organizer, a sponsor, or an entrant adds a judge they control, or
creates many accounts to outvote the panel.

*Defence.* Judges are **not self-service.** A judge is a user account added to an
event roster by a user holding `ManageEvent`, with an explicit track scope and a
capacity. Creating one is an authenticated, audited organizer action, not a public
endpoint. There is no "sign up as a judge" path anywhere in the router.

*Residual, and it is the real one.* **Nothing stops a hostile organizer from
adding judges they control.** No technical control can: the organizer *is* the
trust anchor for panel composition, and the platform has no view on whether a
person is a legitimate judge. What the portal can do is make the panel auditable —
every judge is named, every assignment is recorded, every score is attributable —
so a disputed result can be traced to the people who produced it. Judging panels
are a governance problem wearing a technical costume; the platform's job is to
make it visible.

*Partial mitigation available, not implemented.* Weighting by declared expertise
(`judge_profiles.expertise`, roster `expertise`) and capping a judge's
contribution are both supported by the data model and neither is implemented. See
`JUDGING.md` section 6.

### A4 — Ballot stuffing

*Threat.* One person casts a thousand community votes, or scripts the endpoint.

*Defence.* Three independent limits:

- **Authentication.** A ballot requires a session. There is no anonymous voting
  path, so the cheapest possible attack does not exist.
- **A per-user cap enforced in the store**, not the handler: `CastBallot` refuses
  when `existing + new > MaxChoicesPerUser`, under the store's write lock. Two
  concurrent requests cannot both slip past a check-then-act race in the handler.
- **A rate limit of 10 ballot actions per minute per account**, keyed on the
  account rather than the address — see section 3.

*Verified by.* `TestBallotStuffingIsRefused` and the rate-limit test.

### A5 — Fixed-order ballot bias (coordination channel)

*Threat.* The ballot always lists projects alphabetically, so a campaign tells its
audience "vote for everything from A to M", or colluders agree on a visible
pattern.

*Defence.* Ballot options are ordered by a keyed digest of `(campaign, voter,
project)`, so **different voters see different orders**. The order is
**deterministic per voter** — a digest, not a per-request random source — so a
voter cannot refresh until they get a favourable order, and the list does not move
under their cursor.

*Verified by.* `TestBallotOrderDiffersBetweenVoters` and
`TestBallotOrderIsStableForOneVoter`.

*Why a digest rather than a seeded PRNG.* A PRNG draws depend on the order it is
fed, so adding or removing a project reshuffles every other voter's list. Sorting
on per-item keys means adding a project leaves everyone else's order alone.

### A6 — Credential stuffing and login abuse

*Defence.* 10 attempts per minute per client address, the strictest budget in the
system and the only one keyed on the address rather than the account, because
login is the one endpoint where an unauthenticated caller spends unlimited
attempts against a credential. Responses are uniform — one message, one status —
so a wrong email and a wrong password are indistinguishable. Passwords are bcrypt
at default cost, minimum eight characters. Sessions are opaque 256-bit tokens;
only their SHA-256 hash is stored.

*Residual.* Per-address limiting is defeated by a botnet. Account lockout is
**not** implemented, deliberately: an attacker who knows a judge's email can lock
that judge out of an event they are meant to judge, which is a worse outcome than
the brute force it prevents. This is a considered trade, not an oversight.

### A7 — Organizer over-reach (cross-event access)

*Threat.* An organizer of event A reads or rewrites event B's reviews, results or
settings by supplying B's slug or id.

*Defence.* Partial, and this is the most significant gap in the system.
`domain.EventRole` defines a per-event permission matrix with an owner role that
carries `ViewPeerScores` — **and no route consults it.** It is rendered into
`GET /v1/permissions` and used for display only. The enforced model is the five
global roles, so any user holding the global `organizer` role can act on any
event.

*Why it is like this.* For the target deployment — one organisation self-hosting
its own events — the organizer role *is* trusted staff, and a per-event model
would add a second authorization system without adding a trust boundary that
does not already exist.

*Why it is still a gap.* Any deployment that hosts events for mutually distrusting
organisations inherits a cross-tenant read. The fix is to enforce `EventRole.Can`
on the `/v1/organizer/...` routes, which is a bounded change: the matrix exists,
the roles exist, the check does not. Stated here rather than left to be
discovered.

### A8 — PII disclosure

*Threat.* Judge and organizer email addresses harvested from a public endpoint.

*Found and fixed during the build.* Three endpoints disclosed addresses to
anonymous callers: the judge roster, the event staff list, and the webhook list.

*Defence now.* The roster and staff pages stay public — who is judging an event is
public information — but **addresses are added only for staff**, and the
organizer's global reviewer pool is withheld from non-staff entirely. The webhook
list moved behind `ManageIntegrations`, because destination URLs and delivery
counts describe the organizer's infrastructure.

### A9 — Account enumeration

*Defence.* Login returns one message for both unknown-email and wrong-password.
`GET /v1/auth/methods` advertises the available methods deliberately, because a
self-hosted portal with one method should say so. Registration is not open, so
there is no endpoint that reveals whether an address has an account.

### A10 — Cross-tenant writes (event/record mismatch)

*Threat.* A review or comment is filed with an event id that does not match the
project's event, so it vanishes from the leaderboard it should count towards and
appears in the results of an unrelated event.

*Found and fixed during the build.* The store checked that the event and the
project both existed, but not that the project belonged to that event.

*Defence now.* `SaveReview` and `CreateComment` both reject a mismatch, and the
review handler ignores the client-supplied `event_id` entirely, deriving it from
the loaded project. The response to a mismatched body is 422.

### A11 — Result tampering via the session store

*Threat.* A stolen session cookie is replayed after the organizer has revoked it.

*Defence.* `logout` now revokes the session server-side, not just the cookie.
This was a real bug: the login response also returns the token as a bearer
credential, so a client that stored it stayed authenticated until the session
expired on its own. Sessions are not persisted, so **a portal restart invalidates
every session**, which means a stolen cookie does not survive a redeploy.

### A12 — Denial of service through expensive endpoints

*Threat.* Someone calls the results endpoint in a loop. Normalization runs 2000
bootstrap resamples over 126 reviews.

*Defence.* Rate limits on writes and ballots, and a bounded resample count.
Results are organizer-only. The read budget is deliberately generous (600/min) and
there is no blanket limit on the public gallery: a tight read cap on a public
gallery is a self-inflicted denial of service with no security benefit.

### A13 — CSV and export injection

*Defence.* The CSV writer quotes and escapes fields; the export is organizer-only;
`Content-Type` is set explicitly rather than sniffed.

---

## 2. What is not defended, and why

| Gap | Reasoning |
|---|---|
| Hostile organizer adding friendly judges | No technical control exists. The platform's contribution is auditability, not prevention. |
| Judge-to-judge collusion out of band | Physically unpreventable in an in-person panel. The platform removes the *information* channel (A1); it cannot remove the social one. |
| A judge who does not read the projects | Mitigation exists (review-time tracking, progress dashboard) but is advisory. |
| Account lockout | Deliberately omitted: it is a denial-of-service weapon aimed at judges. |
| Rate-limit evasion by a botnet | Accepted. Per-account limits handle the realistic case. |
| Transport security | The portal speaks plain HTTP. TLS is expected to terminate in front of it, and is not configured here. |
| Multi-tenant isolation | See A7. Correct for the stated deployment, a gap for any other. |

---

## 3. Two design choices that run through all of it

**Authorization lives in the backend, in one place.** `Server.requirePermission`
is the only thing that authenticates a request, and it re-reads the role from the
store on every request rather than trusting the token — so a role change takes
effect immediately and a stale token cannot escalate. Object-level checks
(assignment, team membership, comment authorship) live in the handlers, because
they need the object. The requirement is that a check must exist where `curl`
arrives; a check in a template is not a check.

**Rate limits are keyed on the account, not the address, wherever the caller is
authenticated.** This is not a detail. A hackathon venue, a university, or a
corporate network puts an entire panel behind one egress address, and a
per-address budget there would let the busiest attendee lock every other attendee
out of commenting. Anonymous endpoints — login being the important one — key on
the address.

---

## 4. Reporting

No security contact is configured, because this is a 72-hour hackathon project
with no operator behind it after the event. If you are reading this because you
found something: open an issue, and if it is a credential problem, treat the
seeded tokens as public by design. They are documented in `.dogfood.toml`,
printed on every boot, and the configuration refuses to enable seeding in
production without a private password.
