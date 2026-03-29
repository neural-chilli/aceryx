# AGENTS.md — Aceryx

## Project Identity

Aceryx is a developer-first case orchestration engine for business workflows with human tasks, AI agent steps, and compliance-grade audit trails. Single Go binary, Postgres as sole dependency, Vue/PrimeVue frontend.

Reference: `docs/design/aceryx-design-v0.4.md` is the authoritative design document. All feature specs in `docs/specs/` elaborate on it.

---

## Architecture Rules

### Feature Isolation

Aceryx is organised as a set of **isolated feature packages**. Each feature owns its domain and exposes a public API. Features communicate through APIs, never by reaching into each other's internals.

```
internal/
  engine/        # DAG scheduler, step lifecycle, recovery
  cases/         # Case CRUD, schema validation, case types
  steps/         # Step execution (human, agent, integration, rule, timer)
  tasks/         # Human task assignment, claiming, completion, SLA
  connectors/    # Connector interface, registry, built-in connectors
  agents/        # Agent step execution, context assembly, LLM invocation
  vault/         # Document storage, metadata, content hashing
  rbac/          # Principals, roles, permissions, Authorize function
  audit/         # Event recording, hash chain, verification
  notify/        # Notification dispatch (email, WebSocket)
  search/        # Full-text search across cases
  expressions/   # JavaScript expression evaluation via goja
  workflows/     # Workflow AST, YAML serialisation, versioning
api/
  handlers/      # HTTP handlers grouped by domain
  middleware/    # Auth, RBAC, logging, recovery
  routes.go     # Route registration
frontend/
  src/
    views/       # Page-level components (inbox, case view, builder)
    components/  # Reusable UI components
    composables/ # Vue composables (API client, WebSocket, auth)
```

**The rule:** a package imports another package's exported API. It never imports internal types, unexported functions, or directly queries another package's database tables. If you need data from another domain, call its API function.

```go
// CORRECT — use the cases package API
caseData, err := cases.Get(ctx, caseID)

// WRONG — query another package's tables directly
row := db.QueryRow("SELECT data FROM cases WHERE id = $1", caseID)

// WRONG — import internal types
import "aceryx/internal/cases/repository"
```

Each package owns its database tables. Only that package reads from and writes to its tables. Cross-domain queries (e.g. joining cases with case_steps) are implemented as functions in the package that owns the primary table, or as a dedicated query package if the join is genuinely cross-domain.

### Dependency Direction

```
api/handlers → internal/*     (handlers call domain logic)
internal/engine → internal/*  (engine orchestrates domains)
internal/tasks → internal/cases  (tasks read case context)
internal/agents → internal/connectors  (agents call LLM via connector)
internal/agents → internal/cases  (agents resolve context from cases)
internal/* → internal/audit   (everything records events)
internal/* → internal/rbac    (everything checks permissions)
```

No circular dependencies. If two packages need each other, extract a shared interface or use dependency injection.

---

## Development Standards

### Go

- **Go 1.22+**. Use standard library where possible. Minimise external dependencies.
- **gofmt** on every file. No exceptions.
- **golangci-lint** with the project config (`.golangci.yml`). Zero warnings on merge.
- **Errors** are returned, not panicked. Use `fmt.Errorf("context: %w", err)` for wrapping. Define domain error types where callers need to distinguish error cases.
- **Context** (`context.Context`) is the first parameter of every function that does I/O, calls Postgres, or invokes external services.
- **Interfaces** are defined by the consumer, not the provider. Keep them small.
- **Tests** are in the same package (white-box) or `_test` package (black-box). Prefer black-box tests for API-level testing.

### Vue/TypeScript

- **Vue 3 Composition API** with `<script setup>` syntax.
- **PrimeVue** components for all standard UI elements. Do not build custom components when PrimeVue has one.
- **TypeScript** strict mode. No `any` types unless interfacing with an untyped external library.
- **Composables** for shared logic (API calls, WebSocket connection, auth state). No global mutable state outside Pinia stores.
- **VueFlow** for the workflow builder canvas. Custom node components per step type.

### SQL

- All schema changes via numbered migration files in `migrations/`.
- Migrations are forward-only in v1. No down migrations.
- Use `gen_random_uuid()` for primary keys.
- All timestamps are `TIMESTAMPTZ`.
- JSONB columns have a comment indicating their expected schema.
- Never use `SELECT *`. Name columns explicitly.

---

## Verification Protocol

### Before Every Change

1. Read the relevant feature spec in `docs/specs/`.
2. Read the system invariants (design doc Section 7.1).
3. Identify which package(s) the change touches.
4. Confirm the change does not import another package's internals.

### After Every Change

Run the full verification suite:

```bash
# Format
gofmt -w .

# Lint
golangci-lint run ./...

# Unit tests
go test ./internal/... -count=1 -race

# Integration tests (requires Postgres via testcontainers)
go test ./tests/integration/... -count=1 -tags=integration

# BDD scenarios
go test ./tests/bdd/... -count=1 -tags=bdd

# Frontend (if frontend files changed)
cd frontend && npm run lint && npm run type-check && npm run test:unit
```

**All checks must pass before reporting completion.** A change that breaks any check is not complete. Fix the failure, do not skip it.

### Guard Checks

After implementing a feature or fixing a bug, verify the following guards:

- [ ] `gofmt` produces no changes
- [ ] `golangci-lint` reports zero issues
- [ ] All existing unit tests pass
- [ ] All existing integration tests pass
- [ ] All existing BDD scenarios pass
- [ ] New code has tests (unit or integration as appropriate)
- [ ] No package imports another package's internal types
- [ ] No package directly queries another package's tables
- [ ] All database writes within a step transition occur in a single transaction
- [ ] RBAC check exists on every new API endpoint
- [ ] Audit event is recorded for every state-changing operation
- [ ] Every query on a tenant-scoped table includes `WHERE tenant_id = $tenant_id`
- [ ] Error cases return appropriate HTTP status codes (not 500 for expected errors)

**If any guard fails, fix it before proceeding to the next task.**

---

## Invariants to Protect

These invariants (from design doc Section 7.1) must never be violated by any change. If a change would violate an invariant, the change is wrong — not the invariant.

1. A step activates at most once per workflow execution.
2. A task completes at most once.
3. A workflow does not progress after cancellation.
4. Case version increments strictly monotonically on case data changes (not on internal engine operations).
5. Step state transitions are monotonic (forward only).
6. The audit trail is append-only.
7. Case data is always valid against its case type schema.
8. RBAC is enforced on every state-changing operation.

**Retry boundary:** retries are internal to the executor. The step remains in `active` state throughout. The engine never sets a step back to `ready` or `pending`. Do not implement retries by re-entering a step into the DAG.

**Non-deterministic boundary:** agent steps produce non-deterministic output. Do not build logic that assumes agent step reproducibility. The engine treats agent output as opaque and final.

When implementing engine logic, explicitly consider how each invariant is preserved. If you cannot explain how a change maintains all eight invariants, stop and ask.

---

## Transaction Boundaries

All state mutations for a step transition occur within a single Postgres transaction:

```go
tx, err := db.Begin(ctx)
// 1. Update case_steps.state
// 2. Write case_steps.result (if completing)
// 3. Insert case_events audit record
// 4. Update cases.updated_at (always)
// 5. If step output writes to case.data: update cases.data AND increment cases.version
err = tx.Commit(ctx)
```

**This is mandatory.** Never split these writes across separate transactions. Never commit a subset. If any write fails, the entire transaction rolls back and the step transition does not occur.

**Case version rule:** `cases.version` increments only when `cases.data` changes. Step state transitions, DAG evaluations, and task operations do not touch the case version. The version exists for API-level optimistic locking (PATCH with `If-Match`), not for internal engine bookkeeping.

---

## Concurrency Rules

- DAG evaluation for a case is serialised via `SELECT ... FOR UPDATE` on the case row.
- Task claiming uses `UPDATE ... WHERE assigned_to IS NULL RETURNING *`.
- Case data updates use optimistic locking: `UPDATE ... WHERE version = $expected`.
- Webhook deduplication uses `INSERT ... ON CONFLICT DO NOTHING RETURNING *`.
- Never hold a row lock across an external call (LLM invocation, integration call, network I/O).

---

## Testing Expectations

### Unit Tests

Every public function in every package has tests. Table-driven tests for functions with multiple cases. Test both success and error paths.

```go
func TestDAGEvaluation(t *testing.T) {
    tests := []struct {
        name     string
        steps    []StepState
        expected []StepTransition
    }{
        // ... cases covering join:all, join:any, skip propagation, etc.
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ...
        })
    }
}
```

### Integration Tests

Use `testcontainers-go` to spin up a real Postgres instance. Test full request→response cycles through the API handlers. Test transaction boundaries (verify that partial failures roll back completely).

### BDD Scenarios

Six reference scenarios in `tests/bdd/features/`. Each scenario is a Gherkin feature file with step definitions in Go (godog). These validate the generic model — if any scenario requires engine changes beyond configuration, that is a design gap.

### Frontend Tests

Vitest for component tests. Playwright for end-to-end tests against a full stack. Critical paths only — do not aim for exhaustive UI coverage.

---

## Working with Feature Specs

Each feature spec in `docs/specs/` contains:

- **Context:** what this feature does and why
- **Dependencies:** which other features must exist first
- **API surface:** endpoints, request/response shapes
- **Data model:** tables, columns, indexes
- **Behaviour:** detailed rules, edge cases, error handling
- **Tests:** specific test cases that must pass

When implementing a feature spec:

1. Read the entire spec before writing any code.
2. Create the database migration first.
3. Implement the domain logic in the internal package.
4. Add API handlers.
5. Write tests that match the spec's test cases.
6. Run all guards.
7. Report completion with a summary of what was built and all guard results.

**Do not add features, endpoints, or behaviours not described in the spec.** If you believe the spec is missing something, flag it — do not silently add it.

---

## Code Style

### Go Naming

- Package names are short, lowercase, single-word where possible: `cases`, `tasks`, `vault`, `rbac`.
- Exported types use clear, descriptive names: `CaseStep`, `WorkflowAST`, `ConnectorMeta`.
- Avoid stuttering: `cases.Case` not `cases.CaseModel`, `vault.Document` not `vault.VaultDocument`.
- Interface names describe behaviour: `Authorizer`, `ContextResolver`, `VaultStore`.

### File Organisation

- One primary type per file where practical.
- `repository.go` for database access within a package.
- `service.go` for business logic within a package.
- `models.go` for types and data structures within a package.
- `errors.go` for domain-specific error types.

### API Conventions

- REST endpoints follow: `GET /cases`, `POST /cases`, `GET /cases/{id}`, `PUT /cases/{id}`.
- Request bodies are JSON. Response bodies are JSON.
- Error responses use a consistent structure: `{ "error": "message", "code": "CASE_NOT_FOUND" }`.
- 400 for validation errors, 401 for unauthenticated, 403 for unauthorised, 404 for not found, 409 for conflicts (optimistic locking, duplicate idempotency key), 503 for backpressure.

---

## What Not To Do

- Do not introduce external dependencies without justification. No ORMs. No web frameworks (use `net/http`). No DI containers.
- Do not build abstractions before they are needed. If only one connector uses a pattern, it is not a pattern yet.
- Do not optimise before measuring. If it passes the performance targets in the design doc, it is fast enough.
- Do not add configuration options for things that have one correct answer.
- Do not handle errors by logging and continuing. Handle them or propagate them.
- Do not use `init()` functions. Explicit initialisation in `main()`.
- Do not write middleware that modifies request bodies.
- Do not store secrets in workflow YAML, case data, or audit events.
