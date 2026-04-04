---
title: Contributing
weight: 7
---

Aceryx is an open-source project licensed under the **MIT License** (Community Edition). We welcome contributions from the community. This guide explains how to contribute code, documentation, and feedback.

## License

By contributing to Aceryx, you agree to license your contributions under the MIT License. See `LICENSE` file for details.

## Code of Conduct

Be respectful, inclusive, and constructive. We do not tolerate harassment or discrimination.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally
3. **Create a branch** for your feature (`git checkout -b feature/my-feature`)
4. **Make changes** and **test thoroughly**
5. **Commit with clear messages** (see Commit Guidelines below)
6. **Push to your fork** and **open a pull request**

## Architecture & Design

Before making significant changes, understand the core principles:

### Package Boundaries

{{< callout type="info" >}}
This is **the most important rule** for Aceryx development.
{{< /callout >}}

Each package owns its domain completely:

1. **Own tables exclusively** — No other package queries your tables directly
2. **Export public API** — Other packages use your public functions
3. **Never import internals** — Import only types and functions from the `package.go` file
4. **No cross-package queries** — If you need another domain's data, use their public API

**Forbidden**:
```go
// BAD: Importing internal implementation details
import "aceryx/internal/cases/store"
func (t *Task) getCase() {
  return store.GetCase(t.CaseID)  // Direct database access
}

// BAD: Cross-package SQL queries
func GetTasksForCase(caseID string) {
  return db.Query("SELECT * FROM tasks WHERE case_id = ?")
}
```

**Encouraged**:
```go
// GOOD: Using public API
import "aceryx/internal/cases"
func (t *Task) getCase(ctx context.Context) {
  return cases.Get(ctx, t.CaseID)  // Through public API
}
```

### Verify Boundaries

Run this before committing:

```bash
qp check:imports
```

This verifies:
- No package imports another's internals
- No circular dependencies
- All cross-package calls use public APIs

Failing this check blocks merging.

## Development Workflow

### 1. Code Changes

Make changes following the code style guide below.

### 2. Unit Testing

Write unit tests for business logic. Test files live alongside code:

```go
// internal/cases/service.go
func (s *Service) ValidateData(data map[string]interface{}) error {
  // Business logic
}

// internal/cases/service_test.go
func TestValidateData_ValidInput(t *testing.T) {
  s := cases.NewService(nil)  // Mock DB
  err := s.ValidateData(map[string]interface{}{
    "name": "Test",
  })
  assert.NoError(t, err)
}

func TestValidateData_InvalidInput(t *testing.T) {
  s := cases.NewService(nil)
  err := s.ValidateData(map[string]interface{}{
    "name": 123,  // Wrong type
  })
  assert.Error(t, err)
}
```

Run tests:
```bash
go test ./internal/... -v -race
```

### 3. Integration Testing

For cross-package interactions, write integration tests in `tests/integration/`:

```go
// tests/integration/cases_test.go
func TestCaseWithWorkflow(t *testing.T) {
  db := setupTestDB(t)
  caseService := cases.NewService(db)
  engineService := engine.NewEngine(db)

  // Create case
  c, _ := caseService.Create(ctx, &cases.CreateInput{...})

  // Simulate workflow execution
  engineService.ProcessCase(ctx, c.ID)

  // Verify final state
  updated, _ := caseService.Get(ctx, c.ID)
  assert.Equal(t, "completed", updated.Status)
}
```

Run integration tests:
```bash
go test ./tests/integration/... -v -race -tags=integration
```

### 4. Code Quality Checks

Before committing, run:

```bash
qp guards
```

This sequentially runs:
1. **Format**: `gofmt -w` all Go files
2. **Lint**: `golangci-lint run` (static analysis)
3. **Test**: `go test ./internal/... -race` (unit tests)

All three must pass. Fix any issues and rerun.

### 5. Commit & Push

Commit with clear, descriptive messages (see Commit Guidelines). Push to your fork.

### 6. Pull Request

Open a PR with:
- **Title**: Short description (under 70 chars)
- **Description**: Why this change? What problem does it solve?
- **Testing**: How did you test? (link to test files)
- **Related issues**: Reference any issues this addresses

Example:

```markdown
## Description

Add support for case data validation against JSON schema.

## Why

Currently, case data is stored without validation, allowing invalid data
to corrupt workflows. This adds upstream validation in the cases service.

## Changes

- Add `ValidateData()` method to cases.Service
- Add JSON schema support in case_types
- Add integration test for validation

## Testing

- Unit tests: `internal/cases/service_test.go`
- Integration tests: `tests/integration/cases_test.go`

Closes #123
```

## Code Style Guide

### Go

Follow [Effective Go](https://golang.org/doc/effective_go) and use `gofmt`:

```bash
gofmt -w ./internal/...
```

**Naming conventions**:
- Package names: lowercase, single word (`cases`, `tasks`, `audit`)
- Functions: PascalCase (`GetCase`, `CreateTask`, `ValidateData`)
- Constants: UPPER_SNAKE_CASE (`MAX_RETRIES`, `DEFAULT_TIMEOUT`)
- Interfaces: Verb + "er" naming (`Reader`, `Writer`, `Storer`)
- Error variables: `ErrXxx` prefix (`ErrNotFound`, `ErrInvalidData`)

**Examples**:

```go
// Good: Clear, concise
type Store interface {
  Create(ctx context.Context, case *Case) error
  Get(ctx context.Context, id string) (*Case, error)
  Update(ctx context.Context, case *Case) error
}

// Good: Error naming
var ErrCaseNotFound = errors.New("case not found")

// Good: Constant naming
const DefaultPageSize = 25

// Bad: Ambiguous naming
type S interface {
  C(c *Case) error
}

// Bad: Package prefix in name (redundant)
func cases.GetCase() {}

// Bad: Inconsistent error naming
var NotFound = errors.New("not found")
```

### No ORM

Use raw SQL with `pgx`. This gives explicit control and better performance:

```go
// Good: Raw SQL with pgx
rows, _ := db.Query(ctx, "SELECT id, name FROM cases WHERE tenant_id = ? AND status = ?", tenantID, "active")
defer rows.Close()
for rows.Next() {
  var id, name string
  rows.Scan(&id, &name)
  // ...
}

// Bad: Using an ORM
db.Where("tenant_id = ?", tenantID).Where("status = ?", "active").Find(&cases)
```

### Structured Logging

Use `slog` (Go 1.21+ standard library):

```go
import "log/slog"

// Good: Structured logging with context
logger := slog.With(
  slog.String("case_id", caseID),
  slog.String("step_name", stepName),
)
logger.Info("Step activated", slog.String("status", "active"))

// Output (JSON format):
// {"time":"2026-04-04T10:00:00Z","level":"INFO","msg":"Step activated","case_id":"uuid","step_name":"intake","status":"active"}

// Bad: String concatenation
log.Printf("Step %s activated for case %s", stepName, caseID)
```

### Error Handling

Always handle errors explicitly:

```go
// Good: Explicit error handling
case, err := s.Get(ctx, caseID)
if err != nil {
  if errors.Is(err, ErrNotFound) {
    return nil, fmt.Errorf("case not found: %w", err)
  }
  return nil, fmt.Errorf("failed to get case: %w", err)
}

// Bad: Ignoring errors
case, _ := s.Get(ctx, caseID)

// Bad: Generic error message
if err != nil {
  return errors.New("error")
}
```

### Database Transactions

Use transactions for atomic operations:

```go
// Good: Explicit transaction
tx, _ := db.Begin(ctx)
defer tx.Rollback(ctx)

tx.Exec(ctx, "UPDATE cases SET status = ?", "closed")
tx.Exec(ctx, "INSERT INTO case_events (case_id, action) VALUES (?, ?)", caseID, "closed")

tx.Commit(ctx)

// Bad: No transaction
db.Exec(ctx, "UPDATE cases SET status = ?", "closed")
db.Exec(ctx, "INSERT INTO case_events ...")  // Could fail, leaving case in inconsistent state
```

### Frontend (Vue 3 + TypeScript)

Use Vue 3 Composition API with `<script setup>`:

```vue
<script setup lang="ts">
import { ref, computed } from 'vue'
import { useCases } from '@/stores/cases'

interface Case {
  id: string
  status: 'active' | 'closed'
}

const casesStore = useCases()
const selectedId = ref<string | null>(null)

const selectedCase = computed(() => {
  if (!selectedId.value) return null
  return casesStore.cases.find(c => c.id === selectedId.value)
})

const closeCase = async () => {
  if (!selectedCase.value) return
  await casesStore.closeCase(selectedCase.value.id)
}
</script>

<template>
  <div>
    <select v-model="selectedId">
      <option v-for="case in casesStore.cases" :key="case.id" :value="case.id">
        {{ case.id }} - {{ case.status }}
      </option>
    </select>
    <button @click="closeCase" :disabled="!selectedCase">Close Case</button>
  </div>
</template>
```

**Rules**:
- Use `<script setup>` syntax (simpler, more readable)
- Use TypeScript with strict mode (`"strict": true` in tsconfig)
- No `any` types (use generics or better typing)
- Use PrimeVue components for consistency
- Import types with `type` keyword to avoid circular dependencies

```typescript
// Good
import type { Case } from '@/types'
import { useCases } from '@/stores/cases'

// Bad
import { Case, useCases } from '@/types'  // Types and values mixed
const data: any = {}  // No type safety
```

## Audit Trail Requirements

Any state-changing operation must record an audit event within the same transaction:

```go
// Good: Audit event recorded in transaction
func (s *Service) CloseCase(ctx context.Context, id string, reason string) error {
  // All mutations in a single transaction
  tx, err := s.db.Begin(ctx)
  if err != nil {
    return err
  }
  defer tx.Rollback(ctx)

  // Make the change
  case, _ := s.Get(ctx, id)
  case.Status = "closed"
  tx.Exec(ctx, "UPDATE cases SET status = $1 WHERE id = $2", "closed", id)

  // Record audit event in same transaction
  tx.Exec(ctx, `
    INSERT INTO case_events (id, case_id, tenant_id, action, actor, changes, hash, created_at)
    VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
  `, eventID, id, tenantID, "case:closed", userID, changesJSON, hash)

  return tx.Commit(ctx)
}

// Bad: No audit trail
func (s *Service) CloseCase(ctx context.Context, id string) error {
  case, _ := s.Get(ctx, id)
  case.Status = "closed"
  return s.store.Update(ctx, case)  // No event recorded
}
```

## Submitting Changes

### Before You Submit

Run the full quality check:

```bash
qp guards        # Format + lint + test
qp guards:full   # Full suite (fmt + lint + test:all + vet + build + arch-check + vulncheck)
```

All must pass before merge.

### Pull Request Checklist

Guardian checklist (from AGENTS.md — all 12 must be satisfied):

1. [ ] `gofmt` produces no changes
2. [ ] `golangci-lint` reports zero issues
3. [ ] All unit tests pass (`go test ./internal/...`)
4. [ ] All integration tests pass (`go test ./tests/integration/...`)
5. [ ] All BDD scenarios pass (`go test ./tests/bdd/...`)
6. [ ] New code has corresponding tests
7. [ ] No cross-package internal imports (verified by `qp check:imports`)
8. [ ] No direct cross-package table queries (all queries use public APIs)
9. [ ] All DB writes are within a single transaction per step transition
10. [ ] RBAC check on every new API endpoint
11. [ ] Audit event recorded for every state-changing operation
12. [ ] Every tenant-scoped query includes `WHERE tenant_id = $1`

Additional checks:
- [ ] Documentation updated (if user-facing)
- [ ] Commit messages are clear and descriptive
- [ ] No breaking API changes (or documented)

### Commit Guidelines

Use clear, imperative commit messages:

```
Capitalize the first line (50 chars max)

- Use bullet points for multiple changes
- Reference issues: "Fixes #123"
- Explain the "why", not the "what"
- Keep line length under 72 chars

Fixes #123
```

**Good examples**:
```
Add JSON schema validation for case data
Create database migration for new case_types table
Fix race condition in step dispatch
Document expressions feature
```

**Bad examples**:
```
Fix bug
Update code
wip
changed things
```

## Testing Expectations

### Code Coverage

Aim for >80% coverage on business logic. Use:

```bash
go test ./internal/... -cover
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out  # View in browser
```

### Test Levels

- **Unit tests** (80%): Business logic, validators, transformations
- **Integration tests** (15%): Database interactions, cross-package flows
- **E2E tests** (5%): Critical user workflows (via Playwright)

## Questions?

- Open an **issue** for bugs or feature requests
- Open a **discussion** for questions or ideas
- Check **existing issues** before creating duplicates

Thank you for contributing to Aceryx!
