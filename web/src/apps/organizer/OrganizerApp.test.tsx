import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, ApiError, type EventSummary, type Grant, type Project } from "../../lib/api";
import { loadSession } from "../../lib/session";
import { OrganizerApp } from "./OrganizerApp";

/**
 * The organizer app, exercised as a component.
 *
 * These are the failures that no type checker and no API-contract test can see,
 * because each of them type-checks, uses a path the server really serves, and
 * still takes the page down: a hook below an early return, a state that is
 * never reset, a destructive action that fires on the first click, and a
 * summary read as a row array.
 */

const SPRING: EventSummary = {
  id: "evt_spring",
  slug: "spring-hack",
  name: "Spring Hack",
  state: "judging",
  submissions_open: false,
  submissions_close: "2026-04-01T00:00:00Z",
};

const WINTER: EventSummary = { ...SPRING, id: "evt_winter", slug: "winter-hack", name: "Winter Hack" };

const ORGANIZER = {
  id: "usr_ada",
  email: "ada@example.com",
  display_name: "Ada Organizer",
  role: "organizer" as const,
};

const UNAVAILABLE = () => new ApiError(503, "unavailable", "The judging service is unavailable.");

function renderOrganizer(route: string) {
  // Nested exactly as App.tsx nests it: OrganizerApp owns a relative <Routes>,
  // so it has to be mounted under the /organizer/* route or every path inside it
  // falls through to its own not-found.
  return render(
    <MemoryRouter initialEntries={[route]}>
      <Routes>
        <Route path="/organizer/*" element={<OrganizerApp />} />
      </Routes>
    </MemoryRouter>,
  );
}

function project(id: string, title: string): Project {
  // Every field the wire sends. The fixture used to declare five of them, and
  // the real type has twenty, so a test fixture that is a strict subset is a
  // fixture of an object the server does not send.
  return {
    id,
    event_id: "evt_1",
    team_id: "tm_1",
    track_id: "trk_1",
    title,
    summary: "",
    description: "",
    story: "",
    repo_url: "",
    live_url: "",
    video_url: "",
    thumbnail_url: "",
    tags: [],
    status: "submitted",
    eligibility: "pending",
    submitted_at: "2026-03-01T00:00:00Z",
    updated_at: "2026-03-01T00:00:00Z",
    version: 1,
  };
}

beforeEach(async () => {
  // The app reads the session from a module-level value the shell normally
  // resolves before the router mounts, so it is resolved here the same way.
  vi.spyOn(api, "me").mockResolvedValue({ data: ORGANIZER });
  await loadSession();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("organizer: a failed load does not take the page down", () => {
  it("renders the error state instead of unmounting on the render that sets it", async () => {
    // The useMemo that filters the panel used to sit below `if (error) return`,
    // so this render called one hook fewer than the last one. React threw
    // "Rendered fewer hooks than expected", and with no error boundary in the
    // tree the organizer got a blank page rather than a message.
    vi.spyOn(api, "events").mockResolvedValue({ data: [SPRING] });
    vi.spyOn(api, "progress").mockRejectedValue(UNAVAILABLE());
    vi.spyOn(api, "panel").mockRejectedValue(UNAVAILABLE());

    renderOrganizer("/organizer");

    expect(await screen.findByText("Progress could not be loaded")).toBeInTheDocument();
    // The page is still mounted around it, which is the half of the bug that a
    // blank screen hid.
    expect(screen.getByRole("heading", { name: "Progress" })).toBeInTheDocument();
    expect(screen.queryByText("The portal could not be reached.")).not.toBeInTheDocument();
  });

  it("clears the error when the query runs again and succeeds", async () => {
    vi.spyOn(api, "events").mockResolvedValue({ data: [SPRING, WINTER] });
    const progress = vi.spyOn(api, "progress");
    const panel = vi.spyOn(api, "panel");
    progress.mockRejectedValueOnce(new ApiError(500, "boom", "The portal could not answer."));
    panel.mockRejectedValueOnce(new ApiError(500, "boom", "The portal could not answer."));
    progress.mockResolvedValue({
      data: {
        event_id: SPRING.id,
        submissions: 12,
        eligible_submissions: 11,
        assignments: 30,
        reviews_started: 4,
        reviews_completed: 2,
        reviews_pending: 28,
      },
    });
    panel.mockResolvedValue({ data: [] });

    renderOrganizer("/organizer");

    expect(await screen.findByText("Progress could not be loaded")).toBeInTheDocument();

    // Without the reset beside setProgress(null), the stale error survived the
    // successful reload and the page showed the failure and the data together.
    await userEvent.selectOptions(screen.getByLabelText("Event"), WINTER.slug);

    await waitFor(() =>
      expect(screen.queryByText("Progress could not be loaded")).not.toBeInTheDocument(),
    );
    expect(screen.getByText("11 eligible")).toBeInTheDocument();
  });
});

describe("organizer: results", () => {
  beforeEach(() => {
    vi.spyOn(api, "events").mockResolvedValue({ data: [SPRING] });
    vi.spyOn(api, "projects").mockResolvedValue({
      data: [project("prj_1", "Alpha"), project("prj_2", "Beta")],
      meta: { page: 1, page_size: 24, total: 2 },
    });
  });

  function stubResults() {
    vi.spyOn(api, "results").mockResolvedValue({
      data: {
        method: "judge-zscore-shrink-logistic",
        method_version: 1,
        confidence: 0.9,
        resamples: 2000,
        reviews: 7,
        projects: [
          {
            project_id: "prj_1",
            raw_mean: 4.38,
            normalized_mean: 63.7,
            low: 58.9,
            high: 69.1,
            review_count: 3,
            low_information_reviews: 0,
            rank: 1,
            separable: true,
            tie_group: 0,
            previous_rank: 0,
          },
          {
            project_id: "prj_2",
            raw_mean: 4.06,
            normalized_mean: 58.7,
            low: 36.4,
            high: 68.8,
            review_count: 4,
            low_information_reviews: 1,
            rank: 2,
            separable: false,
            tie_group: 1,
            previous_rank: 1,
          },
        ],
        low_information_judges: [],
        judges: [],
        criterion_stats: [],
      },
      rubric: null,
    });
  }

  it("reads the judging summary and joins the rows to the project titles", async () => {
    // The endpoint returns a summary with the rows under `projects`, and a row
    // is keyed by project_id. Read as a row array it rendered an empty table
    // with a heading that said nothing.
    stubResults();

    renderOrganizer("/organizer/results");

    expect(await screen.findByText("Alpha")).toBeInTheDocument();
    expect(screen.getByText("Beta")).toBeInTheDocument();
    expect(screen.getByText(/Computed by judge-zscore-shrink-logistic/)).toBeInTheDocument();
    expect(screen.getByText("58.9–69.1")).toBeInTheDocument();
    // The rank this project failed to beat is what makes "Overlapping" a
    // statement about one pair rather than about the whole column.
    expect(screen.getByText("Did not beat rank 1")).toBeInTheDocument();
  });

  it("says so when nothing has been scored", async () => {
    vi.spyOn(api, "results").mockResolvedValue({
      data: {
        method: "judge-zscore-shrink-logistic",
        method_version: 1,
        confidence: 0.9,
        resamples: 2000,
        reviews: 0,
        projects: [],
        low_information_judges: [],
        judges: [],
        criterion_stats: [],
      },
      rubric: null,
    });

    renderOrganizer("/organizer/results");

    expect(await screen.findByText("Nothing has been scored yet")).toBeInTheDocument();
  });

  it("sends one publish however many times the button is clicked", async () => {
    stubResults();
    // Held open, so the button stays busy and every click lands in one batch
    // reading the same state.
    const publish = vi.spyOn(api, "publishResults").mockImplementation(() => new Promise(() => {}));

    renderOrganizer("/organizer/results");
    const button = await screen.findByRole("button", { name: "Publish results" });

    act(() => {
      fireEvent.click(button);
      fireEvent.click(button);
      fireEvent.click(button);
    });

    // Every one of those clicks notifies every team on the event.
    expect(publish).toHaveBeenCalledTimes(1);
    expect(button).toBeDisabled();
  });
});

describe("organizer: revoking a grant", () => {
  const GRANT: Grant = {
    id: "grn_1",
    user_id: "usr_grantee",
    action: "results.read",
    event_id: SPRING.id,
    allow: true,
    reason: "Scout for the next event",
    granted_by: "usr_ada",
    created_at: "2026-03-01T10:00:00Z",
  };

  it("asks first, and reports a refused revoke instead of failing silently", async () => {
    vi.spyOn(api, "events").mockResolvedValue({ data: [SPRING] });
    vi.spyOn(api, "grants").mockResolvedValue({ data: [GRANT], count: 1 });
    const revoke = vi
      .spyOn(api, "revokeGrant")
      .mockRejectedValue(new ApiError(403, "forbidden", "You may not revoke a grant on this event."));

    renderOrganizer("/organizer/grants");

    await userEvent.click(await screen.findByRole("button", { name: "Revoke" }));

    // Nothing has been sent. Revoke is immediate and cannot be undone.
    expect(revoke).not.toHaveBeenCalled();
    const dialog = screen.getByRole("dialog", { name: "Revoke this grant?" });
    expect(dialog).toHaveTextContent("results.read");
    expect(dialog).toHaveTextContent("usr_grantee");

    await userEvent.click(within(dialog).getByRole("button", { name: "Revoke" }));

    // The row is still in force, so the failure has to be on the page.
    expect(await screen.findByText("That grant was not revoked")).toBeInTheDocument();
    expect(screen.getByText(/The grant is still in force/)).toBeInTheDocument();
    expect(revoke).toHaveBeenCalledWith(GRANT.id);
  });

  it("closes on Escape without revoking", async () => {
    vi.spyOn(api, "events").mockResolvedValue({ data: [SPRING] });
    vi.spyOn(api, "grants").mockResolvedValue({ data: [GRANT], count: 1 });
    const revoke = vi.spyOn(api, "revokeGrant").mockResolvedValue({ data: { id: GRANT.id, revoked: true } });

    renderOrganizer("/organizer/grants");
    await userEvent.click(await screen.findByRole("button", { name: "Revoke" }));
    await userEvent.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(revoke).not.toHaveBeenCalled();
  });
});
