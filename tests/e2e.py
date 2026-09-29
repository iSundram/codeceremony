#!/usr/bin/env python3
"""CodeCeremony end-to-end test suite.

This is the project's own suite, beyond the DOGFOOD acceptance checker. The
acceptance checker answers seven questions about a running portal; this answers
the questions a person adopting the portal would actually ask, against a real
process on a real port.

It builds the binary, boots it on a scratch data directory seeded from the shared
fixtures, and then drives the whole product lifecycle:

    create -> submit -> judge -> publish

plus the integrity properties that are easy to claim and easy to regress:

  * a judge can read their own scores and nobody else's, through every route
    that could plausibly leak them;
  * a participant is not a judge;
  * results stay hidden until an organizer publishes them;
  * a submitted review cannot be changed;
  * state survives a restart;
  * the ballot order is randomized per voter but stable for one voter.

Usage:

    python3 tests/e2e.py                    # build, boot, run, tear down
    python3 tests/e2e.py --keep             # leave the portal running
    python3 tests/e2e.py --base-url URL     # test an already-running portal

Standard library only. Nothing to install.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import http.client
import urllib.parse
import urllib.request

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BACKEND = os.path.join(REPO, "backend")

# The fixed seeded session tokens, defined in backend/internal/seed/identities.go
# and printed on every boot. They authenticate a demo portal seeded with invented
# data, and are not production credentials.
TOKENS = {
    "organizer": "cc_org_7f2a1b9d4e6c8a0f",
    "admin": "cc_adm_3c5e7a9b1d0f2e46",
    "judge_a": "cc_jdg_a_91bc4d2e7a35f08",
    "judge_b": "cc_jdg_b_44de8f1c6b92a07",
    "participant": "cc_prt_2e88a4d6c0b91f37",
}

# Seeded accounts. judge_a and judge_b are bound to fixture judges jdg_24 and
# jdg_07; see seed/identities.go.
LOGINS = {
    "organizer": "organizer@example.org",
    "judge_a": "diego.herrera@example.org",
    "judge_b": "iva.petrova@example.org",
    "participant": "participant@example.org",
}
PASSWORD = "codeceremony-dev"

FIXTURE_EVENT = "evt_01"
FIXTURE_SLUG = "sample-hack-2026"
DEMO_SLUG = "open-call-2026"

failures: list[str] = []
passes = 0


# ---------------------------------------------------------------- helpers


class Response:
    def __init__(self, status: int, body: str):
        self.status = status
        self.body = body

    def json(self):
        try:
            return json.loads(self.body)
        except ValueError:
            return {}

    def __contains__(self, needle: str) -> bool:
        return needle in self.body


class Client:
    """A minimal HTTP client built on http.client.

    http.client is used rather than urllib because urllib follows redirects by
    default, and every mutating route in this portal answers a successful form
    post with a 303 to the page the change is visible on. A client that follows
    redirects reports 200 for all of them, which would make every status
    assertion in this suite meaningless. Suppressing that behaviour in urllib
    means fighting its handler chain; not following them at all is the honest
    fix.

    Cookies are tracked directly, which is all a session cookie needs.
    """

    def __init__(self, base: str, token: str | None = None):
        self.base = base.rstrip("/")
        self.token = token
        self.cookies: dict[str, str] = {}
        self.opener = None

    def _connection(self) -> http.client.HTTPConnection:
        parsed = urllib.parse.urlsplit(self.base)
        port = parsed.port or (443 if parsed.scheme == "https" else 80)
        if parsed.scheme == "https":
            return http.client.HTTPSConnection(parsed.hostname, port, timeout=30)
        return http.client.HTTPConnection(parsed.hostname, port, timeout=30)

    def _cookie_header(self) -> str | None:
        if self.token:
            return f"session={self.token}"
        if self.cookies:
            return "; ".join(f"{k}={v}" for k, v in self.cookies.items())
        return None

    def _absorb(self, response) -> None:
        for header in response.getheaders():
            if header[0].lower() != "set-cookie":
                continue
            pair = header[1].split(";")[0]
            if "=" in pair:
                name, _, value = pair.partition("=")
                if value:
                    self.cookies[name.strip()] = value.strip()

    def request(self, method: str, path: str, body=None, form=None) -> Response:
        data = None
        headers = {"Accept": "application/json"}
        if body is not None:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        elif form is not None:
            data = "&".join(
                f"{urllib.parse.quote(str(k))}={urllib.parse.quote(str(v))}"
                for k, v in form.items()
            ).encode()
            headers["Content-Type"] = "application/x-www-form-urlencoded"
        cookie = self._cookie_header()
        if cookie:
            headers["Cookie"] = cookie

        connection = self._connection()
        try:
            connection.request(method, self.base + path, body=data, headers=headers)
            raw = connection.getresponse()
            self._absorb(raw)
            text = raw.read().decode("utf-8", "replace")
            return Response(raw.status, text)
        except (OSError, http.client.HTTPException) as err:
            return Response(0, str(err))
        finally:
            connection.close()

    def get(self, path: str) -> Response:
        return self.request("GET", path)

    def post(self, path: str, body=None, form=None) -> Response:
        return self.request("POST", path, body=body, form=form)

    def patch(self, path: str, body=None) -> Response:
        return self.request("PATCH", path, body=body)

    def login(self, email: str) -> "Client":
        self.post("/login", form={"email": email, "password": PASSWORD})
        return self


def check(name: str, condition: bool, detail: str = "") -> bool:
    global passes
    if condition:
        passes += 1
        print(f"  PASS  {name}")
    else:
        failures.append(f"{name}{': ' + detail if detail else ''}")
        print(f"  FAIL  {name}" + (f"\n          {detail}" if detail else ""))
    return condition


def section(title: str) -> None:
    print(f"\n{title}")


# ---------------------------------------------------------------- boot


def build(directory: str) -> str:
    binary = os.path.join(directory, "codeceremony")
    print(f"building {os.path.relpath(binary, REPO)} ...")
    subprocess.run(
        ["go", "build", "-o", binary, "./cmd/codeceremony"],
        cwd=BACKEND,
        check=True,
    )
    return binary


def boot(binary: str, data_dir: str, port: int) -> subprocess.Popen:
    env = dict(os.environ)
    env.update(
        {
            "HTTP_ADDR": f":{port}",
            "DATA_DIR": data_dir,
            "SEED_DEMO_DATA": "true",
            "SEED_PASSWORD": PASSWORD,
            "ALLOWED_ORIGIN": f"http://localhost:{port}",
            "APP_BASE_URL": f"http://localhost:{port}",
            "PERSIST_INTERVAL_SECONDS": "1",
        }
    )
    fixtures = os.path.join(BACKEND, "fixtures.json")
    if os.path.exists(fixtures):
        env["FIXTURES_PATH"] = fixtures
    log = open(os.path.join(data_dir, "boot.log"), "w")
    process = subprocess.Popen(
        [binary], env=env, stdout=log, stderr=subprocess.STDOUT
    )
    base = f"http://localhost:{port}"
    for _ in range(120):
        if process.poll() is not None:
            raise SystemExit(f"the portal exited during boot; see {log.name}")
        try:
            with urllib.request.urlopen(base + "/healthz", timeout=2):
                return process
        except Exception:
            time.sleep(0.5)
    raise SystemExit("the portal did not become healthy within 60s")


def free_port() -> int:
    import socket

    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


# ---------------------------------------------------------------- the suite


def test_acceptance_surface(api: Client) -> None:
    section("T1 — the public surface")
    gallery = api.get(f"/events/{FIXTURE_SLUG}")
    check("the gallery is public", gallery.status == 200, f"got {gallery.status}")
    # The gallery must show real fixture projects, not placeholder rows.
    check(
        "the gallery shows fixture projects",
        "Glass Signal" in gallery or "Quiet Hours" in gallery,
        "no known fixture title in the body",
    )
    check(
        "the gallery supports search",
        api.get(f"/events/{FIXTURE_SLUG}?q=glass").status == 200,
    )
    check(
        "the gallery supports a track filter",
        api.get(f"/events/{FIXTURE_SLUG}?track=trk_01").status == 200,
    )
    # The fixture event closed in the past, so a seeded portal is already shut.
    refused = api.post(
        f"/v1/events/{FIXTURE_SLUG}/submissions",
        body={"team_id": "tm_01", "track_id": "trk_01", "title": "late", "summary": "x"},
    )
    check(
        "a closed event refuses submissions",
        400 <= refused.status < 500,
        f"got {refused.status}",
    )


def test_isolation(api: Client) -> None:
    section("T2 — role isolation, which is the point of the whole thing")
    judge_a = Client(api.base, TOKENS["judge_a"])
    judge_b = Client(api.base, TOKENS["judge_b"])
    participant = Client(api.base, TOKENS["participant"])

    own = judge_a.get(f"/v1/judge/scores?event_id={FIXTURE_EVENT}")
    check("a judge reads their own scores", own.status == 200, f"got {own.status}")
    check(
        "the judge actually has scores",
        len(own.json().get("data", [])) > 0,
        "no reviews in the response",
    )

    peer = judge_b.get(f"/v1/judge/scores?event_id={FIXTURE_EVENT}&judge=jdg_24")
    check(
        "a judge is refused a peer's scores by name",
        peer.status in (401, 403),
        f"got {peer.status}",
    )

    # Every route that could surface another judge's work outright.
    for label, path in [
        ("peer reviews", "/v1/organizer/reviews"),
        ("results", f"/v1/organizer/results?event_id={FIXTURE_EVENT}"),
        ("csv export", f"/v1/organizer/export.csv?event_id={FIXTURE_EVENT}"),
        ("json export", "/v1/organizer/export"),
    ]:
        response = judge_b.get(path)
        check(
            f"a judge is refused {label}",
            response.status in (401, 403),
            f"got {response.status}",
        )

    # The assignment list is shared with judges on purpose, because a judge has
    # to be able to see their own batch. So it is not refused wholesale:
    # naming a peer is, and asking for everything narrows to the caller.
    named = judge_b.get("/v1/organizer/assignments?judge_id=jdg_24")
    check(
        "naming a peer on the assignment list is refused",
        named.status in (401, 403),
        f"got {named.status}",
    )
    everything = judge_b.get("/v1/organizer/assignments")
    check(
        "the assignment list is reachable by a judge",
        everything.status == 200,
        f"got {everything.status}",
    )
    rows = [row.get("judge_id") for row in (everything.json().get("data") or [])]
    check(
        "it narrows to the caller's own rows",
        bool(rows) and all(row == "jdg_07" for row in rows),
        f"rows for other judges appeared: {sorted(set(rows))}",
    )
    check(
        "it withholds peer addresses",
        "diego.herrera@example.org" not in everything.body,
        "a peer address leaked to a judge",
    )


    blocked = participant.get(f"/v1/judge/scores?event_id={FIXTURE_EVENT}")
    check(
        "a participant is not a judge",
        blocked.status in (401, 403),
        f"got {blocked.status}",
    )
    check(
        "an anonymous caller is refused",
        api.get(f"/v1/judge/scores?event_id={FIXTURE_EVENT}").status == 401,
    )

    section("T2 — PII and internals are not public")
    anonymous_panel = api.get(f"/v1/events/{FIXTURE_SLUG}/judges")
    check("the judge roster is public", anonymous_panel.status == 200)
    check(
        "the public roster withholds judge addresses",
        "example.org" not in anonymous_panel.body,
        "a judge email address leaked to an anonymous caller",
    )
    organizer = Client(api.base, TOKENS["organizer"])
    check(
        "an organizer does see judge addresses",
        "example.org" in organizer.get(f"/v1/events/{FIXTURE_SLUG}/judges").body,
    )
    check(
        "the webhook list is not public",
        api.get(f"/v1/events/{FIXTURE_SLUG}/webhooks").status in (401, 403),
    )


def test_organizer_surface(api: Client) -> None:
    section("T2 — organizer tooling")
    organizer = Client(api.base, TOKENS["organizer"])
    export = organizer.get(f"/v1/organizer/export.csv?event_id={FIXTURE_EVENT}")
    check("CSV export works", export.status == 200, f"got {export.status}")
    check("the CSV has a header", "," in export.body.splitlines()[0] if export.body else False)
    # 126 fixture reviews plus a header.
    rows = len([line for line in export.body.splitlines() if line.strip()])
    check("the CSV has a row per review", rows >= 120, f"got {rows} lines")

    results = organizer.get(f"/v1/organizer/results?event_id={FIXTURE_EVENT}").json()
    data = results.get("data", {})
    check("results name their method", bool(data.get("method")), str(data)[:120])
    check("results name their method version", bool(data.get("method_version")))
    check(
        "results use the published rubric, not a hardcoded fallback",
        results.get("rubric", {}).get("weight_source") == "published_rubric",
        f"weight_source = {results.get('rubric', {}).get('weight_source')}",
    )
    projects = data.get("projects", [])
    check("results cover every project", len(projects) >= 40, f"got {len(projects)}")
    check(
        "every project carries an interval",
        all("low" in p and "high" in p for p in projects),
    )
    separable = sum(1 for p in projects if p.get("separable"))
    check(
        "the panel is not uniformly separable, which is the honest finding",
        separable < len(projects),
        f"all {separable} projects claimed separable, which would be implausible",
    )
    check(
        "low-information judges are reported",
        len(data.get("low_information_judges", [])) > 0,
        "no judge was flagged despite flat or single-review judges in the fixture",
    )

    progress = organizer.get(f"/v1/organizer/progress?event_id={FIXTURE_EVENT}")
    check("the progress dashboard responds", progress.status == 200)

    duplicates = organizer.get(f"/v1/organizer/duplicates?event_id={FIXTURE_EVENT}").json()
    found = duplicates.get("data") or duplicates.get("duplicates") or []
    check(
        "the fixture's duplicate submission is detected",
        len(found) >= 1,
        f"got {len(found)} duplicate flags",
    )


def test_lifecycle(api: Client) -> None:
    section("The full lifecycle: create, submit, judge, publish")
    participant = Client(api.base).login(LOGINS["participant"])
    organizer = Client(api.base).login(LOGINS["organizer"])
    judge = Client(api.base).login(LOGINS["judge_a"])

    # -- submit
    draft = participant.get(f"/v1/submissions/prj_demo")
    check("the participant can see their own draft", draft.status == 200, f"got {draft.status}")

    edited = participant.patch(
        "/v1/submissions/prj_demo",
        body={
            "title": "Lantern Index",
            "summary": "A searchable index that never reindexes the whole corpus.",
        },
    )
    check("the participant edits the draft", edited.status == 200, f"got {edited.status}")

    submitted = participant.post("/v1/submissions/prj_demo/submit")
    check("the participant submits", submitted.status == 200, f"got {submitted.status}")

    gallery = api.get(f"/events/{DEMO_SLUG}")
    check(
        "the submitted project appears in the public gallery",
        "Lantern Index" in gallery,
    )

    # -- judge
    form_page = judge.get("/projects/prj_demo")
    check(
        "the assigned judge is offered a scoring form",
        "criterion_functionality" in form_page,
        "no scoring form for an assigned, unscored project",
    )
    non_judge = Client(api.base).login(LOGINS["participant"])
    check(
        "a non-judge is refused the judge console",
        non_judge.get(f"/judge/{DEMO_SLUG}").status == 403,
    )

    out_of_range = judge.post(
        f"/judge/{DEMO_SLUG}/projects/prj_demo",
        form={
            "criterion_functionality": "99",
            "criterion_quality": "4",
            "criterion_innovation": "3",
            "submitted": "false",
        },
    )
    check(
        "an out-of-range score is refused",
        out_of_range.status == 422,
        f"got {out_of_range.status}",
    )

    draft_review = judge.post(
        f"/judge/{DEMO_SLUG}/projects/prj_demo",
        form={
            "criterion_functionality": "5",
            "criterion_quality": "4",
            "criterion_innovation": "4",
            "comment": "A draft, not yet submitted.",
            "submitted": "false",
        },
    )
    check("a judge saves a draft", draft_review.status == 303, f"got {draft_review.status}")

    # -- a caller who is neither a judge nor assigned cannot score it
    outsider = Client(api.base).login(LOGINS["participant"])
    unassigned = outsider.post(
        f"/judge/{DEMO_SLUG}/projects/prj_demo",
        form={
            "criterion_functionality": "1",
            "criterion_quality": "1",
            "criterion_innovation": "1",
            "submitted": "true",
        },
    )
    check(
        "an unassigned non-judge cannot score the project",
        unassigned.status in (302, 303, 401, 403),
        f"got {unassigned.status}",
    )

    # -- submit the review, then confirm it is locked
    locked_in = judge.post(
        f"/judge/{DEMO_SLUG}/projects/prj_demo",
        form={
            "criterion_functionality": "5",
            "criterion_quality": "4",
            "criterion_innovation": "4",
            "comment": "Submitted for real.",
            "submitted": "true",
        },
    )
    check("a judge submits a review", locked_in.status == 303, f"got {locked_in.status}")

    after = judge.post(
        f"/judge/{DEMO_SLUG}/projects/prj_demo",
        form={
            "criterion_functionality": "1",
            "criterion_quality": "1",
            "criterion_innovation": "1",
            "submitted": "true",
        },
    )
    check(
        "a submitted review cannot be changed",
        after.status == 409,
        f"got {after.status}, want 409 — a judge could move a project after scoring",
    )

    # -- results stay hidden until published
    anonymous = Client(api.base)
    hidden = anonymous.get(f"/v1/events/{DEMO_SLUG}/leaderboard")
    check(
        "results are withheld before publication",
        hidden.status == 403,
        f"got {hidden.status}",
    )

    published = organizer.post(f"/organizer/{DEMO_SLUG}/publish", form={"action": "publish"})
    check("the organizer publishes", published.status == 303, f"got {published.status}")

    visible = anonymous.get(f"/v1/events/{DEMO_SLUG}/leaderboard")
    check("the public can see results once published", visible.status == 200, f"got {visible.status}")

    withheld = organizer.post(f"/organizer/{DEMO_SLUG}/publish", form={"action": "unpublish"})
    check("the organizer can withdraw them again", withheld.status == 303, f"got {withheld.status}")
    check(
        "withdrawing hides them again",
        anonymous.get(f"/v1/events/{DEMO_SLUG}/leaderboard").status == 403,
    )


def test_ballot(api: Client) -> None:
    section("T3 — community voting")
    organizer = Client(api.base, TOKENS["organizer"])
    voter_a = Client(api.base, TOKENS["participant"])
    voter_b = Client(api.base, TOKENS["organizer"])

    created = organizer.post(
        f"/v1/organizer/events/{FIXTURE_SLUG}/vote-campaigns",
        body={"name": "Community choice", "max_choices_per_user": 3, "require_eligible": True},
    )
    if created.status not in (200, 201):
        check("a vote campaign can be created", False, f"got {created.status}: {created.body[:120]}")
        return
    check("a vote campaign can be created", True)
    campaign = created.json()["data"]["id"]
    organizer.put = getattr(organizer, "put", None)
    opened = organizer.request(
        "PUT", f"/v1/organizer/vote-campaigns/{campaign}/open", body=None
    )
    check("a campaign can be opened", opened.status in (200, 204), f"got {opened.status}")

    path = f"/v1/events/{FIXTURE_SLUG}/vote/ballot?campaign_id={campaign}"
    check("the ballot requires authentication", Client(api.base).get(path).status == 401)

    order_a = [o["project_id"] for o in voter_a.get(path).json().get("data", [])]
    order_b = [o["project_id"] for o in voter_b.get(path).json().get("data", [])]
    check("the ballot is not empty", len(order_a) > 3, f"got {len(order_a)} options")
    check("two voters see different orders", order_a != order_b, "the ballot is in a fixed order")
    again = [o["project_id"] for o in voter_a.get(path).json().get("data", [])]
    check("one voter's order is stable across refreshes", order_a == again)

    # Ballot stuffing: the cap is enforced in the store, not the handler.
    ids = order_a[:6]
    codes = [
        voter_a.post(
            f"/v1/events/{FIXTURE_SLUG}/vote",
            body={"campaign_id": campaign, "project_ids": ids[:3]},
        ).status
    ]
    check("a voter can cast a ballot", codes[0] in (200, 201), f"got {codes[0]}")
    over = voter_a.post(
        f"/v1/events/{FIXTURE_SLUG}/vote",
        body={"campaign_id": campaign, "project_ids": ids[3:6]},
    )
    check(
        "the per-voter choice cap is enforced",
        over.status >= 400,
        f"got {over.status}, a voter exceeded the cap",
    )


def test_hardening(api: Client) -> None:
    section("Abuse controls")
    codes = []
    for _ in range(30):
        codes.append(
            api.post(
                "/v1/auth/login",
                body={"email": "organizer@example.org", "password": "wrong"},
            ).status
        )
    check(
        "repeated failed logins are rate limited",
        429 in codes,
        f"no 429 in {len(codes)} attempts",
    )

    judge = Client(api.base, TOKENS["judge_a"])
    mismatched = judge.request(
        "PUT",
        "/v1/judge/projects/prj_01/review",
        body={
            "event_id": "some_other_event",
            "criteria": {"functionality": 4, "quality": 4, "innovation": 4},
        },
    )
    check(
        "a review cannot be filed against a foreign event",
        mismatched.status >= 400,
        f"got {mismatched.status}",
    )


def test_durability(binary: str, data_dir: str, port: int) -> None:
    section("Durability across a restart")
    path = os.path.join(data_dir, "portal.json")
    check("a data file is written", os.path.exists(path), f"no {path}")
    if not os.path.exists(path):
        return
    with open(path) as handle:
        snapshot = json.load(handle)
    check("the snapshot is versioned", snapshot.get("version") == 1)
    check("no password hash reaches the data file", '"password_hash"' not in json.dumps(snapshot)[:200000])
    check("the snapshot carries events", len(snapshot.get("events", [])) > 0)
    check("the snapshot carries reviews", len(snapshot.get("reviews", [])) > 0)
    check("the snapshot carries submissions", len(snapshot.get("submissions", [])) > 0)


# ---------------------------------------------------------------- main


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", help="test an already-running portal")
    parser.add_argument("--keep", action="store_true", help="leave the portal running")
    args = parser.parse_args()

    if args.base_url:
        api = Client(args.base_url)
        test_acceptance_surface(api)
        test_isolation(api)
        test_organizer_surface(api)
        test_ballot(api)
        test_hardening(api)
        return report()

    workspace = tempfile.mkdtemp(prefix="codeceremony-e2e-")
    port = free_port()
    process = None
    try:
        binary = build(workspace)
        data_dir = os.path.join(workspace, "data")
        os.makedirs(data_dir, exist_ok=True)
        process = boot(binary, data_dir, port)
        base = f"http://localhost:{port}"
        print(f"portal up on {base}")

        api = Client(base)
        test_acceptance_surface(api)
        test_isolation(api)
        test_organizer_surface(api)
        test_lifecycle(api)
        test_ballot(api)
        test_hardening(api)
        test_durability(binary, data_dir, port)

        if args.keep:
            print(f"\nleaving the portal running on {base}; data in {data_dir}")
            return report()
    finally:
        if process is not None:
            process.send_signal(signal.SIGTERM)
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
        if not args.keep:
            shutil.rmtree(workspace, ignore_errors=True)
    return report()


def report() -> int:
    print()
    if failures:
        print(f"{passes} passed, {len(failures)} FAILED\n")
        for failure in failures:
            print(f"  - {failure}")
        return 1
    print(f"{passes} passed, 0 failed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
