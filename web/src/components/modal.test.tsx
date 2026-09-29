import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Modal } from "./feedback";

describe("modal smoke", () => {
  it("is a labelled dialog that traps and closes", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(
      <>
        <button type="button">Open</button>
        <Modal open title="Revoke this grant?" onClose={onClose} actions={<button type="button">Revoke</button>}>
          <p>This cannot be undone.</p>
        </Modal>
      </>,
    );
    const dialog = screen.getByRole("dialog", { name: "Revoke this grant?" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog.className).toContain("modal");
    expect(screen.getByText("This cannot be undone.")).toBeInTheDocument();

    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);

    // Tab from the last focusable wraps to the first.
    const focusables = dialog.querySelectorAll<HTMLElement>('a[href], button:not([disabled])');
    const last = focusables[focusables.length - 1]!;
    last.focus();
    await user.tab();
    expect(focusables[0]).toHaveFocus();
  });

  it("renders nothing when closed", () => {
    const { container } = render(<Modal open={false} title="x" onClose={() => {}} />);
    expect(container.firstChild).toBeNull();
  });
});
