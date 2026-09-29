import type { ReactElement } from "react";
import { render, type RenderOptions } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

/**
 * renderWithRouter renders a component that contains a Link.
 *
 * LinkButton and the sidebar both use react-router's Link, and a Link outside a
 * router throws on a null context rather than failing quietly. Wrapping here
 * means a component test does not have to know that, and the path is a parameter
 * so a test that cares about the current location can set one.
 */
export function renderWithRouter(
  ui: ReactElement,
  { route = "/", ...options }: RenderOptions & { route?: string } = {},
) {
  return render(ui, {
    wrapper: ({ children }) => <MemoryRouter initialEntries={[route]}>{children}</MemoryRouter>,
    ...options,
  });
}
