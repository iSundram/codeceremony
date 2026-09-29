import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The portal is one Go binary. This build produces static assets that the
// binary serves, so the production image carries no Node runtime: the
// one-command rule survives, because `docker compose up` needs a network only to
// fetch npm packages while building the image, never to run the portal.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // Shipping sourcemaps would publish the whole component tree to anyone who
    // asked. The test runner reads from source instead.
    sourcemap: false,
    rollupOptions: {
      output: { manualChunks: { vendor: ["react", "react-dom"] } },
    },
  },
  server: {
    port: 5173,
    // In production the SPA and the API share an origin, so there is no CORS
    // configuration and no credential difference between environments.
    proxy: {
      "/v1": "http://localhost:8080",
      "/login": "http://localhost:8080",
      "/logout": "http://localhost:8080",
      "/brand": "http://localhost:8080",
    },
  },
});
