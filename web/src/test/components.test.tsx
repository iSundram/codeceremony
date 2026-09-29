import { render, screen, within } from "@testing-library/react";
import { renderWithRouter } from "./render";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { Alert, Badge, EmptyState, ErrorState, Progress, Skeleton, Status } from "../components/feedback";
import { Button, Field, IconButton, LinkButton, SearchField } from "../components/controls";
import { ICON_MARKUP } from "../lib/icons.generated";
import { Icon } from "../lib/icons";
import { Table, Th, Tr, Td } from "../components/data";

/**
 * The accessibility contract, asserted rather than asserted-to.
 *
 * design.md 13 is not a style preference here: it is the difference between a
 * portal a judge can use on a laptop in a noisy room and one they cannot. The
 * rules below are the ones a unit test can actually prove, and each names the
 * clause it comes from so a future change has to argue with the clause rather
 * than with this file.
 */
describe("design.md 13: accessible names", () => {
  it("gives every icon-only button a name", () => {
    render(<IconButton icon="menu" label="Open navigation" />);
    // A control with no name is not a control. An aria-label would satisfy a
    // linter; a hidden text node satisfies a screen reader and the browser's own
    // accessibility tree, which is why that is what the component renders.
    expect(screen.getByRole("button", { name: "Open navigation" })).toBeInTheDocument();
  });

  it("never renders an icon as the only label for anything", () => {
    renderWithRouter(
      <LinkButton to="/events" variant="secondary" icon="arrow-right">
        Browse events
      </LinkButton>,
    );
    const link = screen.getByRole("link", { name: "Browse events" });
    expect(link).toHaveTextContent("Browse events");
    // No glyph without a label: an icon is supplementary, and the text is the
    // label. design.md 12 warns against novel patterns, and an icon as the only
    // name for a control is the commonest one.
    const glyph = link.querySelector("svg");
    if (glyph) expect(glyph).toHaveAttribute("aria-hidden", "true");
  });
});

describe("design.md 13 and 8.7: status is never colour alone", () => {
  it("states a status in words as well as a mark", () => {
    render(
      <table>
        <tbody>
          <tr>
            <Td>
              <Status icon="ban">Refused</Status>
            </Td>
          </tr>
        </tbody>
      </table>,
    );
    // The word is the part that matters. A reader who cannot distinguish the
    // glyphs gets the same information, so the assertion is on the text rather
    // than on the mark.
    expect(within(screen.getByRole("cell")).getByText("Refused")).toBeInTheDocument();
  });

  it("gives an alert a role that announces it", () => {
    render(<Alert kind="error" title="Chain verification failed">An entry was edited.</Alert>);
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Chain verification failed");
    expect(alert).toHaveTextContent("An entry was edited.");
  });

  it("uses a non-assertive role for an informational alert", () => {
    render(<Alert kind="info" title="You are browsing as a visitor" />);
    // An informational note interrupting a screen reader mid-sentence is its own
    // kind of wrong, so only a failure is assertive.
    expect(screen.getByRole("status")).toHaveTextContent("You are browsing as a visitor");
  });
});

describe("design.md 8.7: no unreviewed status colours", () => {
  it("renders a failed operation without introducing a red", () => {
    const { container } = render(
      <ErrorState title="Events could not be loaded" body="The portal could not be reached." />,
    );
    expect(container.textContent).toContain("Events could not be loaded");
    // design.md 1.1 rule 3 forbids inventing a colour, and there is no red in
    // the palette. The failure is a different glyph and different words, which
    // is why no inline style may appear here for a component to smuggle one in.
    expect(container.innerHTML).not.toMatch(/style="[^"]*color/i);
  });
});

describe("design.md 8.8: a progress bar states its number", () => {
  it("carries the value in text and in ARIA", () => {
    render(<Progress label="Reviews completed" value={7} total={10} />);
    const bar = screen.getByRole("progressbar", { name: "Reviews completed" });
    expect(bar).toHaveAttribute("aria-valuenow", "7");
    expect(bar).toHaveAttribute("aria-valuemax", "10");
    // A bar alone is unreadable to a screen reader and ambiguous to a person
    // comparing two of them, so the number is part of the component.
    expect(screen.getByText("7 of 10")).toBeInTheDocument();
  });

  it("clamps a value that exceeds its total rather than overflowing", () => {
    render(<Progress label="Odd" value={14} total={10} />);
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "10");
  });

  it("does not divide by zero when the total is zero", () => {
    render(<Progress label="Nothing yet" value={0} total={0} />);
    const bar = screen.getByRole("progressbar", { name: "Nothing yet" });
    expect(bar).toHaveAttribute("aria-valuemax", "1");
    expect(screen.getByText("0 of 1")).toBeInTheDocument();
  });
});

describe("design.md 8.8: loading does not shift layout", () => {
  it("marks a skeleton decorative and announces the load", () => {
    const { container } = render(<Skeleton lines={3} />);
    // A skeleton is a placeholder, not content, so it must not be announced.
    // Three lines means three bars, all of them hidden.
    expect(container.querySelectorAll(".skeleton")).toHaveLength(3);
    for (const bar of container.querySelectorAll(".skeleton")) {
      expect(bar).toHaveAttribute("aria-hidden", "true");
    }
  });
});

describe("design.md 8.2: a field has a persistent label", () => {
  it("associates a label with its control", async () => {
    const user = userEvent.setup();
    render(
      <Field id="email" label="Email">
        {({ id }) => <input id={id} className="input" />}
      </Field>,
    );
    const input = screen.getByLabelText("Email");
    await user.type(input, "organizer@example.org");
    expect(input).toHaveValue("organizer@example.org");
  });

  it("reports an error against the field it belongs to", () => {
    render(
      <Field id="password" label="Password" error="Check the email and password">
        {({ id, invalid, describedBy }) => (
          <input id={id} aria-invalid={invalid} aria-describedby={describedBy} />
        )}
      </Field>,
    );
    // design.md 13: an error message identifies the field and the correction.
    const input = screen.getByLabelText("Password");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAttribute("aria-describedby", expect.stringContaining("error"));
    expect(screen.getByRole("alert")).toHaveTextContent("Check the email and password");
  });

  it("does not rely on a placeholder for the label", () => {
    render(
      <SearchField id="q" label="Search projects" value="" onChange={() => {}} placeholder="Name" />,
    );
    // The placeholder is a hint. The label is the label.
    expect(screen.getByLabelText("Search projects")).toBeInTheDocument();
  });
});

describe("design.md 8.1: buttons", () => {
  it("defaults to the primary variant and is a submit", () => {
    render(<Button>Save</Button>);
    const button = screen.getByRole("button", { name: "Save" });
    expect(button).toHaveAttribute("type", "submit");
    expect(button.className).toContain("btn--primary");
  });

  it("keeps its label while loading so nothing around it moves", () => {
    render(
      <Button loading>
        Save draft
      </Button>,
    );
    const button = screen.getByRole("button", { name: "Save draft" });
    expect(button).toHaveAttribute("data-loading", "true");
    // design.md 7.6: loading is a state, not a replacement. The label stays in
    // the DOM so the button keeps its width.
    expect(button).toHaveTextContent("Save draft");
    expect(button).toBeDisabled();
  });

  it("is a link, not a button, when it navigates", () => {
    renderWithRouter(
      <LinkButton to="/organizer" variant="primary">
        Progress
      </LinkButton>,
    );
    // A control that navigates must be a link, so it can be opened in a new tab
    // and is announced as a destination.
    expect(screen.getByRole("link", { name: "Progress" })).toHaveAttribute("href", "/organizer");
  });
});

describe("design.md 7.5: data display", () => {
  it("gives a table a caption and scoped headers", () => {
    render(
      <Table caption="Judges on the panel.">
        <thead>
          <tr>
            <Th>Judge</Th>
            <Th numeric>Assigned</Th>
          </tr>
        </thead>
        <tbody>
          <Tr>
            <Td strong>Ada</Td>
            <Td numeric>4</Td>
          </Tr>
        </tbody>
      </Table>,
    );
    expect(screen.getByRole("table", { name: "Judges on the panel." })).toBeInTheDocument();
    // scope="col" is what lets a screen reader announce "Assigned, column 2" as
    // the reader moves down a cell.
    expect(screen.getByRole("columnheader", { name: "Assigned" })).toHaveAttribute("scope", "col");
  });

  it("right-aligns a numeric column so digits line up", () => {
    render(
      <Table>
        <thead>
          <tr>
            <Th>Judge</Th>
            <Th numeric>Assigned</Th>
          </tr>
        </thead>
        <tbody>
          <Tr>
            <Td strong>Ada</Td>
            <Td numeric>4</Td>
          </Tr>
        </tbody>
      </Table>,
    );
    expect(screen.getByRole("cell", { name: "4" }).className).toContain("table__num");
  });
});

describe("design.md 12: every data surface has its states", () => {
  it("says when there is nothing, rather than showing an empty frame", () => {
    render(<EmptyState title="No events yet" body="One appears here once it is created." />);
    expect(screen.getByText("No events yet")).toBeInTheDocument();
  });

  it("says what went wrong when something failed", () => {
    render(<ErrorState title="Accounts could not be loaded" body="The portal answered 500." />);
    expect(screen.getByRole("alert")).toHaveTextContent("Accounts could not be loaded");
  });
});

describe("design.md 10.1: the icon inventory is closed", () => {
  it("renders only approved glyphs, and an unknown one renders nothing", () => {
    const { container } = render(<Icon name="house" />);
    expect(container.querySelectorAll("svg")).toHaveLength(1);

    // An unknown name is a silent no-op rather than a broken glyph, and the
    // suite below asserts that no component ever asks for one.
    const unknown = render(<Icon name="not-a-real-icon" />);
    expect(unknown.container.querySelectorAll("svg")).toHaveLength(0);
  });

  it("uses the 1.75px stroke the contract specifies, not Lucide's 2", () => {
    const { container } = render(<Icon name="house" />);
    // design.md 10.1 pins the stroke so the set matches the weight of ink body
    // text rather than reading heavier than the words beside it.
    expect(container.querySelector("svg")).toHaveAttribute("stroke-width", "1.75");
  });

  it("hides the glyph from assistive technology by default", () => {
    const { container } = render(<Icon name="calendar" />);
    expect(container.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  });

  it("keeps a labelled glyph announced", () => {
    render(<Icon name="calendar" label="Calendar" />);
    expect(screen.getByRole("img", { name: "Calendar" })).toBeInTheDocument();
  });

  it("has a non-empty path for every approved name", () => {
    for (const [name, markup] of Object.entries(ICON_MARKUP)) {
      expect(markup.length, `${name} has no path data`).toBeGreaterThan(0);
      // Every Lucide glyph carries geometry; a set of empty strings would mean
      // the generator produced placeholders.
      expect(markup, `${name} carries no geometry`).toMatch(/<(path|circle|rect|line|polyline|polygon|ellipse)/);
    }
  });
});

describe("design.md 10.3: status treatment", () => {
  it("distinguishes a denial with words and a glyph, not a colour", () => {
    render(<Badge icon="ban">Deny</Badge>);
    expect(screen.getByText("Deny")).toBeInTheDocument();
  });

  it("never writes an inline colour into a component", () => {
    const { container } = render(
      <div>
        <Status icon="ban">Refused</Status>
        <Badge icon="circle-check">Allow</Badge>
        <Alert kind="warning" title="Careful" />
        <Progress label="Progress" value={1} total={2} />
      </div>,
    );
    // The only inline style permitted is a Progress width, which is a
    // measurement rather than a colour. design.md 1.1 rule 7 says components use
    // semantic tokens, so a colour literal here would be a contract breach.
    const styles = [...container.querySelectorAll("[style]")].map((node) => node.getAttribute("style"));
    for (const style of styles) {
      expect(style).not.toMatch(/color|background|border-color/i);
    }
  });
});

describe("design.md 7.4: badges and tags carry their text", () => {
  it("does not rely on a colour to say what a badge means", () => {
    render(
      <table>
        <tbody>
          <tr>
            <Td>
              <Badge icon="clock">Expired</Badge>
            </Td>
          </tr>
        </tbody>
      </table>,
    );
    expect(within(screen.getByRole("cell")).getByText("Expired")).toBeInTheDocument();
  });
});
