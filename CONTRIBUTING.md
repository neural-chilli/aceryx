# Contributing to Aceryx

Thanks for your interest in contributing to Aceryx. This guide covers how we work, how to set up your environment, and how to get your changes merged.

## How We Work

Aceryx follows a **spec-first methodology**. Every feature starts as a specification in `docs/specs/` with BDD scenarios before any code is written. If you're proposing a new feature, start with a spec. If you're fixing a bug, reference the relevant spec.

The specs are the source of truth. If the code and the spec disagree, the spec wins (or the spec needs updating — file an issue).

## Getting Started

**Prerequisites:**

- Go 1.22+
- PostgreSQL 16+ with pgvector extension
- TinyGo 0.31+ (for plugin development only)
- Node.js 20+ and pnpm (for frontend only)

**Setup:**

```bash
git clone https://github.com/neural-chilli/aceryx.git
cd aceryx

# Start Postgres (or use your own)
docker compose up -d postgres

# Run migrations
go run ./cmd/aceryx migrate up

# Run the server
go run ./cmd/aceryx serve

# Run tests
go test ./...
```

## Making Changes

### Bug Fixes

1. Open an issue describing the bug with steps to reproduce.
2. Fork the repo and create a branch: `fix/short-description`.
3. Write a test that reproduces the bug.
4. Fix the bug.
5. Verify all tests pass: `go test ./...`
6. Open a PR referencing the issue.

### Features

1. Open an issue describing the feature and which spec it relates to (or propose a new spec).
2. Discuss the approach before writing code — this saves everyone time.
3. Fork the repo and create a branch: `feat/short-description`.
4. Implement against the spec. Include tests covering the spec's BDD scenarios.
5. Open a PR referencing the spec and issue.

### Plugin Contributions

If you're building a new connector plugin (not modifying the core):

1. Scaffold with `aceryx plugin init --lang=go --type=step --name=your-connector`.
2. Implement against the Plugin SDK (`sdk/go/`).
3. Include a `manifest.yaml` with all required fields.
4. Include tests using `sdk.MockContext`.
5. Open a PR into `plugins/community/`.

Community plugins ship with `maturity: community` in their manifest.

## Code Standards

**Go:**

- Idiomatic Go. Run `gofmt` and `go vet` before committing.
- Error handling via `(value, error)` returns. No panics in library code.
- No global state. Dependencies are passed explicitly.
- No ORMs. SQL via `pgx` with parameterised queries.
- Test files live alongside the code they test.

**Naming:**

- Packages: lowercase, single word where possible.
- Interfaces: named for what they do, not what they are (`Store`, not `IStore`).
- Errors: `fmt.Errorf` with context, or sentinel errors in `errors.go`.

**Testing:**

- Table-driven tests where appropriate.
- Use `testdata/` for fixtures.
- Integration tests use the test Postgres instance. Unit tests use mocks.
- Aim for tests that document behaviour, not tests that chase coverage numbers.

**Commits:**

- Short, descriptive commit messages: `fix: prevent SQL injection in query executor` or `feat: add HMAC webhook authentication`.
- One logical change per commit. Squash before merging if needed.

## Project Structure

```
cmd/aceryx/          — CLI and server entry point
internal/            — core packages (not importable externally)
  cases/             — case management
  channels/          — data ingestion channels and pipeline
  drivers/           — core drivers (database, queue, file, protocol)
  engine/            — workflow execution engine
  http/              — HTTP connector framework
  llm/               — LLM adapter framework
  plugins/           — plugin runtime, registry, host functions
  triggers/          — trigger plugin framework
sdk/go/              — plugin SDK for Go/TinyGo
docs/specs/          — feature specifications with BDD scenarios
migrations/          — Postgres migration files
frontend/            — Vue/PrimeVue application
```

## Licensing

Aceryx uses a dual licence model. The open-source core is MIT licensed. Commercial features are licensed under BSL 1.1 with a change date. Each spec declares its tier. If you're unsure whether your contribution falls under open source or commercial, ask in the issue before starting work.

## Questions?

Open a discussion on GitHub or reach out at hello@neuralchilli.com.
