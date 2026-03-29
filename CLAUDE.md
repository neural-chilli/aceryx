# CLAUDE.md

## Project

Aceryx — a developer-first case orchestration engine. Single Go binary + Postgres + Vue/PrimeVue frontend.

## Primary Instructions

Read and follow `AGENTS.md` — it is the constitution for this project. Everything in it applies to you.

Read `docs/design/aceryx-design-v0.4.md` for product context and architecture decisions.

## Working with Feature Specs

Before implementing anything, read the relevant spec in `docs/specs/`. Specs are numbered and ordered by dependency. Do not start a spec until its dependencies are implemented and passing.

Current spec list:
- `001-postgres-schema.md` → `002-execution-engine.md` → `003-case-management-api.md` → `008-rbac.md` → `013-branding-theming-terminology.md` → `004-human-tasks.md` → `005-connector-framework.md` → `006-agent-steps.md` → `007-vault.md` → `009-schema-driven-forms.md` → `010-visual-builder.md` → `011-audit-trail.md` → `012-notifications.md`

## Key Rules

- **Feature isolation.** Packages import each other's APIs, never internals. Never query another package's tables directly.
- **Single transaction.** All state mutations for a step transition (case_steps update + case_events insert + cases update) occur in one Postgres transaction. No exceptions.
- **Retries are internal to executors.** The step stays `active`. The engine never re-enters a step into the DAG.
- **Agent output is non-deterministic.** Never build logic that assumes agent step reproducibility.
- **Case version increments only on `cases.data` or `cases.status` changes.** Not on DAG evaluation or step transitions.
- **Every query on a tenant-scoped table must include `WHERE tenant_id = $tenant_id`.**
- **Notifications are best-effort.** They must never block or fail the operation that triggered them.

## Guards

Run after every change:

```bash
gofmt -w .
golangci-lint run ./...
go test ./internal/... -count=1 -race
```

All must pass before reporting completion. If frontend files changed, also run:

```bash
cd frontend && npm run lint && npm run type-check && npm run test:unit
```

## Stack

- **Backend:** Go 1.26, net/http (no framework), pgx for Postgres, goja for expressions
- **Frontend:** Vue 3 + TypeScript, PrimeVue (Aura theme), VueFlow, Pinia, Vue Router
- **Database:** Postgres 17 with pgvector. Docker Compose for local dev.
- **Testing:** Go standard testing + testcontainers-go, godog for BDD, Vitest + Playwright for frontend

## Style

- `gofmt` is law. No exceptions.
- No ORMs. No web frameworks. No DI containers.
- Errors are returned, not panicked. Wrap with `fmt.Errorf("context: %w", err)`.
- Interfaces are defined by the consumer, not the provider.
- Vue uses `<script setup>` with Composition API. TypeScript strict mode. No `any`.
- PrimeVue components for all standard UI. Don't build custom when PrimeVue has one.

## Dev Workflow

```bash
qp db:fresh        # reset + migrate + seed
qp dev             # backend (air) + frontend (vite) in parallel
qp guards          # format + lint + test
qp test:all        # full test suite
qp check:imports   # verify feature isolation
```
