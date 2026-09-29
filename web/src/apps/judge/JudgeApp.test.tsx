import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { JudgeApp } from "./JudgeApp";
import { loadSession } from "../../lib/session";

/**
 * The scoring surface, against the wire shapes the server actually sends.
 *
 * These are the failures that produce a control which looks right and cannot be
 * used: a rubric criterion whose bounds are read from fields the payload does not
 * have renders an empty select, and a save that never asserts the ETag renders a
 * perfectly working page that silently loses a judge's second tab. Neither shows
 * up as an exception, so neither is caught by anything but a test.
 */

// min_score/max_score, not min/max: the Go struct tags, verbatim.
const RUBRIC = {
  id: "rub_1",
  version: 3,
  criteria: [
    {
      key: "functionality",
      label: "Functionality",
      weight: 50,
      min_score: 1,
      max_score: 5,
      required: true,
      description: "Does it do the thing it claims to do",
    },
    {
      key: "craft",
      label: "Craft",
      weight: 50,
      min_score: 0,
      max_score: 3,
      required: false,
      description: "Is it built well",
    },
  ],
};

const EVENT = {
  id: "evt_1",
  slug: "demo",
  name: "Demo Jam",
  state: "judging",
  submissions_open: false,
  submissions_close: "2026-02-01T00:00:00Z",
};

/** The assignment view the backend sends, which carries review_submitted. */
const assignment = {
  id: "asn_1",
  event_id: "evt_1",
  event_slug: "demo",
  judge_id: "usr_judge",
  project_id: "prj_1",
  project_title: "Nimbus",
  track_id: "trk_1",
  review_submitted: false,
};

function json(body: unknown, etag?: string): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: {
      "Content-Type": "application/json",
      ...(etag ? { ETag: etag } : {}),
    },
  });
}

describe("scoring a project against the rubric the server sent", () => {
  let reviewEtag: string;
  let savedBodies: Record<string, unknown>[];
  let assignments: typeof assignment[];
  let assignmentsFail = false;

  const fetchMock = vi.fn();

  // A named handler rather than an inline one, so a test that swaps the
  // implementation can hand the shared behaviour back to the next test.
  async function handle(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
    const method = init.method ?? "GET";
    const path = new URL(String(input), "http://portal.test").pathname;

    if (path === "/v1/me") {
      return json({
        data: { id: "usr_judge", email: "judge@example.test", display_name: "Jo", role: "judge" },
      });
    }
    if (path === "/v1/events") return json({ data: [EVENT] });
    if (path === "/v1/judge/assignments") {
      if (assignmentsFail) {
        return new Response(JSON.stringify({ error: { code: "forbidden", message: "refused" } }), {
          status: 403,
          headers: { "Content-Type": "application/json" },
        });
      }
      return json({ data: assignments, count: assignments.length, pending: assignments.length });
    }
    if (path === "/v1/judging/rubric") return json({ data: RUBRIC });
    if (path === "/v1/judge/projects/prj_1/review") {
      if (method === "PUT") {
        const body = JSON.parse(String(init.body)) as Record<string, unknown>;
        savedBodies.push(body);
        // A write moves the version on, which is what the next If-Match must see.
        reviewEtag = `"saved-${savedBodies.length}"`;
        return json({ data: { ...body, id: "rev_1", updated_at: "2026-02-02T10:00:00Z" } }, reviewEtag);
      }
      return json(
        {
          data: {
            id: "rev_1",
            project_id: "prj_1",
            judge_id: "usr_judge",
            criteria: {},
            comment: "",
            submitted: false,
            updated_at: "2026-02-01T10:00:00Z",
          },
        },
        reviewEtag,
      );
    }
    return new Response("not found", { status: 404 });
  }

  function renderScoring() {
    return render(
      <MemoryRouter initialEntries={["/judge/prj_1"]}>
        <Routes>
          <Route path="/judge/*" element={<JudgeApp />} />
        </Routes>
      </MemoryRouter>,
    );
  }

  /** The most recent PUT, which is the one whose If-Match matters. */
  function saveOf() {
    const calls = fetchMock.mock.calls.filter(
      ([input, init]) =>
        String(input).includes("/v1/judge/projects/prj_1/review") && init?.method === "PUT",
    );
    return calls.at(-1)?.[1] as (RequestInit & { headers: Record<string, string> }) | undefined;
  }

  beforeEach(async () => {
    savedBodies = [];
    assignments = [assignment];
    assignmentsFail = false;
    reviewEtag = '"read-1"';
    fetchMock.mockReset();
    fetchMock.mockImplementation(handle);
    vi.stubGlobal("fetch", fetchMock);
    await loadSession();
  });

  it("offers one option per point between min_score and max_score", async () => {
    renderScoring();

    // The regression this guards: with the bounds read as min/max, both are
    // undefined, Array.from({length: NaN}) yields nothing, and every select is
    // empty while still looking like a control.
    const functionality = await screen.findByLabelText("Functionality score");
    const options = within(functionality)
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(options).toEqual(["—", "1", "2", "3", "4", "5"]);

    // A criterion whose scale does not start at zero is the other half of it.
    const craft = screen.getByLabelText("Craft score");
    expect(
      within(craft)
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["—", "0", "1", "2", "3"]);
  });

  it("saves the scores under the criterion key the rubric declares", async () => {
    const user = userEvent.setup();
    renderScoring();

    await user.selectOptions(await screen.findByLabelText("Functionality score"), "4");
    await user.selectOptions(screen.getByLabelText("Craft score"), "2");
    await user.click(screen.getByRole("button", { name: /save draft/i }));
    await screen.findByText("Draft saved");

    expect(savedBodies).toHaveLength(1);
    expect(savedBodies[0]?.criteria).toEqual({ functionality: 4, craft: 2 });
    expect(savedBodies[0]?.event_id).toBe("evt_1");
  });

  it("asserts the ETag the review read returned on the save", async () => {
    const user = userEvent.setup();
    renderScoring();

    await user.selectOptions(await screen.findByLabelText("Functionality score"), "3");
    await user.click(screen.getByRole("button", { name: /save draft/i }));
    await screen.findByText("Draft saved");

    // Without If-Match the server's 412 guard is unreachable and two tabs
    // scoring the same project overwrite each other with no error anywhere.
    expect(saveOf()?.headers?.["If-Match"]).toBe('"read-1"');
  });

  it("asserts the version its own save produced on the next save", async () => {
    const user = userEvent.setup();
    renderScoring();

    await user.selectOptions(await screen.findByLabelText("Functionality score"), "3");
    await user.click(screen.getByRole("button", { name: /save draft/i }));
    await screen.findByText("Draft saved");
    await user.click(screen.getByRole("button", { name: /save draft/i }));
    await waitFor(() => expect(savedBodies).toHaveLength(2));

    // A client that saved once and then asserted the version it replaced would
    // be refused by its own last write.
    expect(saveOf()?.headers?.["If-Match"]).toBe('"saved-1"');
  });

  it("explains a rejected precondition rather than repeating the save", async () => {
    const user = userEvent.setup();
    let firstSave = true;
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init: RequestInit = {}) => {
      const method = init.method ?? "GET";
      const path = new URL(String(input), "http://portal.test").pathname;
      if (path === "/v1/judge/projects/prj_1/review" && method === "PUT") {
        if (firstSave) {
          firstSave = false;
          return new Response(
            JSON.stringify({
              error: { code: "version_conflict", message: "this resource has changed since it was read" },
            }),
            { status: 412, headers: { "Content-Type": "application/json" } },
          );
        }
        savedBodies.push(JSON.parse(String(init.body)) as Record<string, unknown>);
        return json({ data: {} }, '"rev-2"');
      }
      return handle(input, init);
    });

    renderScoring();
    await user.selectOptions(await screen.findByLabelText("Functionality score"), "3");
    await user.click(screen.getByRole("button", { name: /save draft/i }));

    expect(await screen.findByText("This review changed while you were editing")).toBeInTheDocument();
    expect(savedBodies).toHaveLength(0);
  });

  it("asks before locking a submitted review", async () => {
    const user = userEvent.setup();
    renderScoring();

    await user.selectOptions(await screen.findByLabelText("Functionality score"), "5");
    await user.click(screen.getByRole("button", { name: /submit and lock this review/i }));

    // Irreversible, so it cannot happen on the click that expresses the intent.
    const dialog = await screen.findByRole("dialog", { name: "Submit and lock this review?" });
    expect(dialog).toBeInTheDocument();
    expect(savedBodies).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: /keep editing/i }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(savedBodies).toHaveLength(0);
  });

  it("keeps the scores on the page when a save is refused", async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init: RequestInit = {}) => {
      if (
        new URL(String(input), "http://portal.test").pathname === "/v1/judge/projects/prj_1/review" &&
        init.method === "PUT"
      ) {
        return new Response(
          JSON.stringify({ error: { code: "review_locked", message: "a submitted review cannot be changed" } }),
          { status: 409, headers: { "Content-Type": "application/json" } },
        );
      }
      return handle(input, init);
    });

    renderScoring();
    const score = await screen.findByLabelText("Functionality score");
    await user.selectOptions(score, "4");
    await user.click(screen.getByRole("button", { name: /save draft/i }));

    expect(await screen.findByText("That was not saved")).toBeInTheDocument();
    // design.md 12: the input survives a failed write. Replacing the page with an
    // error would discard the scores the judge just entered.
    expect(screen.getByLabelText("Functionality score")).toHaveValue("4");
  });

  it("says a judge with no assignment has nothing to score, rather than loading forever", async () => {
    assignments = [];
    const user = userEvent.setup();
    renderScoring();

    expect(
      await screen.findByText("This project is not assigned to you"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Functionality score")).toBeNull();
    await user.click(screen.getByRole("link", { name: /my assignments/i }));
  });

  it("shows a failed load as an error with a retry, not a permanent skeleton", async () => {
    assignmentsFail = true;
    renderScoring();

    expect(await screen.findByText("This review could not be loaded")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /try again/i }).length).toBeGreaterThan(0);
    // The skeleton guard used to sit in front of the error, so a failed load
    // looked like a load that never finishes.
    expect(document.querySelectorAll(".skeleton")).toHaveLength(0);
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes("/judging/rubric"))).toBe(false);
  });
});

/**
 * The compare surface, which rendered recorded verdicts and had no way to record
 * one. Its own empty state told the judge to record comparisons and offered
 * nothing to do it with.
 */
describe("recording a head-to-head comparison", () => {
  const posted: Record<string, unknown>[] = [];

  const secondAssignment = {
    ...assignment,
    id: "asn_2",
    project_id: "prj_2",
    project_title: "Cirrus",
  };

  async function handle(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
    const method = init.method ?? "GET";
    const path = new URL(String(input), "http://portal.test").pathname;

    if (path === "/v1/me") {
      return json({
        data: { id: "usr_judge", email: "judge@example.test", display_name: "Jo", role: "judge" },
      });
    }
    if (path === "/v1/events") return json({ data: [EVENT] });
    if (path === "/v1/judge/assignments") {
      return json({ data: [assignment, secondAssignment], count: 2, pending: 2 });
    }
    if (path === "/v1/events/demo/comparisons") {
      if (method === "POST") {
        const body = JSON.parse(String(init.body)) as Record<string, unknown>;
        posted.push(body);
        return json(
          {
            data: {
              id: "cmp_1",
              event_id: "evt_1",
              judge_id: "usr_judge",
              ...body,
              created_at: "2026-02-03T09:00:00Z",
              updated_at: "2026-02-03T09:00:00Z",
            },
          },
          '"cmp-1"',
        );
      }
      return json({ data: [], count: 0 });
    }
    return new Response("not found", { status: 404 });
  }

  beforeEach(async () => {
    posted.length = 0;
    vi.stubGlobal("fetch", vi.fn(handle));
    await loadSession();
  });

  function renderCompare() {
    return render(
      <MemoryRouter initialEntries={["/judge/compare"]}>
        <Routes>
          <Route path="/judge/*" element={<JudgeApp />} />
        </Routes>
      </MemoryRouter>,
    );
  }

  it("sends the pair, the verdict and the reason the judge gave", async () => {
    const user = userEvent.setup();
    renderCompare();

    await user.selectOptions(await screen.findByLabelText("First project"), "prj_1");
    await user.selectOptions(screen.getByLabelText("Second project"), "prj_2");
    await user.selectOptions(screen.getByLabelText("Which one"), "left");
    await user.type(screen.getByLabelText("Why"), "It answers the brief.");
    await user.click(screen.getByRole("button", { name: /record comparison/i }));

    await screen.findByText("Comparison recorded");
    expect(posted).toEqual([
      { left: "prj_1", right: "prj_2", verdict: "left", comment: "It answers the brief." },
    ]);
    // The recorded verdict is the surface's own point, so it is in the list and
    // the empty state that asked for one is gone.
    const table = await screen.findByRole("table");
    expect(within(table).getAllByText("Nimbus").length).toBeGreaterThan(0);
    expect(within(table).getByText("Cirrus")).toBeInTheDocument();
    expect(screen.queryByText("No comparisons recorded yet")).toBeNull();
  });

  it("will not let a judge compare a project with itself", async () => {
    const user = userEvent.setup();
    renderCompare();

    await user.selectOptions(await screen.findByLabelText("First project"), "prj_1");
    await user.selectOptions(screen.getByLabelText("Second project"), "prj_1");
    await user.selectOptions(screen.getByLabelText("Which one"), "tie");
    await user.click(screen.getByRole("button", { name: /record comparison/i }));

    expect(await screen.findByText(/cannot be compared with itself/)).toBeInTheDocument();
    expect(posted).toHaveLength(0);
  });
});
