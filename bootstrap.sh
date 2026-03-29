#!/usr/bin/env bash
set -euo pipefail

# Aceryx Repository Bootstrap
# Creates the complete project structure, initialises Go module,
# scaffolds Vue/PrimeVue frontend, and sets up tooling.

REPO_ROOT="${1:-.}"
GO_VERSION="1.26.1"
NODE_MIN="20"
MODULE="github.com/neural-chilli/aceryx"

# Colours
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $1"; }
warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[x]${NC} $1"; exit 1; }

# ── Preflight checks ────────────────────────────────────────────

check_command() {
    command -v "$1" >/dev/null 2>&1 || error "$1 is required but not installed."
}

info "Running preflight checks..."
check_command go
check_command node
check_command npm

GO_CURRENT=$(go version | sed 's/.*go\([0-9]*\.[0-9]*\).*/\1/')
GO_MAJOR=$(echo "$GO_CURRENT" | cut -d. -f1)
GO_MINOR=$(echo "$GO_CURRENT" | cut -d. -f2)
if (( GO_MAJOR < 1 || (GO_MAJOR == 1 && GO_MINOR < 26) )); then
    error "Go >= 1.26 required (found $GO_CURRENT). Install from https://go.dev/dl/"
fi
info "Go $GO_CURRENT ✓"

NODE_CURRENT=$(node --version | sed 's/v//' | cut -d. -f1)
if (( NODE_CURRENT < NODE_MIN )); then
    error "Node >= $NODE_MIN required (found v$NODE_CURRENT)."
fi
info "Node v$(node --version | sed 's/v//') ✓"

# ── Create repo root ────────────────────────────────────────────

if [ "$REPO_ROOT" != "." ]; then
    mkdir -p "$REPO_ROOT"
fi
cd "$REPO_ROOT"
REPO_ROOT=$(pwd)
info "Repository root: $REPO_ROOT"

# ── Directory structure (matches AGENTS.md) ─────────────────────

info "Creating directory structure..."

# Go backend
mkdir -p cmd/aceryx
mkdir -p internal/{engine,cases,steps,tasks,connectors,agents,vault,rbac,audit,notify,search,expressions,workflows}
mkdir -p api/{handlers,middleware}
mkdir -p migrations
mkdir -p tests/{integration,bdd/features}

# Docs
mkdir -p docs/{design,specs}

# Config
mkdir -p deploy/{docker,helm}

# ── Go module ───────────────────────────────────────────────────

info "Initialising Go module..."
if [ ! -f go.mod ]; then
    go mod init "$MODULE"
    go mod edit -go=1.26
fi

# ── Main entrypoint ─────────────────────────────────────────────

info "Creating Go entrypoint..."
cat > cmd/aceryx/main.go << 'GOEOF'
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("aceryx v0.0.1-dev")
		return
	}

	fmt.Println("aceryx - case orchestration engine")
	fmt.Println("usage: aceryx [serve|version]")
}
GOEOF

# ── Package stubs ───────────────────────────────────────────────

info "Creating package stubs..."

# Engine
cat > internal/engine/engine.go << 'GOEOF'
// Package engine implements the DAG workflow scheduler, step lifecycle
// management, and recovery logic.
package engine
GOEOF

# Cases
cat > internal/cases/cases.go << 'GOEOF'
// Package cases implements case CRUD, case type schema validation,
// and case lifecycle management.
package cases
GOEOF

# Steps
cat > internal/steps/steps.go << 'GOEOF'
// Package steps implements step execution dispatch for all step types:
// human_task, agent, integration, rule, timer, notification, subflow.
package steps
GOEOF

# Tasks
cat > internal/tasks/tasks.go << 'GOEOF'
// Package tasks implements human task assignment, claiming, completion,
// SLA tracking, and escalation.
package tasks
GOEOF

# Connectors
cat > internal/connectors/connectors.go << 'GOEOF'
// Package connectors defines the Connector interface and provides
// the connector registry and built-in connector implementations.
package connectors

// Connector defines the interface that all connectors must implement.
type Connector interface {
	Meta() ConnectorMeta
	Auth() AuthSpec
	Triggers() []TriggerSpec
	Actions() []ActionSpec
}

// ConnectorMeta describes a connector.
type ConnectorMeta struct {
	Key         string
	Name        string
	Description string
	Version     string
	Icon        string
}

// AuthSpec describes a connector's authentication requirements.
type AuthSpec struct {
	Type   string // "none", "api_key", "oauth2", "basic"
	Fields []AuthField
}

// AuthField describes a single auth configuration field.
type AuthField struct {
	Key      string
	Label    string
	Type     string // "string", "password", "url"
	Required bool
}

// TriggerSpec describes an inbound trigger a connector provides.
type TriggerSpec struct {
	Key         string
	Name        string
	Description string
	Type        string // "webhook", "polling", "scheduled"
}

// ActionSpec describes an action a connector can perform.
type ActionSpec struct {
	Key         string
	Name        string
	Description string
	InputSchema map[string]any
	OutputSchema map[string]any
}
GOEOF

# Agents
cat > internal/agents/agents.go << 'GOEOF'
// Package agents implements agent step execution: context assembly,
// prompt template rendering, LLM invocation, output validation,
// confidence evaluation, and human escalation.
package agents
GOEOF

# Vault
cat > internal/vault/vault.go << 'GOEOF'
// Package vault implements the document storage layer with content-hash
// addressing, metadata management, and RBAC-controlled access.
package vault

// VaultStore defines the interface for document storage backends.
type VaultStore interface {
	Put(tenantID, hash, ext string, data []byte) (uri string, err error)
	Get(uri string) ([]byte, error)
	Delete(uri string) error
	SignedURL(uri string, expirySeconds int) (string, error)
}
GOEOF

# RBAC
cat > internal/rbac/rbac.go << 'GOEOF'
// Package rbac implements role-based access control with principals,
// roles, permissions, and the Authorize function.
package rbac
GOEOF

# Audit
cat > internal/audit/audit.go << 'GOEOF'
// Package audit implements the append-only, tamper-evident event log
// with cryptographic hash chaining.
package audit
GOEOF

# Notify
cat > internal/notify/notify.go << 'GOEOF'
// Package notify implements notification dispatch via email and
// WebSocket push.
package notify
GOEOF

# Search
cat > internal/search/search.go << 'GOEOF'
// Package search implements full-text search across case data
// using Postgres tsvector/tsquery.
package search
GOEOF

# Expressions
cat > internal/expressions/expressions.go << 'GOEOF'
// Package expressions implements sandboxed JavaScript expression
// evaluation via goja for conditions, computed fields, and guards.
package expressions
GOEOF

# Workflows
cat > internal/workflows/workflows.go << 'GOEOF'
// Package workflows implements the workflow AST, YAML serialisation,
// versioning, validation, and the canonical model.
package workflows
GOEOF

# API handlers stub
cat > api/handlers/health.go << 'GOEOF'
package handlers

import (
	"encoding/json"
	"net/http"
)

// HealthResponse is returned by the health check endpoint.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Health returns the service health status.
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HealthResponse{
		Status:  "ok",
		Version: "0.0.1-dev",
	})
}
GOEOF

# API routes stub
cat > api/routes.go << 'GOEOF'
package api

import (
	"net/http"

	"github.com/neural-chilli/aceryx/api/handlers"
)

// NewRouter creates and configures the HTTP router.
func NewRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handlers.Health)

	// Case endpoints
	// mux.HandleFunc("GET /cases", handlers.ListCases)
	// mux.HandleFunc("POST /cases", handlers.CreateCase)
	// mux.HandleFunc("GET /cases/{id}", handlers.GetCase)

	// Task endpoints
	// mux.HandleFunc("GET /tasks", handlers.ListTasks)
	// mux.HandleFunc("POST /tasks/{id}/claim", handlers.ClaimTask)
	// mux.HandleFunc("POST /tasks/{id}/complete", handlers.CompleteTask)

	// Workflow endpoints
	// mux.HandleFunc("GET /workflows", handlers.ListWorkflows)
	// mux.HandleFunc("POST /workflows/{id}/publish", handlers.PublishWorkflow)

	return mux
}
GOEOF

# API middleware stub
cat > api/middleware/auth.go << 'GOEOF'
package middleware

import "net/http"

// Auth validates the request authentication.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: implement authentication check
		next.ServeHTTP(w, r)
	})
}
GOEOF

# ── Initial migration stub ─────────────────────────────────────

info "Creating migration stub..."
cat > migrations/001_initial.sql << 'SQLEOF'
-- Aceryx initial schema
-- See docs/specs/001-postgres-schema.md for full specification

BEGIN;

-- Cases
CREATE TABLE IF NOT EXISTS cases (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    case_type_id    UUID NOT NULL,
    case_number     TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open',
    current_stage   TEXT,
    data            JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID NOT NULL,
    assigned_to     UUID,
    due_at          TIMESTAMPTZ,
    priority        INTEGER DEFAULT 0,
    version         INTEGER NOT NULL DEFAULT 1,
    workflow_id     UUID NOT NULL,
    workflow_version INTEGER NOT NULL,
    UNIQUE(tenant_id, case_number)
);

-- Case Steps (execution state)
CREATE TABLE IF NOT EXISTS case_steps (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id         UUID NOT NULL REFERENCES cases(id),
    step_id         TEXT NOT NULL,
    state           TEXT NOT NULL DEFAULT 'pending',
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    result          JSONB,
    events          JSONB NOT NULL DEFAULT '[]',
    error           JSONB,
    assigned_to     UUID,
    sla_deadline    TIMESTAMPTZ,
    metadata        JSONB,
    UNIQUE(case_id, step_id)
);

CREATE INDEX idx_case_steps_state ON case_steps(state) WHERE state IN ('ready', 'active');
CREATE INDEX idx_case_steps_sla ON case_steps(sla_deadline) WHERE state = 'active' AND sla_deadline IS NOT NULL;

-- Case Events (audit trail)
CREATE TABLE IF NOT EXISTS case_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id         UUID NOT NULL REFERENCES cases(id),
    step_id         TEXT,
    event_type      TEXT NOT NULL,
    actor_id        UUID NOT NULL,
    actor_type      TEXT NOT NULL CHECK (actor_type IN ('human', 'agent', 'system')),
    action          TEXT NOT NULL,
    data            JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    prev_event_hash TEXT NOT NULL,
    event_hash      TEXT NOT NULL
);

CREATE INDEX idx_case_events_case ON case_events(case_id, created_at);

-- Vault Documents
CREATE TABLE IF NOT EXISTS vault_documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    case_id         UUID REFERENCES cases(id),
    step_id         TEXT,
    filename        TEXT NOT NULL,
    mime_type       TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL,
    content_hash    TEXT NOT NULL,
    storage_uri     TEXT NOT NULL,
    uploaded_by     UUID NOT NULL,
    uploaded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    extracted_text  TEXT,
    extracted_data  JSONB,
    metadata        JSONB
);

-- RBAC
CREATE TABLE IF NOT EXISTS principals (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type            TEXT NOT NULL CHECK (type IN ('human', 'agent')),
    name            TEXT NOT NULL,
    email           TEXT,
    password_hash   TEXT,
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS roles (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT,
    UNIQUE(tenant_id, name)
);

CREATE TABLE IF NOT EXISTS principal_roles (
    principal_id    UUID NOT NULL REFERENCES principals(id),
    role_id         UUID NOT NULL REFERENCES roles(id),
    PRIMARY KEY (principal_id, role_id)
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id         UUID NOT NULL REFERENCES roles(id),
    permission      TEXT NOT NULL,
    PRIMARY KEY (role_id, permission)
);

-- Workflows
CREATE TABLE IF NOT EXISTS workflows (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    name            TEXT NOT NULL,
    case_type       TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

CREATE TABLE IF NOT EXISTS workflow_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id     UUID NOT NULL REFERENCES workflows(id),
    version         INTEGER NOT NULL,
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'withdrawn')),
    ast             JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at    TIMESTAMPTZ,
    UNIQUE(workflow_id, version)
);

-- Prompt Templates
CREATE TABLE IF NOT EXISTS prompt_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    name            TEXT NOT NULL,
    version         INTEGER NOT NULL,
    template        TEXT NOT NULL,
    output_schema   JSONB,
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name, version)
);

-- Case Types
CREATE TABLE IF NOT EXISTS case_types (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    name            TEXT NOT NULL,
    version         INTEGER NOT NULL,
    schema          JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name, version)
);

-- Webhook Deliveries (idempotency)
CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key TEXT NOT NULL UNIQUE,
    payload         JSONB,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;
SQLEOF

# ── Test stubs ──────────────────────────────────────────────────

info "Creating test stubs..."

cat > internal/engine/engine_test.go << 'GOEOF'
package engine_test

import "testing"

func TestPlaceholder(t *testing.T) {
	// Placeholder — replaced by real tests when engine is implemented
	t.Log("engine package: tests pending implementation")
}
GOEOF

cat > tests/bdd/features/loan_origination.feature << 'GOEOF'
Feature: Loan origination workflow

  Scenario: Low risk loan is auto-approved
    Given a case type "loan_application" is registered
    And a workflow "loan_origination" is deployed
    When a case is created with:
      | field                  | value    |
      | applicant.company_name | Acme Ltd |
      | loan.amount            | 8000     |
    And the intake step completes
    And the agent "risk_assessment" returns:
      | score | risk_level | confidence |
      | 85    | low        | 0.92       |
    Then the case should proceed to "generate_offer"
    And no human task should be created

  Scenario: High value loan requires underwriter review
    Given a case type "loan_application" is registered
    And a workflow "loan_origination" is deployed
    When a case is created with:
      | field       | value   |
      | loan.amount | 150000  |
    And the risk assessment completes with score 65
    And the auto_decision routes to "underwriter_review"
    Then a task should be created for role "underwriter"
    And the task SLA should be 24 hours

  Scenario: Underwriter refers to senior
    Given a case with an active underwriter review task
    When the underwriter submits outcome "referred"
    Then a task should be created for role "senior_underwriter"
    And the task SLA should be 48 hours
    And the underwriter review task should be completed
GOEOF

# ── Frontend scaffolding ────────────────────────────────────────

info "Scaffolding Vue frontend..."

cd "$REPO_ROOT"

if [ ! -d "frontend" ]; then
    npm create vite@latest frontend -- --template vue-ts 2>/dev/null
else
    warn "frontend/ already exists, skipping Vite scaffold"
fi

cd frontend

info "Installing frontend dependencies..."
npm install 2>/dev/null

# PrimeVue and dependencies
npm install primevue@latest primeicons @primeuix/themes 2>/dev/null

# VueFlow for the workflow builder
npm install @vue-flow/core @vue-flow/background @vue-flow/controls @vue-flow/minimap 2>/dev/null

# Vue Router and state management
npm install vue-router@latest pinia@latest 2>/dev/null

# Dev dependencies
npm install -D @vue/test-utils vitest @playwright/test 2>/dev/null

# Configure PrimeVue in main.ts
cat > src/main.ts << 'TSEOF'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHistory } from 'vue-router'
import PrimeVue from 'primevue/config'
import Aura from '@primeuix/themes/aura'
import 'primeicons/primeicons.css'

import App from './App.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'inbox', component: () => import('./views/Inbox.vue') },
    { path: '/cases/:id', name: 'case', component: () => import('./views/CaseView.vue') },
    { path: '/builder', name: 'builder', component: () => import('./views/Builder.vue') },
    { path: '/builder/:id', name: 'builder-edit', component: () => import('./views/Builder.vue') },
  ],
})

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(PrimeVue, {
  theme: {
    preset: Aura,
  },
})
app.mount('#app')
TSEOF

# App component
cat > src/App.vue << 'VUEEOF'
<script setup lang="ts">
import { RouterView } from 'vue-router'
</script>

<template>
  <div class="app-layout">
    <nav class="app-nav">
      <div class="app-brand">
        <span class="brand-name">aceryx</span>
      </div>
      <ul class="nav-links">
        <li><RouterLink to="/">Inbox</RouterLink></li>
        <li><RouterLink to="/builder">Builder</RouterLink></li>
      </ul>
    </nav>
    <main class="app-main">
      <RouterView />
    </main>
  </div>
</template>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

.app-layout {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.app-nav {
  display: flex;
  align-items: center;
  gap: 2rem;
  padding: 0.75rem 1.5rem;
  background: var(--p-surface-800, #1e1e2e);
  color: white;
}

.brand-name {
  font-weight: 700;
  font-size: 1.25rem;
  letter-spacing: -0.02em;
}

.nav-links {
  display: flex;
  list-style: none;
  gap: 1rem;
}

.nav-links a {
  color: var(--p-surface-300, #ccc);
  text-decoration: none;
  font-size: 0.875rem;
}

.nav-links a.router-link-active {
  color: white;
}

.app-main {
  flex: 1;
  padding: 1.5rem;
  background: var(--p-surface-50, #f8f8f8);
}
</style>
VUEEOF

# View stubs
mkdir -p src/views src/components src/composables

cat > src/views/Inbox.vue << 'VUEEOF'
<script setup lang="ts">
// Task inbox — shows assigned and claimable tasks
</script>

<template>
  <div>
    <h1>Task Inbox</h1>
    <p>Task list will be rendered here using PrimeVue DataTable.</p>
  </div>
</template>
VUEEOF

cat > src/views/CaseView.vue << 'VUEEOF'
<script setup lang="ts">
import { useRoute } from 'vue-router'

const route = useRoute()
const caseId = route.params.id
</script>

<template>
  <div>
    <h1>Case {{ caseId }}</h1>
    <p>Case detail view: timeline, documents, active task form.</p>
  </div>
</template>
VUEEOF

cat > src/views/Builder.vue << 'VUEEOF'
<script setup lang="ts">
// Workflow visual builder — VueFlow canvas with step palette
</script>

<template>
  <div>
    <h1>Workflow Builder</h1>
    <p>VueFlow canvas will be rendered here.</p>
  </div>
</template>
VUEEOF

# API client composable
cat > src/composables/useApi.ts << 'TSEOF'
const API_BASE = import.meta.env.VITE_API_BASE || '/api'

export function useApi() {
  async function get<T>(path: string): Promise<T> {
    const res = await fetch(`${API_BASE}${path}`)
    if (!res.ok) throw new Error(`API error: ${res.status}`)
    return res.json()
  }

  async function post<T>(path: string, body: unknown): Promise<T> {
    const res = await fetch(`${API_BASE}${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw new Error(`API error: ${res.status}`)
    return res.json()
  }

  async function put<T>(path: string, body: unknown): Promise<T> {
    const res = await fetch(`${API_BASE}${path}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw new Error(`API error: ${res.status}`)
    return res.json()
  }

  return { get, post, put }
}
TSEOF

cd "$REPO_ROOT"

# ── Config files ────────────────────────────────────────────────

info "Creating config files..."

# golangci-lint config
cat > .golangci.yml << 'YAMLEOF'
run:
  timeout: 5m

linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gofmt
    - goimports
    - misspell
    - unconvert
    - unparam

linters-settings:
  goimports:
    local-prefixes: github.com/neural-chilli/aceryx

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - errcheck
        - unparam
YAMLEOF

# .env.example
cat > .env.example << 'ENVEOF'
# Aceryx configuration
ACERYX_PORT=8080
ACERYX_DB_URL=postgres://aceryx:aceryx@localhost:5432/aceryx?sslmode=disable
ACERYX_VAULT_PATH=./data/vault
ACERYX_LOG_LEVEL=info

# LLM configuration
ACERYX_LLM_ENDPOINT=https://api.openai.com/v1
ACERYX_LLM_MODEL=gpt-4o
ACERYX_LLM_API_KEY=

# Frontend dev proxy (Vite forwards /api to this)
VITE_API_BASE=http://localhost:8080
ENVEOF

# .gitignore
cat > .gitignore << 'GITEOF'
# Go
/aceryx
*.exe
*.test
*.out
vendor/

# Frontend
frontend/node_modules/
frontend/dist/

# Data
/data/
*.db

# IDE
.idea/
.vscode/
*.swp
*.swo

# Env
.env
.env.local

# OS
.DS_Store
Thumbs.db
GITEOF

# Makefile
cat > Makefile << 'MAKEEOF'
.PHONY: build test lint run dev clean

# ── Build ──────────────────────────────────────────────────────

build:
	go build -o aceryx ./cmd/aceryx

build-frontend:
	cd frontend && npm run build

build-all: build build-frontend

# ── Test ───────────────────────────────────────────────────────

test:
	go test ./internal/... -count=1 -race

test-integration:
	go test ./tests/integration/... -count=1 -tags=integration -race

test-bdd:
	go test ./tests/bdd/... -count=1 -tags=bdd

test-frontend:
	cd frontend && npm run test:unit

test-e2e:
	cd frontend && npx playwright test

test-all: test test-integration test-bdd test-frontend

# ── Lint ───────────────────────────────────────────────────────

lint:
	gofmt -l .
	golangci-lint run ./...

lint-frontend:
	cd frontend && npm run lint && npm run type-check

lint-all: lint lint-frontend

# ── Run ────────────────────────────────────────────────────────

run: build
	./aceryx serve

dev:
	@echo "Starting backend..."
	go run ./cmd/aceryx serve &
	@echo "Starting frontend dev server..."
	cd frontend && npm run dev

# ── Guards (run all checks — matches AGENTS.md) ────────────────

guards: lint test
	@echo ""
	@echo "✓ All guards passed"

guards-all: lint-all test-all
	@echo ""
	@echo "✓ All guards passed (full suite)"

# ── Clean ──────────────────────────────────────────────────────

clean:
	rm -f aceryx
	rm -rf frontend/dist
	rm -rf data/
MAKEEOF

# Docker compose for local Postgres
cat > docker-compose.yml << 'DCEOF'
services:
  postgres:
    image: pgvector/pgvector:pg17
    environment:
      POSTGRES_USER: aceryx
      POSTGRES_PASSWORD: aceryx
      POSTGRES_DB: aceryx
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./migrations:/docker-entrypoint-initdb.d
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U aceryx"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  pgdata:
DCEOF

# ── Docs placeholders ──────────────────────────────────────────

info "Creating doc placeholders..."

cat > docs/design/README.md << 'MDEOF'
# Design Documents

- `aceryx-design-v0.4.md` — Product design & strategy (final)

Copy the design document into this directory.
MDEOF

cat > docs/specs/README.md << 'MDEOF'
# Feature Specifications

Feature specs are written here before implementation begins.
Each spec is a self-contained unit of work that can be handed to a coding agent.

## Spec Order

1. `001-postgres-schema.md` — Complete DDL, indexes, constraints
2. `002-execution-engine.md` — DAG scheduler, step lifecycle, recovery
3. `003-case-management-api.md` — Case CRUD, schema validation, versioning
4. `004-human-tasks.md` — Task assignment, claiming, completion, SLA
5. `005-connector-framework.md` — Connector interface, registry, v1 connectors
6. `006-agent-steps.md` — Context assembly, LLM invocation, validation
7. `007-vault.md` — Document storage, metadata, content hashing
8. `008-rbac.md` — Principals, roles, permissions, Authorize
9. `009-schema-driven-forms.md` — Form schema, renderer, data binding
10. `010-visual-builder.md` — VueFlow, AST editing, round-trip contract
11. `011-audit-trail.md` — Event recording, hash chain, verification
12. `012-notifications.md` — Email and WebSocket push
MDEOF

# ── Final setup ─────────────────────────────────────────────────

info "Running go mod tidy..."
cd "$REPO_ROOT"
go mod tidy 2>/dev/null || true

info "Verifying build..."
go build ./cmd/aceryx && rm -f aceryx
info "Go build ✓"

info "Verifying frontend..."
if cd frontend && npx vue-tsc --noEmit 2>/dev/null; then
    info "Frontend type check ✓"
else
    warn "Frontend type check had warnings (expected with stubs)"
fi
cd "$REPO_ROOT"

# ── Summary ─────────────────────────────────────────────────────

echo ""
echo "════════════════════════════════════════════════════════════"
echo ""
info "Aceryx repo bootstrapped successfully!"
echo ""
echo "  Go module:    $MODULE"
echo "  Go version:   $(go version | sed 's/.*\(go[0-9.]*\).*/\1/')"
echo "  Node version: $(node --version)"
echo "  Root:         $REPO_ROOT"
echo ""
echo "  Next steps:"
echo "    1. Copy AGENTS.md into the repo root"
echo "    2. Copy aceryx-design-v0.4.md into docs/design/"
echo "    3. Start Postgres:  docker compose up -d"
echo "    4. Run backend:     go run ./cmd/aceryx serve"
echo "    5. Run frontend:    cd frontend && npm run dev"
echo "    6. Run guards:      make guards"
echo ""
echo "════════════════════════════════════════════════════════════"