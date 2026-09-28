# CodeCeremony

> **Work in progress.** This repository is being built during the DOGFOOD 2026 hackathon. The project is not complete and no tier should be claimed from this README alone.

CodeCeremony is a self-hostable hackathon submission and judging platform: a modern, open-source alternative to the platforms organizers currently run.

## Current status

The project is in backend foundation development.

Implemented or being implemented in the first slice:

- Go service under `backend/`;
- environment-based configuration;
- local development seed data;
- authentication and signed session tokens;
- role model and backend authorization boundaries;
- public event and project gallery endpoints;
- submission deadline enforcement;
- judge own-score access with peer-score isolation;
- organizer progress, results, and CSV export endpoints;
- weighted raw scoring, within-judge normalization, and deterministic ranking;
- initial PostgreSQL migration schema;
- health and readiness endpoints;
- focused Go tests.

The full product surface is documented in:

- [`rules.md`](rules.md) — event rules and constraints;
- [`STRUCTURE.md`](STRUCTURE.md) — repository and technology structure;
- [`design.md`](design.md) — light-theme design system and component contract;
- [`FEATURES.md`](FEATURES.md) — comprehensive feature specification.

## Technology

- Backend: Go;
- Frontend: TypeScript and Next.js in SPA mode;
- Database target: PostgreSQL with migrations;
- Local runtime: Docker Compose;
- Contracts: OpenAPI and shared TypeScript types.

## Backend development

Run the Go service from the repository root:

```bash
cd backend
go run ./cmd/codeceremony
```

The default local address is `:8080`. Configuration is read from environment variables. Copy the example file when adding local settings:

```bash
cp .env.example .env
```

Run backend tests:

```bash
cd backend
go test ./...
```

The service is not yet a complete organizer portal. The current store is an in-memory development implementation behind a persistence boundary; PostgreSQL wiring and the full UI remain unfinished. This is a foundation for the T1/T2 path, not a finished submission.

## Planned startup

The intended event entry point will be:

```bash
docker compose up
```

The Compose stack must eventually provide the API, frontend, local database, migrations, and deterministic fixture seeding without cloud accounts or hosted services.

## Important constraints

- Keep the repository public and OSI-licensed.
- Do not commit secrets, dependency directories, build outputs, local databases, or organizer-provided fixture artifacts.
- Keep role enforcement in the backend.
- Keep tier claims honest.
- Follow `design.md` for every UI surface.
- Do not claim T1, T2, T3, or T4 until the corresponding behavior is implemented and verified.

## License

A license will be added before submission. The intended default is MIT or Apache-2.0.
