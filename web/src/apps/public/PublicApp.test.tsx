import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PublicApp } from "./PublicApp";

/**
 * The public surface, against the two responses that made it wrong.
 *
 * The route is declared `/events/*`, so the only param react-router yields is the
 * splat under "*": reading params.slug gave undefined for every address, which
 * sent every event link to the directory. And the gallery page ended at the
 * server's 24-project page cap, because nothing read meta.total.
 */

const EVENT = {
  id: "evt_1",
  slug: "demo",
  name: "Demo Jam",
  summary: "A weekend of building",
  description: "Two days, one brief, a panel of five.",
  timezone: "UTC",
  state: "judging",
  submissions_open: true,
  submissions_close: "2026-03-01T00:00:00Z",
};

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function project(index: number) {
  return {
    id: `prj_${index}`,
    title: `Project ${index}`,
    summary: "Something built over a weekend",
    track_id: "trk_1",
    status: "submitted",
    tags: ["go"],
  };
}

describe("the public surface", () => {
  const requested: string[] = [];

  async function handle(input: RequestInfo | URL): Promise<Response> {
    const url = new URL(String(input), "http://portal.test");
    requested.push(url.pathname + url.search);

    if (url.pathname === "/v1/events") return json({ data: [EVENT] });
    if (url.pathname === "/v1/events/demo/projects") {
      const page = Number(url.searchParams.get("page") ?? "1");
      const total = 30;
      const start = (page - 1) * 24;
      const size = Math.min(24, total - start);
      return json({
        data: Array.from({ length: size }, (_, index) => project(start + index + 1)),
        meta: { page, page_size: 24, total },
      });
    }
    if (url.pathname === "/v1/events/demo") {
      return json({
        data: EVENT,
        tracks: [{ id: "trk_1", event_id: "evt_1", name: "General", slug: "general", order: 1 }],
        prizes: [{ id: "prz_1", event_id: "evt_1", name: "Grand prize", description: "Best overall", rank: 1 }],
        milestones: [
          {
            id: "mls_1",
            event_id: "evt_1",
            title: "Submissions close",
            detail: "Nothing after this",
            due_at: "2026-03-01T00:00:00Z",
            position: 1,
          },
        ],
        questions: [],
        hosts: [],
        rubric: null,
        team_policy: { min_size: 2, max_size: 5, allow_solo: true, requires_approval: false },
        judge_count: 5,
      });
    }
    return new Response("not found", { status: 404 });
  }

  function renderAt(path: string) {
    return render(
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/events/*" element={<PublicApp />} />
        </Routes>
      </MemoryRouter>,
    );
  }

  beforeEach(() => {
    requested.length = 0;
    vi.stubGlobal("fetch", vi.fn(handle));
  });

  it("reads the event out of the splat the route actually yields", async () => {
    renderAt("/events/demo");

    // params.slug is undefined on a `/events/*` route, so this used to render the
    // directory instead of the event.
    expect(await screen.findByRole("heading", { name: "Demo Jam" })).toBeInTheDocument();
    // tracks, prizes and milestones are siblings of data; reading them off the
    // event threw on the first length.
    expect(screen.getByText("General")).toBeInTheDocument();
    expect(screen.getByText("Grand prize")).toBeInTheDocument();
    expect(screen.getByText("Submissions close")).toBeInTheDocument();
  });

  it("opens the gallery from the event's own sub-route", async () => {
    renderAt("/events/demo/gallery");

    expect(await screen.findByText("Project 1")).toBeInTheDocument();
    expect(requested.some((path) => path.startsWith("/v1/events/demo/projects"))).toBe(true);
  });

  it("pages past the server's page cap", async () => {
    const user = userEvent.setup();
    renderAt("/events/demo/gallery");

    await screen.findByText("Project 1");
    // 30 projects at 24 a page is two pages, and before meta.total was read the
    // second page had no way to be reached at all.
    expect(screen.getByText(/30 projects/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Next" }));

    expect(await screen.findByText("Project 25")).toBeInTheDocument();
    await waitFor(() =>
      expect(requested).toContain("/v1/events/demo/projects?page=2"),
    );
  });

  it("keeps /events/gallery working for the sidebar link", async () => {
    renderAt("/events/gallery");

    expect(await screen.findByText("Project 1")).toBeInTheDocument();
  });
});
