import "@testing-library/jest-dom/vitest";

// jsdom does not implement matchMedia, and the shell queries
// prefers-reduced-motion. Without a stub, rendering the shell throws.
if (!window.matchMedia) {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}
