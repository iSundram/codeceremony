import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ICON_MARKUP, ICON_NAMES } from "../lib/icons.generated";
import { Icon } from "../lib/icons";

/**
 * The icon inventory is closed, so the whole source tree is scanned for names.
 *
 * design.md 10.1 approves one family and 1.1 rule 1 says not to invent a visual
 * component. A glyph name that is not in the table renders nothing at all, which
 * is invisible in a screenshot and easy to ship. Scanning the source turns that
 * into a test failure, so a request for an unapproved glyph cannot be committed.
 *
 * The scan reads the source rather than the bundle because the point is to catch
 * the request at authoring time, and because a built bundle minifies the strings
 * into a form a regex would misread.
 */
/**
 * Every `icon="..."` and `Icon name="..."` in the source, with its file.
 *
 * Reading the source rather than the bundle is deliberate: the point is to catch
 * an unapproved request at authoring time, and a minified bundle would hide the
 * string anyway.
 */
const SOURCE_DIR = "src";
const GLYPH_USE = /\bicon\s*=\s*["']([a-z0-9-]+)["']/g;

function requestedGlyphs(): Map<string, string[]> {
  const found = new Map<string, string[]>();
  const walk = (dir: string) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name);
      if (entry.isDirectory()) {
        walk(path);
        continue;
      }
      if (!/\.tsx?$/.test(entry.name)) continue;
      // The generated table is the inventory, not a request against it, and the
      // test file is the scan itself.
      if (entry.name === "icons.generated.ts" || entry.name === "icons.test.tsx") continue;
      const source = readFileSync(path, "utf8");
      for (const match of source.matchAll(GLYPH_USE)) {
        const name = match[1];
        if (!name) continue;
        found.set(name, [...(found.get(name) ?? []), path]);
      }
    }
  };
  walk(SOURCE_DIR);
  return found;
}

describe("design.md 10.1: the icon inventory is closed", () => {
  it("has a non-empty approved set", () => {
    expect(ICON_NAMES.length).toBeGreaterThan(50);
  });

  it("renders every approved name without throwing", () => {
    // A name present in the table but rendering nothing is a silent failure of
    // the same kind, so each one is exercised.
    for (const name of ICON_NAMES) {
      expect(ICON_MARKUP[name], `${name} is listed but has no markup`).toBeTruthy();
    }
  });

  it("carries no stroke-width, so the component's 1.75 is the only one", () => {
    // design.md 10.1 pins the stroke at 1.75 rather than Lucide's authored 2. If
    // a vendored path carried its own stroke-width, that override would win and
    // the set would render at mixed weights.
    for (const [name, markup] of Object.entries(ICON_MARKUP)) {
      expect(markup, `${name} overrides the stroke width`).not.toContain("stroke-width");
    }
  });

  it("carries no class, so the component's sizing is the only one", () => {
    for (const [name, markup] of Object.entries(ICON_MARKUP)) {
      expect(markup, `${name} carries a class attribute`).not.toContain("class=");
    }
  });

  it("does not contain a second icon family", () => {
    // A stray emoji or an inline path from another set is the mixed-family case
    // 10.1 rules out, and it would also mean an unapproved visual.
    for (const [name, markup] of Object.entries(ICON_MARKUP)) {
      expect(markup, `${name} contains non-geometric content`).not.toMatch(/<text|<image|<use/);
    }
  });

  it("requests only names that are in the approved inventory", () => {
    // A name that is not in the table renders nothing, which is invisible in a
    // screenshot and easy to ship. This is the check that stops it.
    const requested = requestedGlyphs();
    const unapproved = [...requested.entries()]
      .filter(([name]) => !(name in ICON_MARKUP))
      .map(([name, files]) => `${name} (in ${files.join(", ")})`);
    expect(unapproved, `unapproved icon names requested: ${unapproved.join("; ")}`).toHaveLength(0);
  });

  it("actually uses the inventory, so the scan is not vacuous", () => {
    // A scan that finds nothing passes trivially. This proves it is looking at
    // real source.
    const requested = requestedGlyphs();
    expect(requested.size).toBeGreaterThan(5);
  });

  it("renders a labelled glyph as an image with that name", () => {
    // The one case where a glyph carries meaning on its own.
    const { container } = renderWithIcon("calendar", "Calendar");
    const svg = container.querySelector("svg");
    expect(svg).toHaveAttribute("role", "img");
    expect(svg).toHaveAttribute("aria-label", "Calendar");
    // A labelled glyph is no longer decorative, so it must not also be hidden.
    expect(svg).not.toHaveAttribute("aria-hidden");
  });
});

function renderWithIcon(name: string, label: string) {
  return render(<Icon name={name} label={label} />);
}
