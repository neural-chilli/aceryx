---
title: Project Structure
weight: 2
---

The Aceryx repository is organized by domain, with a clear separation between the CLI, backend services, frontend, and supporting infrastructure.

## Directory Tree

```
aceryx/
├── cmd/
│   └── aceryx/
│       ├── main.go
│       ├── commands/
│       │   ├── serve.go       — Start the server
│       │   ├── migrate.go      — Run database migrations
│       │   ├── seed.go         — Populate test data
│       │   ├── backup.go       — Create database backup
│       │   ├── restore.go      — Restore from backup
│       │   └── version.go      — Print version info
│       └── config.go           — Configuration loading
│
├── internal/
│   ├── cases/
│   │   ├── case.go            — Case domain model
│   │   ├── store.go           — Case CRUD operations
│   │   ├── service.go         — Case business logic
│   │   ├── schema.go          — JSON schema validation
│   │   ├── reports.go         — Case reporting (loaders, transformations)
│   │   └── test_helpers.go
│   │
│   ├── tasks/
│   │   ├── task.go            — Task domain model
│   │   ├── store.go           — Task inbox, claiming, completion
│   │   ├── service.go         — Task business logic
│   │   ├── sla.go             — SLA calculation and monitoring
│   │   ├── escalation.go       — Escalation rules and triggers
│   │   └── draft.go           — Draft saving and retrieval
│   │
│   ├── agents/
│   │   ├── executor.go        — LLM step executor
│   │   ├── context.go         — Context assembly from case data
│   │   ├── templates.go       — Prompt template storage and versioning
│   │   ├── validation.go       — Prompt validation
│   │   ├── confidence.go       — Confidence scoring logic
│   │   └── escalation.go       — LLM error handling and escalation
│   │
│   ├── connectors/
│   │   ├── connector.go        — Connector interface
│   │   ├── registry.go         — Connector registration
│   │   ├── http.go            — HTTP connector
│   │   ├── email.go           — Email connector
│   │   ├── webhooks.go        — Webhook receiver
│   │   ├── slack.go           — Slack connector
│   │   ├── teams.go           — Microsoft Teams connector
│   │   ├── googlechat.go      — Google Chat connector
│   │   ├── jira.go            — Jira connector
│   │   ├── docgen.go          — Document generation connector
│   │   ├── config.go          — Connector credentials and config
│   │   └── actions.go         — Connector action definitions
│   │
│   ├── plugins/
│   │   ├── runtime.go         — WASM plugin runtime
│   │   ├── manifest.go        — manifest.yaml parsing
│   │   ├── host_functions.go  — HTTP, connectors, secrets, logging, audit, vault access
│   │   ├── executor.go        — Plugin step executor
│   │   ├── store.go           — Plugin storage and versioning
│   │   └── test_helpers.go
│   │
│   ├── workflows/
│   │   ├── workflow.go        — Workflow domain model (AST)
│   │   ├── parser.go          — YAML to AST conversion
│   │   ├── serializer.go      — AST to YAML conversion
│   │   ├── validator.go       — Workflow validation
│   │   ├── store.go           — Workflow storage and versioning
│   │   ├── version.go         — Workflow version management (draft/published/withdrawn)
│   │   ├── dag.go             — DAG construction and verification
│   │   └── executor.go        — Workflow execution initialization
│   │
│   ├── engine/
│   │   ├── engine.go          — DAG scheduler main loop
│   │   ├── step.go            — Step activation and state management
│   │   ├── dispatch.go        — Worker pool dispatch
│   │   ├── recovery.go        — Crash recovery and resumption
│   │   ├── retries.go         — Retry logic with backoff
│   │   ├── sla.go             — SLA monitoring and alerts
│   │   ├── cancel.go          — Cancellation handling
│   │   ├── metrics.go         — Engine metrics collection
│   │   └── test_helpers.go
│   │
│   ├── vault/
│   │   ├── vault.go           — Document storage interface
│   │   ├── store.go           — PostgreSQL-backed storage
│   │   ├── content.go         — Content-hash addressing
│   │   ├── signed_urls.go     — Signed URL generation
│   │   ├── cleanup.go         — Orphaned document cleanup
│   │   ├── erasure.go         — Data erasure on user deletion
│   │   └── test_helpers.go
│   │
│   ├── rbac/
│   │   ├── jwt.go             — JWT token creation and validation
│   │   ├── auth.go            — Authentication flow
│   │   ├── roles.go           — Role storage and querying
│   │   ├── permissions.go     — Permission checking
│   │   ├── cache.go           — Permission cache (short-lived)
│   │   ├── middleware.go      — HTTP middleware for auth
│   │   └── test_helpers.go
│   │
│   ├── audit/
│   │   ├── event.go           — Audit event domain model
│   │   ├── store.go           — Event storage with hash-chaining
│   │   ├── chain.go           — Hash-chain verification
│   │   ├── export.go          — Event export (CSV, JSON)
│   │   ├── middleware.go      — Automatic event recording
│   │   └── test_helpers.go
│   │
│   ├── tenants/
│   │   ├── tenant.go          — Tenant domain model
│   │   ├── store.go           — Tenant CRUD
│   │   ├── branding.go        — Branding (logo, colors)
│   │   ├── terminology.go     — Custom terminology
│   │   ├── themes.go          — Theme storage (dark/light, custom CSS)
│   │   └── context.go         — Tenant context extraction from requests
│   │
│   ├── notify/
│   │   ├── notify.go          — Notification interface
│   │   ├── email.go           — Email sender (async, non-blocking)
│   │   ├── websocket.go       — WebSocket broadcaster
│   │   └── queue.go           — Message queue (optional persistence)
│   │
│   ├── search/
│   │   ├── search.go          — Full-text search interface
│   │   ├── postgres.go        — PostgreSQL tsvector implementation
│   │   ├── indexing.go        — Index building and updates
│   │   └── query.go           — Search query builder
│   │
│   ├── expressions/
│   │   ├── evaluator.go       — goja JavaScript sandbox
│   │   ├── context.go         — Expression context (case data, results)
│   │   ├── validation.go       — Expression validation
│   │   └── cache.go           — Compiled expression caching
│   │
│   ├── reports/
│   │   ├── report.go          — Report domain model
│   │   ├── store.go           — Report storage
│   │   ├── executor.go        — Report query execution
│   │   ├── llm.go             — LLM-powered natural language queries
│   │   ├── builtin.go         — Built-in reports (summary, ageing, SLA compliance, etc.)
│   │   ├── materialized.go    — Materialized view refresh
│   │   └── export.go          — Report export (CSV, JSON, PDF)
│   │
│   ├── activity/
│   │   ├── feed.go            — Activity feed domain model
│   │   ├── store.go           — Activity storage
│   │   ├── listener.go        — Audit event subscription
│   │   └── aggregation.go     — Activity aggregation (optional)
│   │
│   ├── backup/
│   │   ├── backup.go          — Backup interface
│   │   ├── dump.go            — pg_dump wrapper
│   │   ├── archive.go         — Vault archive creation
│   │   ├── restore.go         — Restore from backup
│   │   └── verify.go          — Backup verification
│   │
│   └── observability/
│       ├── logging.go         — slog initialization (JSON)
│       ├── metrics.go         — Prometheus metrics
│       └── tracing.go         — Optional distributed tracing
│
├── api/
│   ├── handlers/
│   │   ├── auth.go            — /auth/* endpoints
│   │   ├── tenants.go         — /tenant/* endpoints
│   │   ├── admin.go           — /admin/* endpoints (users, roles)
│   │   ├── case_types.go      — /case-types/* endpoints
│   │   ├── cases.go           — /cases/* endpoints
│   │   ├── documents.go       — /cases/{id}/documents/* endpoints
│   │   ├── tasks.go           — /tasks/* endpoints
│   │   ├── connectors.go      — /connectors/* endpoints
│   │   ├── templates.go       — /prompt-templates/* endpoints
│   │   ├── reports.go         — /reports/* endpoints
│   │   ├── webhooks.go        — /webhooks/* endpoints
│   │   ├── activity.go        — /activity/* endpoints
│   │   ├── health.go          — /health, /healthz, /readyz
│   │   ├── metrics.go         — /metrics (Prometheus)
│   │   ├── websocket.go       — WebSocket upgrade handler
│   │   └── middleware.go      — Shared middleware (logging, error handling)
│   │
│   ├── routes.go              — Route registration and middleware setup
│   ├── server.go              — HTTP server initialization
│   └── errors.go              — Standardized error responses
│
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   │   ├── CaseForm.vue
│   │   │   ├── TaskInbox.vue
│   │   │   ├── WorkflowBuilder.vue
│   │   │   ├── CaseDashboard.vue
│   │   │   ├── DocumentUpload.vue
│   │   │   └── ...
│   │   ├── views/
│   │   │   ├── CaseList.vue
│   │   │   ├── CaseDetail.vue
│   │   │   ├── TaskList.vue
│   │   │   ├── WorkflowList.vue
│   │   │   ├── Reports.vue
│   │   │   └── ...
│   │   ├── stores/
│   │   │   ├── auth.ts
│   │   │   ├── cases.ts
│   │   │   ├── tasks.ts
│   │   │   ├── workflows.ts
│   │   │   └── ui.ts
│   │   ├── api/
│   │   │   ├── client.ts       — Axios instance with auth
│   │   │   ├── cases.ts        — Case API client
│   │   │   ├── tasks.ts        — Task API client
│   │   │   └── ...
│   │   ├── utils/
│   │   │   ├── expressions.ts  — Expression evaluation
│   │   │   ├── validators.ts   — Form validation
│   │   │   └── ...
│   │   ├── App.vue
│   │   └── main.ts
│   │
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── eslint.config.js
│   └── playwright.config.ts
│
├── migrations/
│   ├── 001_initial_schema.sql
│   ├── 002_add_audit_tables.sql
│   ├── 003_add_vault_tables.sql
│   ├── 004_add_workflow_tables.sql
│   ├── 005_add_tenant_tables.sql
│   ├── 006_add_materialized_views.sql
│   └── ...
│
├── deploy/
│   ├── docker/
│   │   ├── Dockerfile          — Multi-stage build
│   │   └── docker-compose.yml  — Local development stack
│   ├── kubernetes/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   ├── configmap.yaml
│   │   ├── secret.yaml
│   │   └── ingress.yaml
│   └── terraform/
│       ├── main.tf
│       ├── postgres.tf
│       └── variables.tf
│
├── tests/
│   ├── integration/
│   │   ├── cases_test.go
│   │   ├── tasks_test.go
│   │   ├── workflows_test.go
│   │   ├── engine_test.go
│   │   ├── rbac_test.go
│   │   └── ...
│   ├── bdd/
│   │   ├── features/
│   │   │   ├── cases.feature
│   │   │   ├── workflows.feature
│   │   │   └── ...
│   │   └── steps/
│   │       ├── cases_steps.go
│   │       └── ...
│   └── setup.go               — Test database setup, fixtures
│
├── testdata/
│   ├── workflows/
│   │   ├── simple_approval.yaml
│   │   ├── complex_parallel.yaml
│   │   └── ...
│   ├── case_types/
│   │   ├── customer_complaint.json
│   │   └── ...
│   └── fixtures.sql           — Test data
│
├── docs/
│   ├── architecture/           — Architecture decision records (ADRs)
│   ├── api/                    — API documentation (OpenAPI)
│   ├── workflows/              — Workflow examples
│   └── operations/             — Operational runbooks
│
├── .github/
│   ├── workflows/
│   │   ├── test.yml           — CI tests
│   │   ├── lint.yml           — Linting checks
│   │   └── release.yml        — Release automation
│   └── CODEOWNERS
│
├── Makefile                    — Build targets
├── go.mod                      — Go dependencies
├── go.sum
├── package.json               — Frontend dependencies
├── package-lock.json
├── docker-compose.yml         — Local development environment
├── .env.example               — Configuration template
├── .gitignore
├── LICENSE                    — MIT
└── README.md
```

## Key Directories

### `cmd/aceryx/`

The CLI entry point. The `main.go` sets up command handling and delegates to subcommands:
- `serve` — starts the HTTP server
- `migrate` — runs pending database migrations
- `seed` — populates the database with test data
- `backup` — creates a full backup (database + vault)
- `restore` — restores from a backup
- `version` — prints build information

### `internal/engine/`

The DAG scheduler. This is the most complex and critical part of the system. It:
- Evaluates workflow graphs and maintains step state
- Dispatches steps to a configurable worker pool
- Handles retries, SLA monitoring, and cancellation
- Implements crash recovery by resuming incomplete executions

### `internal/cases/`, `internal/tasks/`, `internal/agents/`, etc.

Each domain package owns its features completely. For example:

**`internal/cases/`**:
- Define the case domain model (case, version, data)
- Implement case CRUD operations (`Create`, `Get`, `Update`, `Close`, `Cancel`)
- Validate case data against the case type's JSON schema
- Provide case reports and loaders

**`internal/tasks/`**:
- Task inbox (list tasks for user/role)
- Claiming (mark task as being worked on)
- Completion (submit task outcome)
- SLA calculation and escalation
- Draft saving (temporary work-in-progress data)

**`internal/agents/`**:
- Execute LLM steps in a case workflow
- Assemble context from case data
- Manage prompt templates and versioning
- Calculate confidence scores
- Escalate on errors or low confidence

### `api/handlers/`

HTTP request handlers, organized by feature. Each handler imports the necessary domain packages and calls their public APIs. Handlers should be thin—business logic lives in the domain packages.

### `frontend/`

Vue 3 + TypeScript application. The build output is embedded into the Go binary via `go:embed`, so no separate asset server is needed.

### `migrations/`

SQL migration files numbered in order (001, 002, 003, etc.). These are applied sequentially on startup or via `qp db:fresh` to reset the database.

## Package Imports

Imports follow strict rules to maintain package isolation:

- `internal/cases/` may import `internal/vault`, `internal/expressions`, `internal/audit`, etc., **but never imports their internals**.
- `internal/cases/` exports a public API that other packages use.
- Cross-domain queries are forbidden. If you need to join case data with task data, write that join in one package and expose it as a public function.

This is enforced by `qp check:imports`, which verifies the boundary at build time.

## Development Workflow

```bash
# Database management
qp db:up              # Start Postgres in Docker
qp db:reset           # Drop and recreate database
qp db:migrate         # Run pending migrations
qp db:seed            # Populate test data
qp db:fresh           # Full reset (drop, migrate, seed)
qp db:psql            # Open psql shell

# Build & Serve
qp build              # Full production build (frontend + backend)
qp build:backend      # Build Go binary only
qp build:frontend     # Build Vue frontend only
qp serve              # Run backend server (requires build)

# Development
qp dev                # Run backend + frontend dev servers with hot reload
qp dev:backend        # Backend with hot reload (requires air)
qp dev:frontend       # Vite dev server on :5173

# Testing
qp test               # Run Go unit tests
qp test:integration   # Run integration tests (testcontainers)
qp test:bdd           # Run BDD scenarios
qp test:frontend      # Run Vitest unit tests
qp test:e2e           # Run Playwright e2e tests
qp test:all           # Run all tests

# Code Quality
qp fmt                # Format Go code
qp lint               # Lint Go code
qp lint:frontend      # Lint and type-check frontend
qp guards             # Run fmt + lint + test (must pass before merge)
qp guards:full        # Full suite (fmt + lint + test:all + vet + build + arch-check + vulncheck)

# Verification
qp check:imports      # Verify no cross-package internal imports
qp vet                # Run go vet
qp vulncheck          # Run govulncheck

# Utilities
qp backup             # Create database backup
qp clean              # Remove build artifacts
qp loc                # Count lines of code
qp generate           # Run code generators
qp deps               # Install Go dependencies
qp deps:frontend      # Install frontend dependencies
qp deps:tools         # Install development tools

# Docker
qp docker:build       # Build Docker image
qp docker:run         # Run full stack in Docker
qp docker:stop        # Stop all Docker services
```
