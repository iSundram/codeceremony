"""Screenshot the portal so the design can be looked at rather than described.

Reads pages with a real session cookie so the authenticated surfaces render, and
reports any console error and any failed request per page, because a screenshot
of a page that threw is a screenshot of an error boundary.
"""

import sys
import pathlib
from playwright.sync_api import sync_playwright

BASE = "http://localhost:3000"
OUT = pathlib.Path("/tmp/shots")
OUT.mkdir(parents=True, exist_ok=True)

# Fixed seeded session tokens, printed on every boot.
TOKENS = {
    "admin": "cc_adm_3c5e7a9b1d0f2e46",
    "judge_a": "cc_jdg_a_91bc4d2e7a35f08",
    "organizer": "cc_org_7f2a1b9d4e6c8a0f",
    "participant": "cc_prt_2e88a4d6c0b91f37",
}

PAGES = [
    ("01-login", "/login", None),
    ("02-events", "/events", None),
    ("03-event-detail", "/events/sample-hack-2026", None),
    ("04-login-organizer", "/login", "organizer"),
    ("05-dashboard", "/dashboard", "organizer"),
    ("06-organizer", "/organizer", "organizer"),
    ("07-organizer-results", "/organizer/results", "organizer"),
    ("08-organizer-audit", "/organizer/audit", "organizer"),
    ("09-judge", "/judge", "judge_a"),
    ("10-judge-score", "/judge/compare", "judge_a"),
    ("11-account", "/account", "participant"),
    ("12-admin", "/admin/accounts", "admin"),
    ("13-events-gallery", "/events/gallery", None),
]

WIDTH = int(sys.argv[1]) if len(sys.argv) > 1 else 1440
HEIGHT = int(sys.argv[2]) if len(sys.argv) > 2 else 1000
SUFFIX = sys.argv[3] if len(sys.argv) > 3 else ""

with sync_playwright() as p:
    browser = p.chromium.launch()
    for name, path, who in PAGES:
        context = browser.new_context(viewport={"width": WIDTH, "height": HEIGHT})
        if who:
            context.add_cookies(
                [{
                    "name": "session",
                    "value": TOKENS[who],
                    "domain": "localhost",
                    "path": "/",
                }]
            )
        page = context.new_page()
        errors: list[str] = []
        failed: list[str] = []
        page.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
        page.on("requestfailed", lambda r: failed.append(f"{r.url} {r.failure}"))
        try:
            page.goto(f"{BASE}{path}", wait_until="networkidle", timeout=20000)
            page.wait_for_timeout(700)
            shot = OUT / f"{name}{SUFFIX}.png"
            page.screenshot(path=str(shot), full_page=False)
            heading = page.evaluate("document.querySelector('h1,h2')?.textContent?.trim() ?? ''")
            body_len = page.evaluate("document.body.innerText.trim().length")
            flag = ""
            if errors:
                flag += f"  [console x{len(errors)}: {errors[0][:70]}]"
            if failed:
                flag += f"  [failed x{len(failed)}: {failed[0][:60]}]"
            if body_len < 40:
                flag += "  [NEARLY EMPTY PAGE]"
            print(f"  {shot.name:28} {heading[:34]:36} text={body_len:5}{flag}")
        except Exception as exc:  # noqa: BLE001
            print(f"  {name:28} FAILED: {str(exc)[:100]}")
        finally:
            context.close()
    browser.close()
print("screenshots in", OUT)
