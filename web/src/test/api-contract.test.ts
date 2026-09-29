import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * The API client and the server must agree on every path.
 *
 * The frontend was written against the documented routes, and several of the
 * paths it guessed did not exist. A mismatch is a 404 in production and nothing
 * at all in development, so it is checked here rather than discovered by a user
 * clicking a link. This is the test that caught six wrong paths the first time
 * it ran: /v1/account/whoami, /v1/teams, /v1/organizer/publish, /v1/events/{slug}
 * under the wrong verb, and two more.
 */
const SERVER = "../backend/internal/httpapi";

/** Every route the Go mux registers, as "METHOD /path". */
function serverRoutes(): { method: string; path: string }[] {
  const out: { method: string; path: string }[] = [];
  const walk = (dir: string) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name);
      if (entry.isDirectory()) {
        walk(path);
        continue;
      }
      if (!entry.name.endsWith(".go") || entry.name.endsWith("_test.go")) continue;
      const source = readFileSync(path, "utf8");
      for (const match of source.matchAll(/mux\.Handle(?:Func)?\("([A-Z]+ )?([^"]+)"/g)) {
        out.push({
          method: match[1]?.trim() || "GET",
          // Routes are registered under /v1; the client is written without the
          // prefix because that lives in one constant in the client.
          path: (match[2]?.trim() ?? "").replace(/^\/v1/, ""),
        });
      }
    }
  };
  walk(SERVER);
  return [...new Map(out.map((r) => [`${r.method} ${r.path}`, r])).values()];
}

/**
 * Every path the client names, with the template segments blanked.
 *
 * `${slug}` becomes `{}` so it matches any single server segment, and a query
 * string is dropped because a route is a path.
 */
function clientPaths(): Map<string, string[]> {
  const found = new Map<string, string[]>();
  const add = (raw: string, file: string) => {
    // Only the path part is compared, because a route is a path.
    //
    // Every marker is a path segment. `/submissions/${id}` and
    // `/grants${search ? "?" : ""}` both reduce to a real segment once the
    // query string is gone, so all of them become `{}` and the server's
    // `{projectID}` matches them.
    const path = raw
      .split("?")[0]
      ?.replace(/^\/+/, "/")
      .replace(/\/{2,}/g, "/")
      .replace(/@@/g, "{}")
      // A query marker contributes nothing to the path, so it and any empty
      // segment it leaves behind are removed together.
      .replace(/(\/\{\})?~~\}?$/, "")
      .replace(/\/+$/, "");
    if (!path || path === "/" || !path.startsWith("/")) return;
    found.set(path, [...(found.get(path) ?? []), file]);
  };

  const walk = (dir: string) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name);
      if (entry.isDirectory()) {
        walk(path);
        continue;
      }
      if (!/\.tsx?$/.test(entry.name)) continue;
      // A path is often built with a nested template, e.g.
      //   `/grants${search ? `?${search}` : ""}`
      // whose backticks a backtick-delimited pattern cannot span. The inner
      // backticks are removed first so the literal is a single one, and every
      // interpolation is then collapsed to a marker. add() decides whether a
      // marker was a path segment or a query fragment.
      // A query fragment becomes a distinct marker, because a segment marker
      // and a query marker mean different things: `/submissions/${id}` has a
      // segment there and `/grants${search ? "?" : ""}` does not.
      const source = readFileSync(path, "utf8")
        .replace(/\$\{`[^`]*`\}/g, "~~")
        .replace(/\$\{[^}]*\?[^}]*\}/g, "~~")
        .replace(/\$\{(?:[^{}]|\{[^{}]*\})*\}/g, "@@");
      // Only a path that reaches request() is an API call. A `to="/projects/@@"`
      // in a router link is a frontend route with no endpoint behind it, and
      // counting it would report every client route as missing.
      const calls = [
        ...source.matchAll(/request<[^>]*>\(\s*`([^`]*@@[^`]*)`/g),
        ...source.matchAll(/request<[^>]*>\(\s*"([^"]+)"/g),
      ];
      for (const match of calls) {
        if (match[1]?.startsWith("/")) add(match[1], path);
      }
    }
  };
  walk("src");
  return found;
}

/** "/grants/{}" matches "GET /grants/{grantID}". */
function matches(clientPath: string, serverPath: string): boolean {
  const client = clientPath.split("/").filter(Boolean);
  const server = serverPath.split("/").filter(Boolean);
  if (client.length !== server.length) return false;
  return client.every((segment, index) => {
    if (segment === "{}") return !server[index]?.includes(".");
    return segment === server[index];
  });
}

describe("the API client and the server agree on every path", () => {
  it("has routes to check against", () => {
    // A scan that finds nothing passes vacuously, so the surface is proven first.
    expect(serverRoutes().length).toBeGreaterThan(50);
  });

  it("has client calls to check", () => {
    expect(clientPaths().size).toBeGreaterThan(8);
  });

  it("requests only paths that exist on the server", () => {
    const routes = serverRoutes();
    const unmatched: string[] = [];
    for (const [clientPath, files] of clientPaths()) {
      if (!routes.some((route) => matches(clientPath, route.path))) {
        unmatched.push(`${clientPath} (in ${[...new Set(files)].join(", ")})`);
      }
    }
    expect(unmatched, `client paths with no server route: ${unmatched.join("; ")}`).toHaveLength(0);
  });

  it("uses the right method on the paths it mutates", () => {
    const routes = serverRoutes();
    // Every write in the client must be a write on the server. A GET sent to a
    // POST-only route is a 405 that looks like a bug report.
    const source = readFileSync("src/lib/api.ts", "utf8");
    const mismatches: string[] = [];
    for (const match of source.matchAll(/request<[^>]*>\(\s*`?([^`",]+)`?\s*,\s*\{([^}]*)method:\s*"([A-Z]+)"/g)) {
      const [, clientPath, , method] = match;
      if (!clientPath) continue;
      const normalised = clientPath.replace(/\$\{[^}]+\}/g, "{}");
      if (!routes.some((route) => route.method === method && matches(normalised, route.path))) {
        mismatches.push(`${method} ${normalised}`);
      }
    }
    expect(mismatches, `method mismatches: ${mismatches.join("; ")}`).toHaveLength(0);
  });
});
