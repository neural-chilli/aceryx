---
title: Building & Testing
weight: 5
---

Aceryx is built with Go 1.26+ and Node 20+. The build process compiles the backend, builds the frontend with Vite, and embeds everything into a single binary.

## Prerequisites

- **Go** 1.26+ ([download](https://golang.org/dl/))
- **Node** 20+ ([download](https://nodejs.org/))
- **PostgreSQL** 17 (or Docker)

Verify installation:

```bash
go version       # Should be 1.26+
node --version   # Should be 20+
npm --version    # Should be 10+
```

## Quick Start

### Development Mode (with hot reload)

```bash
# Start backend + frontend with live reload
qp dev

# Backend: http://localhost:8080
# Frontend: http://localhost:5173 (Vite dev server)
```

This runs:
1. **Backend**: `go run ./cmd/aceryx serve` with auto-reload on file changes
2. **Frontend**: `npm run dev` (Vite) with hot module replacement
3. **Database**: PostgreSQL in Docker (via docker-compose)

### Production Build

```bash
# Build the full binary
make build

# Or manually:
make build-frontend
make build-backend

# Result: ./bin/aceryx (single binary, ready to deploy)
```

## Build Targets

### `make build`

Full production build:
1. Builds frontend (Vue + TypeScript + Vite) to `frontend/dist/`
2. Embeds frontend into Go binary via `go:embed`
3. Builds Go backend with version/build info

Output: `./bin/aceryx`

### `make dev`

Start development environment:
```bash
# Equivalent to `qp dev`
make dev
```

Runs backend, frontend, and PostgreSQL (docker-compose).

### `make lint`

Run linters:
- `gofmt` — Go formatting
- `golangci-lint` — Go code quality
- `eslint` — JavaScript/TypeScript linting

All files must pass linting before merging.

### `make test`

Run test suite:
- Unit tests: `go test ./internal/... -race`
- Integration tests: `go test ./tests/integration/... -race`

### `make clean`

Remove build artifacts (`./bin/`, `frontend/dist/`, etc.).

## Go Build

### Backend Build

```bash
go build -o bin/aceryx ./cmd/aceryx

# With version info (recommended)
go build -ldflags "-X main.Version=1.0.0 -X main.CommitHash=$(git rev-parse --short HEAD)" \
  -o bin/aceryx ./cmd/aceryx
```

### Environment Variables

Configuration is loaded from environment or `.env` file:

```env
# Database
DATABASE_URL=postgres://user:pass@localhost/aceryx

# Server
LISTEN_ADDR=:8080
FRONTEND_ENABLED=true

# JWT
JWT_SECRET=your-secret-key-change-in-production

# LLM
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...

# Vault (document storage)
VAULT_STORAGE_PATH=./vault
VAULT_MAX_SIZE_MB=10000

# Logging
LOG_LEVEL=info
LOG_FORMAT=json

# Features
ENABLE_WEBHOOKS=true
ENABLE_REPORTS=true
```

See `.env.example` for full list.

## Frontend Build

### Development

```bash
cd frontend
npm install
npm run dev
```

Vite dev server runs on `http://localhost:5173` with hot module replacement.

### Production

```bash
cd frontend
npm run build
```

Output: `frontend/dist/` (static assets)

### Frontend Dependencies

Key packages:
- **Vue 3** — UI framework
- **TypeScript** — Type safety
- **Vite** — Build tool
- **PrimeVue** — Component library
- **VueFlow** — Workflow diagram editor
- **Axios** — HTTP client
- **Pinia** — State management
- **Vitest** — Unit testing
- **Playwright** — E2E testing

## Testing

### Unit Tests

Test business logic in isolation.

```bash
# Run all unit tests
go test ./internal/... -v

# Run with race detector
go test ./internal/... -race

# Run specific package
go test ./internal/cases -v

# Run with coverage
go test ./internal/... -cover

# Generate coverage report
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

Test files use `*_test.go` naming convention and live alongside code.

### Integration Tests

Test multiple components working together (database, API, etc.). These use testcontainers to spin up PostgreSQL.

```bash
# Run integration tests (requires Docker)
go test ./tests/integration/... -v -race -tags=integration

# Or via make
make test:integration
```

Example integration test:

```go
// tests/integration/cases_test.go
func TestCaseCreation(t *testing.T) {
  db := setupTestDB(t)  // Spin up Postgres container
  defer db.Close()

  service := cases.NewService(db)
  case_, err := service.Create(context.Background(), &cases.CreateInput{
    CaseTypeID: typeID,
    Data: map[string]interface{}{"name": "Test Case"},
  })

  require.NoError(t, err)
  require.NotNil(t, case_)
  assert.Equal(t, "Test Case", case_.Data["name"])
}
```

### BDD Tests

Behavior-Driven Development tests using Gherkin syntax (godog).

```bash
# Run BDD tests
go test ./tests/bdd/... -v -tags=bdd
```

Example BDD scenario:

```gherkin
# tests/bdd/features/cases.feature
Feature: Case Management
  Scenario: Create a case
    Given a case type "complaint"
    When I create a case with data:
      | field | value |
      | description | System is down |
    Then the case is created successfully
    And the case status is "active"
```

### Frontend Unit Tests

```bash
cd frontend
npm run test:unit

# Watch mode
npm run test:unit -- --watch

# Coverage
npm run test:unit -- --coverage
```

Uses **Vitest** with Vue 3 testing utilities.

### Frontend E2E Tests

End-to-end tests using Playwright (tests full user workflows).

```bash
cd frontend
npx playwright test

# In headed mode (see browser)
npx playwright test --headed

# Single test file
npx playwright test tests/e2e/login.spec.ts

# Debug mode
npx playwright test --debug
```

Example Playwright test:

```typescript
// frontend/tests/e2e/case-creation.spec.ts
import { test, expect } from '@playwright/test';

test('Create a case', async ({ page }) => {
  await page.goto('http://localhost:5173');
  await page.click('[data-testid="new-case-btn"]');
  await page.fill('[name="description"]', 'Test case');
  await page.click('[data-testid="submit-btn"]');

  await expect(page.locator('.success-message')).toBeVisible();
});
```

## Code Quality & CI

### The `qp guards` Workflow

Before merging, code must pass:

```bash
# Quick guard check (format + lint + test)
qp guards

# Full guard check (format + lint + test:all + vet + build + arch-check + vulncheck)
qp guards:full
```

The `qp guards` task runs:
1. **Format**: `gofmt -w .`
2. **Lint**: `golangci-lint run ./...`
3. **Test**: `go test ./... -count=1 -race`

The `qp guards:full` task additionally runs:
- All test suites (unit, integration, BDD, frontend)
- `go vet ./...`
- Full build (`go build ./cmd/aceryx`)
- `qp arch-check` (enforce package boundaries)
- `govulncheck ./...` (security checks)

All must pass. If any fails, fix and rerun.

### Import Verification

Enforce package boundaries:

```bash
qp check:imports
```

This verifies:
- No package imports another's internals
- Cross-package queries use public APIs only
- No circular dependencies

### Continuous Integration

GitHub Actions (`.github/workflows/`):

- **test.yml** — Run tests on every PR
- **lint.yml** — Run linters on every PR
- **release.yml** — Build and publish on tag

Example CI job:

```yaml
name: Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:17
        env:
          POSTGRES_PASSWORD: postgres
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v4
        with:
          go-version: '1.26'
      - run: go test ./... -race
```

### Performance Tests

Performance benchmarks are located in `tests/integration/performance_test.go`.

```bash
# Run performance tests
go test ./tests/integration/... -bench=. -benchmem -tags=integration

# Compare with baseline
go test ./tests/integration/... -bench=. -benchmem -tags=integration -benchtime=10s
```

## Debugging

### Verbose Logging

Set log level:

```bash
LOG_LEVEL=debug go run ./cmd/aceryx serve
```

Output includes slog messages in JSON format:

```json
{
  "time": "2026-04-04T10:00:00Z",
  "level": "DEBUG",
  "msg": "Step activated",
  "case_id": "uuid",
  "step_id": "uuid",
  "step_type": "task"
}
```

### Local Database Inspection

```bash
# Connect to local Postgres
psql -U postgres -h localhost -d aceryx

# Useful queries
SELECT COUNT(*) FROM cases;
SELECT * FROM case_events WHERE case_id = 'uuid' ORDER BY created_at;
SELECT * FROM principals WHERE email = 'user@example.com';
```

### Trace Logging

For detailed execution traces (engine, step dispatch):

```bash
LOG_LEVEL=trace go run ./cmd/aceryx serve
```

### pprof Profiling

Go's built-in profiler (if enabled in main.go):

```go
import _ "net/http/pprof"
```

Access profiling endpoints:
- `http://localhost:6060/debug/pprof/` — Profile index
- `http://localhost:6060/debug/pprof/heap` — Memory profile
- `http://localhost:6060/debug/pprof/goroutine` — Goroutine count

## Docker

### Build Container Image

```bash
docker build -t aceryx:latest .

# Run in container
docker run -e DATABASE_URL=postgres://... aceryx:latest
```

Dockerfile uses multi-stage build to minimize image size.

### Docker Compose

For local development:

```bash
docker-compose up

# Services:
# - aceryx (backend)
# - postgres (database)
# - pgadmin (optional, DB UI)
```

## Deployment

### Single Binary

Aceryx compiles to a single binary with everything embedded:

```bash
./bin/aceryx serve
```

Deploy to:
- Bare metal server
- Docker container
- Kubernetes (with ConfigMap for config)
- AWS Lambda (experimental)
- Cloud Run (experimental)

### Kubernetes

Example deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aceryx
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: aceryx
        image: aceryx:latest
        ports:
        - containerPort: 8080
        env:
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: aceryx-secrets
              key: database-url
        - name: JWT_SECRET
          valueFrom:
            secretKeyRef:
              name: aceryx-secrets
              key: jwt-secret
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 15
          periodSeconds: 20
```

## Performance Optimization

### Build Size

```bash
# Check binary size
ls -lh bin/aceryx

# Strip debug symbols (smaller but no debugging)
go build -ldflags "-s -w" -o bin/aceryx ./cmd/aceryx
```

### Build Speed

Use `make build-cache`:

```bash
# Enable Go build cache
export GOCACHE=/tmp/go-cache
go build -o bin/aceryx ./cmd/aceryx
```

Or use build cache in CI:

```yaml
- uses: actions/cache@v3
  with:
    path: ~/go/pkg/mod
    key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
    restore-keys: |
      ${{ runner.os }}-go-
```
