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
    //
    // Only /v1 is proxied. /login and /logout were here for the server-rendered
    // form routes, which are retired; leaving them in meant the dev server handed
    // the app's own sign-in page to the Go binary, which answered with the
    // embedded production shell. So /login rendered a stale build while every
    // other route rendered live source, and the two were not comparable. A
    // client route must be served by the dev server that compiles it.
    //
    // /brand is left alone too: the Go binary serves those files, and the
    // frontend's own copies under web/public are equivalent.
    proxy: {
      "/v1": "http://localhost:8080",
    },
  },
});
