<img src="logo-full.png" alt="Aceryx" height="96" align="left" hspace="16" />

## Aceryx

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D?logo=vuedotjs&logoColor=white)](https://vuejs.org)
[![Postgres](https://img.shields.io/badge/Postgres-17-4169E1?logo=postgresql&logoColor=white)](https://www.postgresql.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![CI](https://img.shields.io/github/actions/workflow/status/neural-chilli/aceryx/ci.yml?label=CI&logo=github)](https://github.com/neural-chilli/aceryx/actions)
[![Docs](https://img.shields.io/badge/Docs-Aceryx-blue?logo=gitbook&logoColor=white)](https://neural-chilli.github.io/aceryx/)

<br clear="left"/>

A developer-first case orchestration engine for business workflows with human tasks, AI agent steps, and compliance-grade audit trails.

Aceryx sits between lightweight automation tools (n8n, Zapier) that lack state and human interaction, and enterprise BPM platforms (Pega, Camunda) that cost six figures and take months to deploy. Single Go binary. Single Postgres dependency. Deploy in minutes.

## What It Does

Aceryx manages **cases** — loan applications, insurance claims, employee onboarding, vendor assessments — through workflows that combine human decisions, AI analysis, and system integrations.

A case moves through a workflow, accumulates data and documents, receives decisions from humans and agents, and is tracked and auditable throughout its lifecycle.

**Workflow engine** — DAG-based execution with parallel branches, conditional routing, join semantics, error handling with retries, and SLA monitoring. Workflows are defined as YAML or built visually.

**Human tasks** — tasks appear in a user's inbox, are claimed and completed with schema-driven forms, support SLA deadlines with automatic escalation, and route to the next step based on the chosen outcome.

**AI agent steps** — invoke an LLM with assembled context from the case, validate structured output, evaluate confidence, and escalate to a human when the agent isn't sure enough. Context includes case data, prior step results, documents, and vector-searched knowledge.

**Audit trail** — append-only, cryptographically chained event log of every case action. Who did what, when, with what data. Tamper-evident and exportable.

**Visual builder** — design workflows by dragging steps onto a canvas, connecting them, configuring integrations, writing expressions, and designing forms. Built on VueFlow.

**Connectors** — HTTP/REST, webhooks (inbound and outbound), email (HTML templates), Slack, Microsoft Teams, Google Chat, Jira. Self-describing interfaces so the builder renders config forms automatically.

## Quick Start

**Prerequisites:** Go 1.26+, Node 20+, Docker.

```bash
# Clone and bootstrap
git clone https://github.com/neural-chilli/aceryx.git
cd aceryx

# Start Postgres
docker compose up -d

# Build and migrate
make build
./aceryx migrate
./aceryx seed

# Run
./aceryx serve
```

Open `http://localhost:8080`. Log in with `admin@localhost` / `admin`.

To run the frontend dev server with hot reload:

```bash
cd frontend && npm install && npm run dev
```

## Development

Aceryx uses [qp](https://github.com/neural-chilli/qp) for task running:

```bash
qp db:fresh          # reset + migrate + seed
qp dev               # backend (air) + frontend (vite) in parallel
qp guards            # format + lint + test
qp test:all          # full test suite including integration and BDD
qp check:imports     # verify feature isolation (AGENTS.md rule)
```

Or use Make:

```bash
make build            # build Go binary
make test             # run unit tests
make lint             # format + lint
make guards           # all checks
make dev              # run backend + frontend
```

## Architecture

```
┌─────────────────────────────────────────────────┐
│                   Vue Frontend                  │
│  Inbox · Case View · Builder · Forms · Reports  │
├─────────────────────────────────────────────────┤
│                  Go HTTP API                    │
│  Cases · Tasks · Workflows · Auth · Vault · MI  │
├─────────────────────────────────────────────────┤
│               Execution Engine                  │
│  DAG Scheduler · Step Dispatch · SLA · Recovery │
├─────────────────────────────────────────────────┤
│                   Postgres                      │
│  Cases · Steps · Events · RBAC · PGVector       │
└─────────────────────────────────────────────────┘
```

**Single binary.** The Go backend serves the API and the built frontend. No microservices, no message queues, no Redis.

**Single dependency.** Postgres (with pgvector) is the only external requirement. It handles relational state, JSONB case data, full-text search, vector similarity, and real-time notifications via LISTEN/NOTIFY.

**Feature isolation.** The codebase is organised as isolated packages that import each other's APIs, never internals. See [AGENTS.md](AGENTS.md) for the full architecture rules.

## Project Structure

```
cmd/aceryx/          main entrypoint (serve, migrate, seed)
internal/
  engine/            DAG scheduler, step lifecycle, recovery
  cases/             case CRUD, schema validation, case types
  tasks/             human task assignment, claiming, SLA
  connectors/        connector interface, registry, built-in connectors
  agents/            agent step execution, context assembly, LLM
  vault/             document storage, content hashing
  rbac/              principals, roles, permissions, auth
  audit/             append-only event log, hash chain
  notify/            email + WebSocket notifications
  search/            full-text search
  expressions/       sandboxed JavaScript via goja
  workflows/         workflow AST, YAML, versioning
  tenants/           branding, theming, terminology
api/
  handlers/          HTTP handlers grouped by domain
  middleware/        auth, RBAC, logging
frontend/
  src/
    views/           inbox, case view, builder, login, reports
    components/      reusable UI components
    composables/     API client, auth, branding, theme, terminology
migrations/          numbered SQL migration files
tests/
  integration/       full-stack tests with testcontainers
  bdd/               Gherkin scenarios with godog
docs/
  design/            product design document
  specs/             feature specifications
```

## Key Concepts

**Case** — the unit of work. A loan application, an insurance claim, an employee onboarding. Each case has a type (schema), data (JSONB), attached documents, and a workflow execution.

**Workflow** — a directed acyclic graph of steps. Steps have types (human_task, agent, integration, rule, timer, notification), dependencies, join modes, conditions, and error policies. Workflows are versioned and immutable once published.

**Step** — a single unit of work within a workflow. Steps move through a lifecycle: pending → ready → active → completed/failed/skipped. Each step type has its own executor.

**Task** — a human task step that appears in a user's inbox. Tasks can be assigned to a specific user or a role (claimable by anyone with that role). Tasks have SLA deadlines with automatic escalation.

**Tenant** — an isolated workspace with its own branding, users, roles, case types, workflows, and data. Tenants never share data.

## Customisation

**Branding** — each tenant configures their company name, logo, favicon, and brand colours. The UI renders with the customer's identity, not Aceryx's.

**Themes** — four built-in themes (Light, Dark, High Contrast Light, High Contrast Dark) plus custom themes. Users choose their preferred theme from a dropdown.

**Terminology** — tenants override the default vocabulary: "case" → "application", "task" → "action", "inbox" → "work queue". Applied across the UI, emails, and API responses.

## Configuration

Aceryx is configured via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `ACERYX_PORT` | `8080` | HTTP server port |
| `ACERYX_DB_URL` | — | Postgres connection string |
| `ACERYX_VAULT_PATH` | `./data/vault` | Local document storage path |
| `ACERYX_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `ACERYX_JWT_SECRET` | — | HMAC secret for JWT signing |
| `ACERYX_LLM_ENDPOINT` | — | OpenAI-compatible API endpoint |
| `ACERYX_LLM_MODEL` | `gpt-4o` | Default LLM model |
| `ACERYX_LLM_API_KEY` | — | LLM API key |
| `ACERYX_MAX_CONCURRENT_STEPS` | `10` | Worker pool size |
| `ACERYX_SLA_INTERVAL` | `60s` | SLA check interval |

## Testing

```bash
# Unit tests
go test ./internal/... -count=1 -race

# Integration tests (spins up Postgres via testcontainers)
go test ./tests/integration/... -count=1 -tags=integration -race

# BDD scenarios
go test ./tests/bdd/... -count=1 -tags=bdd

# Frontend unit tests
cd frontend && npm run test:unit

# Frontend e2e tests
cd frontend && npx playwright test
```

## Deployment

**Docker:**

```bash
docker build -t aceryx:latest .
docker run -e ACERYX_DB_URL=postgres://... -p 8080:8080 aceryx:latest
```

**Binary:**

```bash
./aceryx serve
```

Aceryx runs as a single process. Place it behind a reverse proxy (nginx, Caddy) for TLS termination. Postgres can be any managed service (RDS, Cloud SQL, Supabase) or self-hosted — the only requirement is the pgvector extension.

## Licence

Community Edition is [MIT licensed](LICENSE). See [LICENSING.md](LICENSING.md) for the open core model.

## Contributing

Aceryx is built and maintained by [Neural Chilli](https://github.com/neural-chilli). We welcome contributions — please open an issue before starting work on a major change.

[//]: # (## Links)

[//]: # ()
[//]: # (- [Design Document]&#40;docs/design/aceryx-design-v0.4.md&#41; — product design and architecture)

[//]: # (- [Feature Specifications]&#40;docs/specs/&#41; — detailed specs for every capability)

[//]: # (- [AGENTS.md]&#40;AGENTS.md&#41; — architecture rules and development constitution)