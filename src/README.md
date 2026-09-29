# src/

The DOGFOOD submission checklist names a `src/` directory for the code written
during the window. The code is a Go module, so it lives in `backend/` where the
Go toolchain, the Dockerfile and the module resolver expect it. Moving it here
would mean a module with no `go.mod` next to it, and no benefit.

| Directory | What is in it |
|---|---|
| `../backend/cmd/codeceremony` | entry point, boot order, composition root |
| `../backend/internal/domain` | types and their invariants, no internal dependencies |
| `../backend/internal/store` | durable state, snapshot and restore |
| `../backend/internal/persistence` | the journal that writes state to disk |
| `../backend/internal/judging` | normalization, confidence intervals, Bradley-Terry |
| `../backend/internal/fixtures` | the shared DOGFOOD fixture loader |
| `../backend/internal/seed` | built-in demo data, fixed credential table |
| `../backend/internal/auth` | sessions, tokens, password hashing |
| `../backend/internal/authz` | the action vocabulary, the resolver, the grant checks |
| `../backend/internal/httpapi` | routing, JSON API, and the embedded SPA and pages |
| `../backend/internal/httpapi/webassets` | the `go:embed` tree: SPA build, templates, stylesheet, brand |
| `../backend/internal/ratelimit` | fixed-window budgets |
| `../backend/internal/mailer` | templates, queue, dispatch, webhooks |
| `../backend/internal/config` | environment parsing and validation |
| `../web` | the Vite/React/TypeScript frontend, built and embedded, not served by Node |

Tests live beside the code they cover, as `_test.go` files, which is the Go
convention and what `go test ./...` discovers. The cross-process suite is in
`../tests/`, the Vitest suite in `../web/src/test/`.

Start with [`../ARCHITECTURE.md`](../ARCHITECTURE.md) for why it is arranged
this way, and [`../DATA-MODEL.md`](../DATA-MODEL.md) for the schema.
