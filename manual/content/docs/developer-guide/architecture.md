---
title: Architecture
weight: 1
---

Aceryx is a Go-based case orchestration engine built on principles of simplicity, pragmatism, and developer ergonomics. Every architectural decision prioritizes clarity and explicit control over magical abstractions.

## Design Philosophy

The core philosophy guiding Aceryx architecture:

- **Developer-first**: The system is designed for humans to understand and modify. No hidden magic, no implicit behavior.
- **Pragmatic**: We use proven technologies and patterns rather than cutting-edge frameworks. Go's standard library, PostgreSQL, raw SQL.
- **Simplicity as a feature**: Fewer layers, fewer dependencies, fewer ways to shoot yourself in the foot. A new developer should understand the codebase in hours, not weeks.

## Deployment Model

Aceryx ships as a **single binary**. The Go backend includes the Vue frontend embedded via `go:embed`, eliminating the need for a separate asset server or build process complexity.

```
aceryx binary
├── Backend (Go + net/http)
│   ├── Database (PostgreSQL)
│   └── Worker pool (step execution)
└── Frontend (Vue 3, embedded)
    ├── Assets (CSS, JS)
    └── Components
```

No Node.js server in production. No separate asset CDN. Deploy `./aceryx serve`, point to a PostgreSQL database, and you're running.

## Technology Stack

### Backend

- **Language**: Go 1.26+ (standard library, no heavy frameworks)
- **HTTP**: `net/http` (standard library). No gin, no echo, no chi.
- **Database**: PostgreSQL 17 with pgvector. **No ORM**—raw SQL with `pgx` for explicit control and performance.
- **Expression evaluation**: goja (sandboxed JavaScript runtime) for workflow conditions and transformations
- **Structured logging**: slog with JSON output for observability
- **Metrics**: Prometheus format

### Frontend

- **Framework**: Vue 3 with Composition API
- **Language**: TypeScript (strict mode, no `any`)
- **Component library**: PrimeVue
- **Workflow visualization**: VueFlow
- **Build**: Vite (development), embedded in binary (production)

## Package Architecture

The codebase is organized by domain, not by technical layer. Each package owns its domain completely.

```
internal/
├── cases/         — Case CRUD, lifecycle, schema validation
├── tasks/         — Task inbox, claiming, completion
├── agents/        — LLM executor, prompt templates, confidence
├── connectors/    — External system integrations (HTTP, Slack, Jira, etc.)
├── plugins/       — Plugin runtime, WASM execution, host functions
├── workflows/     — Workflow AST, versioning, serialization
├── vault/         — Document storage, content-addressed, cleanup
├── rbac/          — JWT, roles, permissions, caching
├── audit/         — Hash-chained event log, verification
├── engine/        — DAG scheduler, step dispatch, recovery
├── expressions/   — Sandboxed JavaScript (goja)
├── tenants/       — Multi-tenancy, branding
├── notify/        — Email and WebSocket
├── search/        — Full-text search (PostgreSQL)
├── reports/       — Analytics, custom queries
├── activity/      — Activity feed
├── backup/        — Backup/restore/verification
└── observability/ — Logging, metrics
```

### Feature Isolation

Each package:

1. **Owns its database tables** — exclusively. No other package queries those tables directly.
2. **Exposes a public API** — other packages interact only through exported functions and types.
3. **Never imports another package's internals** — cross-package queries are forbidden. If you need data from another domain, request it through that domain's public API.

This is enforced by `qp check:imports`, which verifies the import boundary at build time.

**Benefits**:
- Clear ownership and responsibility
- Easy to test in isolation
- Safe to refactor internals without affecting consumers
- Prevents accidental coupling and circular dependencies

## The DAG Engine

The heart of Aceryx is the **DAG (Directed Acyclic Graph) engine**, located in `internal/engine/`.

The engine:

1. **Evaluates workflow graphs** — traverses the DAG of steps defined in a workflow
2. **Respects dependencies** — a step activates only when all predecessor steps are complete
3. **Dispatches to workers** — steps execute in a configurable worker pool with parallelism
4. **Handles parallel execution** — independent steps run concurrently
5. **Implements retries** — failed steps retry with exponential backoff, within retry boundaries
6. **Monitors SLAs** — tracks step and case deadline compliance
7. **Handles recovery** — on restart, resumes incomplete executions from their exact state

Step lifecycle in the engine:

```
pending → active → completed
            ↓
          failed → active (retry)
```

A step is `pending` until all its predecessors complete. Once activated, it transitions to `active` and never re-enters that state (retries keep it `active`). After completion, it is `completed` and never changes.

## Core Invariants

The architecture enforces eight invariants—guarantees that must always hold:

1. **A step activates at most once per execution** — never enters the DAG twice for the same case execution
2. **A task completes at most once** — a manual step cannot be completed twice
3. **Workflow doesn't progress after cancellation** — once cancelled, no new steps activate
4. **Case version increments monotonically on data changes only** — optimistic locking for safe updates
5. **Step state transitions are forward-only** — never go backward from completed to pending
6. **Audit trail is append-only** — events never deleted or modified, hash-chained for tamper-evidence
7. **Case data always valid against schema** — mutations validate before commit
8. **RBAC enforced on every state change** — permissions checked before any data modification

These invariants are protected by transactional boundaries, careful state machine design, and code review discipline.

## Transaction Boundaries

**All step mutations happen in a single database transaction.**

When a step completes, the operation atomically:

1. Updates the step state (pending → completed)
2. Records audit events
3. Calculates the next activation set
4. Writes to the case data (if a data-modifying step)
5. Activates the next steps

This atomic transaction ensures that even if the system crashes mid-operation, the database is left in a consistent state. On restart, the engine resumes from exactly where it left off.

## Retry Boundary

Retries are **bounded within the step state machine**. A step that fails remains in `active` state and is re-attempted. It is never re-entered into the DAG. This simplifies recovery and prevents infinite loops.

Retry limits, backoff strategy, and SLA impact are configurable per step.

## Notifications

Notifications (email, Slack, webhooks) are **best-effort and non-blocking**. An operation never waits for a notification to be sent. If a notification fails, it is logged but doesn't roll back the operation. This keeps the critical path fast and prevents cascading failures.

## Expression Sandbox

Workflow conditions, form visibility rules, and data transformations use **sandboxed JavaScript** via goja. The sandbox is:

- **Safe**: No filesystem, network, or system access
- **Isolated**: Cannot leak to other expressions or the host runtime
- **Fast**: Pure Go, no spawning external processes
- **Limited**: Maximum expression size 4KB, timeout 100ms, pooled runtimes

Expressions have access to:
- **Case data**: `case.data.fieldName`
- **Step results**: `stepName.result`
- **Step outcomes**: `stepName.outcome`
- **Built-in functions**: `addDays(dateString, days)`, `lower(string)`, `upper(string)`, `contains(array, value)`, `lenOf(value)`

Example conditions:

```javascript
intake.outcome == "approved"
case.data.amount > 10000
review.result.risk_score < 0.5
```

## Multi-Tenancy

Aceryx is built for multi-tenancy from the ground up. Every query includes `tenant_id`, ensuring strict data isolation.

```sql
SELECT * FROM cases WHERE tenant_id = ? AND id = ?
```

A user always belongs to a single tenant. Cross-tenant access is impossible by design—if a query forgets the `tenant_id` filter, it becomes a bug that code review will catch.

Tenant-specific settings include:
- Branding (logo, colors)
- Terminology (what to call a "case", what to call a "task")
- Themes (dark/light modes, custom CSS)
- Roles and permissions

## Security

- **Authentication**: JWT tokens with configurable expiry
- **Authorization**: Role-based access control (RBAC) with fine-grained permissions
- **Audit**: Hash-chained event log, tamper-evident by cryptographic design
- **Data encryption**: At rest (if PostgreSQL is configured with encryption), in transit (TLS)
- **Secrets management**: Connector credentials stored in the vault, never logged or exposed
- **Rate limiting**: Per-user, per-endpoint (TBD based on observability metrics)

## Observability

Aceryx emits structured logs (slog, JSON format) and Prometheus metrics. No magic: every important operation logs why it happened and what it did.

Health and readiness endpoints support Kubernetes-style probes:
- `/health` — overall health
- `/healthz` — alive check
- `/readyz` — ready to serve traffic (database connected, migrations complete)
