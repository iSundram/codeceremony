# CodeCeremony — Project Structure

> **Superseded.** This is the pre-implementation plan, kept as a record of the
> thinking. Some of it no longer matches the code: the portal has no PostgreSQL
> and no `apps/web` frontend, because the one-command-offline rule ruled both
> out. For what is actually built, read
> [`ARCHITECTURE.md`](ARCHITECTURE.md), [`DATA-MODEL.md`](DATA-MODEL.md),
> [`JUDGING.md`](JUDGING.md) and [`README.md`](README.md).

> **Status:** Structure and stack baseline. Backend implementation is in progress under `backend/`.
>
> This document defines repository boundaries and technical direction. Product behavior belongs in `FEATURES.md`; visual rules belong in `design.md`; event constraints belong in `rules.md`.
>
> **Implementation rule:** Continue only within the authorized event window and keep the tier claims honest.

## 1. Product name

- **Name:** CodeCeremony
- **Repository name:** `codeceremony`
- **Display capitalization:** `CodeCeremony`
- **Project type:** Full-stack web application with a Go backend and a TypeScript single-page frontend.

## 2. Technical direction

This is the initial stack direction. Exact package versions and library choices can be finalized during planning without changing the repository boundaries.

| Area | Direction | Location |
|---|---|---|
| Backend | Go, HTTP service, JSON API | `backend/` |
| Frontend | TypeScript, Next.js, React, SPA behavior | `apps/web/` |
| Shared contract | TypeScript types and API schema | `packages/contracts/` |
| Primary database | PostgreSQL, migration-driven | `infra/postgres/` and `backend/` |
| Local runtime | Docker Compose | `infra/compose/` |
| Backend testing | Go test tooling | `backend/` |
| Frontend testing | TypeScript test tooling; browser tests reserved | `apps/web/` |
| API documentation | OpenAPI contract | `packages/contracts/` |
| CI | GitHub Actions | `.github/workflows/` |
| Formatting and linting | Go tooling plus TypeScript tooling | `backend/` and `apps/web/` |
| Brand assets | Logo, marks, color tokens, typography assets | `assets/brand/` |

The frontend is intended to operate as a client-side SPA while using Next.js as the web framework. The backend is a separate service; the browser does not access the database directly.

## 3. Repository layout

```text
codeceremony/
├── backend/
│   ├── cmd/
│   │   └── codeceremony/
│   │       └── main.go
│   ├── internal/
│   │   ├── auth/
│   │   ├── config/
│   │   ├── domain/
│   │   ├── httpapi/
│   │   ├── seed/
│   │   └── store/
│   ├── migrations/
│   ├── go.mod
│   ├── go.sum
│   └── Dockerfile
├── apps/
│   └── web/
│       ├── src/
│       │   ├── app/
│       │   ├── components/
│       │   ├── hooks/
│       │   ├── lib/
│       │   ├── styles/
│       │   └── assets/
│       ├── public/
│       ├── tests/
│       ├── package.json
│       ├── tsconfig.json
│       ├── next.config.*
│       └── Dockerfile
├── packages/
│   └── contracts/
│       ├── src/
│       ├── schemas/
│       ├── generated/
│       ├── package.json
│       └── tsconfig.json
├── infra/
│   ├── compose/
│   │   ├── docker-compose.yml
│   │   └── docker-compose.override.example.yml
│   ├── postgres/
│   │   ├── init/
│   │   └── seed/
│   └── scripts/
├── assets/
│   └── brand/
│       ├── logo/
│       ├── marks/
│       ├── colors/
│       ├── typography/
│       └── README.md
├── docs/
│   ├── BRAND.md
│   ├── decisions/
│   ├── local-development.md
│   └── operations/
├── scripts/
├── .github/
│   └── workflows/
├── .env.example
├── .gitignore
├── .dockerignore
├── docker-compose.yml
├── Makefile
├── README.md
├── ARCHITECTURE.md
├── DATA-MODEL.md
├── JUDGING.md
├── ACCOUNT-MANAGEMENT.md
├── FEATURES.md
├── acceptance-report.txt
├── .dogfood.toml
└── LICENSE
```

The root-level `ARCHITECTURE.md`, `DATA-MODEL.md`, `JUDGING.md`, `README.md`, `ACCOUNT-MANAGEMENT.md`, `FEATURES.md`, `acceptance-report.txt`, `.dogfood.toml`, `docker-compose.yml`, and `LICENSE` are required or normative project documents. The directories above describe where supporting code and documentation may live; they do not imply that every file must exist immediately.

## 4. Application boundaries

### `backend/`

The Go service is the backend boundary.

- `cmd/codeceremony/`: executable entry point.
- `internal/auth/`: password hashing, signed session tokens, request principals.
- `internal/config/`: environment and runtime configuration.
- `internal/domain/`: core types, roles, permissions, and invariants. Split by concern into `models.go`, `account.go`, `roles.go`, `judging.go`, `rubric.go`, `submissions.go`, `eventconfig.go`, `hackathons.go`, `profiles.go`, `staff.go`, `activity.go`, `mail.go`, `community.go`, and `webhooks.go`.
- `internal/httpapi/`: HTTP server, middleware, request/response handling, and handlers. Split by surface into `server.go`, `handlers.go`, `account_handlers.go`, `admin_handlers.go`, `event_handlers.go`, `hackathons.go`, `community.go`, `community_api.go`, `assignments.go`, `rubrics.go`, `submissions.go`, `portability.go`, `openapi.go`, and `results.go`.
- `internal/seed/`: deterministic development seed data.
- `internal/store/`: repository interface implementation and local persistence boundary. `store.go` holds the core repository, `hackathons.go` holds hackathon hosting, profiles, invites, and participations, and `activity.go` holds the activity log, event staff, and the mail outbox, and `community.go` holds comments, reports, vote campaigns, ballots, webhooks, and deliveries.
- `internal/mailer/`: mail templates, SMTP and log senders, the delivery dispatcher with retry and backoff, the mail service that resolves announcement audiences, and `webhooks.go` for signed outbound deliveries.
- `migrations/`: ordered database migrations. `0001_initial.sql` is the base schema, `0002_account_management.sql` covers accounts, `0003_judging_expansion.sql` covers assignments, rubrics, and the submission lifecycle, and `0004_hackathon_hosting.sql` covers custom questions, profiles, team formation, rosters, and publications, and `0005_activity_and_mail.sql` covers the activity log, event staff, and mail, and `0006_community_and_webhooks.sql` covers comments, reports, campaigns, ballots, webhooks, and deliveries.
- `testdata/`: isolated test inputs and fixtures.

The API is the only application component allowed to access the database directly. The current first slice uses an in-memory store behind the store boundary; PostgreSQL persistence is a later P0/P1 task.

### `apps/web/`

The Next.js application is the browser-facing boundary.

- `src/app/`: Next.js application shell and route composition.
- `src/components/`: reusable presentation components.
- `src/hooks/`: client-side reusable hooks.
- `src/lib/`: browser-safe utilities and API client integration.
- `src/styles/`: styling entry points and design tokens.
- `src/assets/`: frontend-owned static assets.
- `public/`: files served as-is by the web application.
- `tests/`: frontend and browser test code.

The web application communicates with the Go API through the shared contract boundary. It does not embed database credentials or connect directly to PostgreSQL.

### `packages/contracts/`

This package is the shared boundary between frontend and backend.

- `schemas/`: source API and configuration schemas.
- `src/`: hand-written shared TypeScript types where appropriate.
- `generated/`: generated clients, types, or artifacts, if generation is introduced.

The contract package should remain independent of both application implementations so that transport types do not become duplicated business logic.

### `infra/`

This directory contains local and deployment infrastructure, not product code.

- `infra/compose/`: local Docker Compose definitions.
- `infra/postgres/`: database initialization and local seed support.
- `infra/scripts/`: repeatable environment and maintenance scripts.

The root `docker-compose.yml` should provide the simple entry point required by the event brief. Supporting compose files may be included for development overrides.

### `assets/brand/`

Brand work is separated from application code:

- `logo/`: primary CodeCeremony logo files.
- `marks/`: compact marks and favicon variants.
- `colors/`: approved color-token source files.
- `typography/`: approved font and typography assets.
- `README.md`: brand usage and asset handoff notes.

The logo and color theme are being prepared separately. No colors, logo files, or visual implementation are defined in this structure document.

## 5. Configuration and secrets

- `.env.example` lists variable names and safe placeholder values only.
- Real secrets must never be committed.
- Local defaults should be represented in development configuration, not embedded in source code.
- Configuration loading belongs in `backend/internal/config/` for the Go service and in the web build/runtime configuration boundary for Next.js.
- Docker Compose may reference local environment files but must not require a hosted account or hosted secret service.

## 6. Build and developer commands

The intended developer entry points are:

```bash
docker compose up
```

Additional command names should be standardized through the root `Makefile`, with small wrappers that work on the supported development platforms. The exact command names will be defined when the toolchain is selected.

## 7. Documentation boundaries

- `README.md`: setup and high-level orientation.
- `ARCHITECTURE.md`: system structure and technical decisions.
- `DATA-MODEL.md`: persistence and data-flow decisions.
- `JUDGING.md`: scoring and evaluation decisions, when that area is planned.
- `docs/BRAND.md`: logo, color, and typography direction.
- `docs/decisions/`: short architecture decision records.
- `docs/local-development.md`: developer environment instructions.
- `docs/operations/`: self-hosting and operational notes.

Feature requirements and behavior should be added to separate planning documents only after the team approves them. This file is the structural baseline.

## 8. Naming conventions

- Go package names: lowercase, short, and descriptive.
- Go exported identifiers: Go-standard capitalization.
- TypeScript files and folders: lowercase kebab-case where practical.
- React component names: PascalCase.
- API contract names: explicit and consistent across both applications.
- Database object names: documented in `DATA-MODEL.md` before migrations are finalized.
- Brand files: lowercase, descriptive filenames with format suffixes where appropriate.

## 9. Deferred decisions

The following are deliberately not decided in this document:

- product feature list;
- detailed screens and user journeys;
- API route names;
- authentication/session implementation;
- database tables and relationships;
- external services;
- feature-specific packages;
- final logo files;
- final color values;
- final third-party library selection.

Those decisions must be made deliberately in a later planning phase, while preserving the repository boundaries above.

## 10. Current handoff

- Product name: `CodeCeremony`.
- Current task: backend foundation implementation.
- Project code: started under `backend/`.
- Brand implementation: not started.
- Feature specification: defined in `FEATURES.md`.
- Next action: read `rules.md`, `FEATURES.md`, and this file, then continue the next authorized T1/T2 backend slice.
