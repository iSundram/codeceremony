import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Tests run against the same component tree the app ships, with the same
// stylesheets, so a token that a component relies on cannot go missing between
// a passing test and a broken page.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
