---
title: Database Schema
weight: 4
---

Aceryx uses **PostgreSQL 17** as its primary data store, with the `pgvector` extension for embedding-based search and semantic operations.

## Setup

### Prerequisites

- PostgreSQL 17+ (can use Docker: `docker-compose up postgres`)
- pgvector extension installed (`CREATE EXTENSION IF NOT EXISTS vector;`)

### Migrations

Aceryx has **6 migrations**:

1. `001_initial.sql` — Foundational schema (tenants, principals, cases, workflows, etc.)
2. `002_case_management_views.sql` — Materialized views for reporting
3. `003_rbac_auth.sql` — RBAC tables, auth_events, and session management
4. `004_connector_framework.sql` — Connector credentials, secrets, document templates, webhooks
5. `005_llm_reports.sql` — LLM reports, saved_reports, report materialized views
6. `006_plugin_runtime.sql` — Plugin tables and plugin_invocations

Migrations are stored in `migrations/` and applied sequentially on startup or manually.

**Run migrations**:
```bash
# Automatic (on server startup)
go run ./cmd/aceryx serve

# Manual
go run ./cmd/aceryx migrate

# Reset database (development only)
qp db:fresh
```

Each migration file is idempotent and can be re-run safely.

## Core Tables

### Tenants

Multi-tenancy root. All other tables include `tenant_id` for strict isolation.

```sql
CREATE TABLE tenants (
  id UUID PRIMARY KEY,
  name VARCHAR NOT NULL,
  slug VARCHAR NOT NULL UNIQUE,
  createdAt TIMESTAMP NOT NULL,

  -- Branding
  organizationName VARCHAR,
  logoUrl TEXT,
  primaryColor VARCHAR,
  secondaryColor VARCHAR,

  -- Configuration
  maxCases INT DEFAULT 10000,
  maxUsers INT DEFAULT 100,

  isActive BOOLEAN DEFAULT true
);
```

### Principals (Users & Agents)

```sql
CREATE TABLE principals (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  type TEXT NOT NULL CHECK (type IN ('human', 'agent')),
  name TEXT NOT NULL,
  email TEXT,
  password_hash TEXT,
  api_key_hash TEXT,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  metadata JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE(tenant_id, email),
  FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);

CREATE INDEX idx_principals_api_key ON principals(api_key_hash)
  WHERE api_key_hash IS NOT NULL;
```

Principal types are "human" (user) and "agent" (service account). Status is "active" or "disabled".

### Roles & Permissions

```sql
CREATE TABLE roles (
  id UUID PRIMARY KEY,
  tenantId UUID NOT NULL REFERENCES tenants(id),
  name VARCHAR NOT NULL,
  displayName VARCHAR,
  description TEXT,
  createdAt TIMESTAMP NOT NULL,

  UNIQUE(tenantId, name),
  FOREIGN KEY (tenantId) REFERENCES tenants(id)
);

CREATE TABLE role_permissions (
  id UUID PRIMARY KEY,
  roleId UUID NOT NULL REFERENCES roles(id),
  permission VARCHAR NOT NULL,  -- e.g., "cases:create", "admin:users"

  UNIQUE(roleId, permission),
  FOREIGN KEY (roleId) REFERENCES roles(id)
);

CREATE TABLE principal_roles (
  principalId UUID NOT NULL REFERENCES principals(id),
  roleId UUID NOT NULL REFERENCES roles(id),

  PRIMARY KEY (principalId, roleId),
  FOREIGN KEY (principalId) REFERENCES principals(id),
  FOREIGN KEY (roleId) REFERENCES roles(id)
);
```

### Case Types

Schema definitions for different case types.

```sql
CREATE TABLE case_types (
  id UUID PRIMARY KEY,
  tenantId UUID NOT NULL REFERENCES tenants(id),
  key VARCHAR NOT NULL,
  displayName VARCHAR NOT NULL,
  description TEXT,

  -- JSON Schema for case data validation
  schema JSONB NOT NULL,

  createdAt TIMESTAMP NOT NULL,
  updatedAt TIMESTAMP NOT NULL,
  createdBy UUID NOT NULL REFERENCES principals(id),

  isActive BOOLEAN DEFAULT true,

  UNIQUE(tenantId, key),
  FOREIGN KEY (tenantId) REFERENCES tenants(id),
  FOREIGN KEY (createdBy) REFERENCES principals(id)
);

CREATE INDEX idx_case_types_tenant ON case_types(tenantId);
```

### Cases

The main case domain table.

```sql
CREATE TABLE cases (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  case_type_id UUID NOT NULL REFERENCES case_types(id),

  status TEXT NOT NULL DEFAULT 'open', -- open, in_progress, completed, cancelled
  data JSONB NOT NULL,  -- Case data (validated against schema)

  -- Optimistic locking: increments on data changes only
  version INT NOT NULL DEFAULT 1,

  -- Execution tracking
  workflow_version_id UUID,
  current_step_id UUID,

  -- Timestamps
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  closed_at TIMESTAMPTZ,
  closed_by UUID REFERENCES principals(id),

  created_by UUID NOT NULL REFERENCES principals(id),

  -- SLA
  sla_deadline TIMESTAMPTZ,
  sla_breach BOOLEAN DEFAULT false,

  FOREIGN KEY (tenant_id) REFERENCES tenants(id),
  FOREIGN KEY (case_type_id) REFERENCES case_types(id),
  FOREIGN KEY (current_step_id) REFERENCES case_steps(id)
);

CREATE INDEX idx_cases_tenant_status ON cases(tenant_id, status);
CREATE INDEX idx_cases_tenant_created ON cases(tenant_id, created_at DESC);
CREATE INDEX idx_cases_tenant_deadline ON cases(tenant_id, sla_deadline);
CREATE INDEX idx_cases_tenant_type ON cases(tenant_id, case_type_id);

-- Full-text search index
CREATE INDEX idx_cases_search ON cases USING GIN (
  to_tsvector('english', data::text)
);
```

Case status: `open`, `in_progress`, `completed`, `cancelled`

### Case Steps (Workflow Execution)

Execution state for each step in a workflow.

```sql
CREATE TABLE case_steps (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  case_id UUID NOT NULL REFERENCES cases(id),

  step_name VARCHAR NOT NULL,  -- "intake", "review", etc.
  step_type VARCHAR NOT NULL,  -- "human_task", "integration", "agent", "rule", "timer", "plugin"

  status VARCHAR NOT NULL DEFAULT 'pending', -- pending, ready, active, completed, failed, skipped

  -- Execution metadata
  activated_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  failed_at TIMESTAMPTZ,
  failure_reason TEXT,

  -- Result & outcome (set when completed)
  result JSONB,  -- Step output data
  outcome VARCHAR,  -- "approved", "rejected", "completed", etc.

  -- Retry tracking
  attempt_count INT DEFAULT 0,
  next_retry_at TIMESTAMPTZ,

  -- Task assignment (if type='human_task')
  assigned_to UUID REFERENCES principals(id),
  assigned_at TIMESTAMPTZ,

  -- Draft saving (if type='human_task')
  draft JSONB,
  draft_saved_at TIMESTAMPTZ,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE(case_id, step_name),
  FOREIGN KEY (tenant_id) REFERENCES tenants(id),
  FOREIGN KEY (case_id) REFERENCES cases(id),
  FOREIGN KEY (assigned_to) REFERENCES principals(id)
);

CREATE INDEX idx_case_steps_case ON case_steps(case_id);
CREATE INDEX idx_case_steps_tenant_status ON case_steps(tenant_id, status);
CREATE INDEX idx_case_steps_assigned ON case_steps(assigned_to, status);
CREATE INDEX idx_case_steps_deadline ON case_steps(tenant_id, completed_at) WHERE status = 'active';
```

Step state: `pending`, `ready`, `active`, `completed`, `failed`, `skipped`

### Workflows

Workflow definitions (DAG).

```sql
CREATE TABLE workflows (
  id UUID PRIMARY KEY,
  tenantId UUID NOT NULL REFERENCES tenants(id),
  key VARCHAR NOT NULL,
  displayName VARCHAR NOT NULL,
  description TEXT,

  -- Current published version ID
  publishedVersionId UUID,

  createdAt TIMESTAMP NOT NULL,
  updatedAt TIMESTAMP NOT NULL,
  createdBy UUID NOT NULL REFERENCES principals(id),

  isActive BOOLEAN DEFAULT true,

  UNIQUE(tenantId, key),
  FOREIGN KEY (tenantId) REFERENCES tenants(id),
  FOREIGN KEY (createdBy) REFERENCES principals(id)
);

CREATE INDEX idx_workflows_tenant ON workflows(tenantId);
```

### Workflow Versions

Each workflow has multiple versions (draft, published, withdrawn).

```sql
CREATE TABLE workflow_versions (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  workflow_id UUID NOT NULL REFERENCES workflows(id),

  version INT NOT NULL,
  status VARCHAR NOT NULL, -- draft, published, withdrawn

  -- Workflow definition (DAG as YAML/JSON)
  definition TEXT NOT NULL,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by UUID NOT NULL REFERENCES principals(id),
  published_at TIMESTAMPTZ,
  published_by UUID REFERENCES principals(id),

  UNIQUE(workflow_id, version),
  FOREIGN KEY (tenant_id) REFERENCES tenants(id),
  FOREIGN KEY (workflow_id) REFERENCES workflows(id),
  FOREIGN KEY (created_by) REFERENCES principals(id),
  FOREIGN KEY (published_by) REFERENCES principals(id)
);

CREATE INDEX idx_workflow_versions_workflow ON workflow_versions(workflow_id);
CREATE INDEX idx_workflow_versions_published ON workflow_versions(status) WHERE status = 'published';
```

Workflow version status: `draft`, `published`, `withdrawn`

### Case Events (Audit Log)

Append-only, hash-chained audit log. Every state change is recorded.

```sql
CREATE TABLE case_events (
  id UUID PRIMARY KEY,
  tenantId UUID NOT NULL REFERENCES tenants(id),
  caseId UUID NOT NULL REFERENCES cases(id),

  timestamp TIMESTAMP NOT NULL,
  actor UUID REFERENCES principals(id),

  action VARCHAR NOT NULL,  -- "case:created", "step:completed", "data:updated"
  resource VARCHAR NOT NULL,  -- "cases/{id}", "cases/{id}/steps/{step_id}"

  -- What changed
  changes JSONB,

  -- Hash-chaining for tamper-evidence
  hash VARCHAR NOT NULL,  -- SHA256 of this event
  previousHash VARCHAR,   -- Hash of the event before this one

  createdAt TIMESTAMP NOT NULL,

  FOREIGN KEY (tenantId) REFERENCES tenants(id),
  FOREIGN KEY (caseId) REFERENCES cases(id),
  FOREIGN KEY (actor) REFERENCES principals(id)
);

CREATE INDEX idx_case_events_case ON case_events(caseId);
CREATE INDEX idx_case_events_tenant_timestamp ON case_events(tenantId, timestamp DESC);
CREATE INDEX idx_case_events_action ON case_events(action);

-- For hash-chain verification
CREATE INDEX idx_case_events_hash ON case_events(hash);
```

### Vault Documents

Content-addressed document storage with vector embeddings for semantic search.

```sql
CREATE TABLE vault_documents (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),

  filename VARCHAR NOT NULL,
  mime_type VARCHAR,
  size INT NOT NULL,

  -- Content hash (SHA256) — used for deduplication
  content_hash VARCHAR NOT NULL,

  -- Storage location (could be S3, local filesystem, etc.)
  storage_path TEXT NOT NULL,

  -- Document lifecycle
  uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  uploaded_by UUID NOT NULL REFERENCES principals(id),

  -- Association
  case_id UUID REFERENCES cases(id),

  -- Content extraction and embedding
  extracted_text TEXT,
  extracted_data JSONB,
  embedding vector(1536),

  -- Lifecycle
  is_deleted BOOLEAN DEFAULT false,
  deleted_at TIMESTAMPTZ,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

  FOREIGN KEY (tenant_id) REFERENCES tenants(id),
  FOREIGN KEY (uploaded_by) REFERENCES principals(id),
  FOREIGN KEY (case_id) REFERENCES cases(id)
);

CREATE INDEX idx_vault_documents_case ON vault_documents(case_id);
CREATE INDEX idx_vault_documents_content_hash ON vault_documents(content_hash);
CREATE INDEX idx_vault_documents_tenant ON vault_documents(tenant_id);
CREATE INDEX idx_vault_documents_embedding ON vault_documents USING ivfflat(embedding vector_cosine_ops) WHERE embedding IS NOT NULL;
```

The embedding field stores pgvector embeddings (1536 dimensions) for semantic search capabilities.

### Connector Credentials

Secure storage of external system credentials.

```sql
CREATE TABLE connector_credentials (
  id UUID PRIMARY KEY,
  tenantId UUID NOT NULL REFERENCES tenants(id),

  connectorKey VARCHAR NOT NULL,  -- "slack", "jira", "http"
  name VARCHAR NOT NULL,  -- Friendly name

  -- Encrypted credentials
  credentialsEncrypted BYTEA NOT NULL,  -- Encrypted JSON
  credentialsNonce BYTEA,  -- IV for encryption

  createdAt TIMESTAMP NOT NULL,
  createdBy UUID NOT NULL REFERENCES principals(id),
  updatedAt TIMESTAMP NOT NULL,

  isActive BOOLEAN DEFAULT true,

  UNIQUE(tenantId, connectorKey, name),
  FOREIGN KEY (tenantId) REFERENCES tenants(id),
  FOREIGN KEY (createdBy) REFERENCES principals(id)
);

CREATE INDEX idx_connector_credentials_tenant ON connector_credentials(tenantId);
```

### Agent Prompt Templates

Prompt templates for LLM steps.

```sql
CREATE TABLE agent_prompt_templates (
  id UUID PRIMARY KEY,
  tenantId UUID NOT NULL REFERENCES tenants(id),

  name VARCHAR NOT NULL,
  description TEXT,

  version INT NOT NULL DEFAULT 1,
  template TEXT NOT NULL,

  -- LLM configuration
  model VARCHAR NOT NULL,  -- "gpt-4", "claude-3-opus", etc.
  temperature FLOAT DEFAULT 0.3,
  maxTokens INT DEFAULT 500,

  createdAt TIMESTAMP NOT NULL,
  createdBy UUID NOT NULL REFERENCES principals(id),
  updatedAt TIMESTAMP NOT NULL,
  updatedBy UUID NOT NULL REFERENCES principals(id),

  UNIQUE(tenantId, name, version),
  FOREIGN KEY (tenantId) REFERENCES tenants(id),
  FOREIGN KEY (createdBy) REFERENCES principals(id),
  FOREIGN KEY (updatedBy) REFERENCES principals(id)
);

CREATE INDEX idx_agent_templates_tenant ON agent_prompt_templates(tenantId);
```

### Reports

Custom report definitions.

```sql
CREATE TABLE reports (
  id UUID PRIMARY KEY,
  tenantId UUID NOT NULL REFERENCES tenants(id),

  name VARCHAR NOT NULL,
  description TEXT,

  reportType VARCHAR,  -- "custom_query", "summary", "ageing", etc.
  query TEXT,  -- SQL query for custom reports

  createdAt TIMESTAMP NOT NULL,
  createdBy UUID NOT NULL REFERENCES principals(id),
  updatedAt TIMESTAMP NOT NULL,
  updatedBy UUID NOT NULL REFERENCES principals(id),

  isPublic BOOLEAN DEFAULT false,

  UNIQUE(tenantId, name),
  FOREIGN KEY (tenantId) REFERENCES tenants(id),
  FOREIGN KEY (createdBy) REFERENCES principals(id),
  FOREIGN KEY (updatedBy) REFERENCES principals(id)
);

CREATE INDEX idx_reports_tenant ON reports(tenantId);
```

### Sessions

JWT/session tracking.

```sql
CREATE TABLE sessions (
  id UUID PRIMARY KEY,
  principalId UUID NOT NULL REFERENCES principals(id),
  tenantId UUID NOT NULL REFERENCES tenants(id),

  tokenHash VARCHAR NOT NULL,  -- Hash of JWT
  expiresAt TIMESTAMP NOT NULL,

  isRevoked BOOLEAN DEFAULT false,
  revokedAt TIMESTAMP,

  createdAt TIMESTAMP NOT NULL,
  lastUsedAt TIMESTAMP,
  userAgent TEXT,
  ipAddress VARCHAR,

  FOREIGN KEY (principalId) REFERENCES principals(id),
  FOREIGN KEY (tenantId) REFERENCES tenants(id)
);

CREATE INDEX idx_sessions_principal ON sessions(principalId);
CREATE INDEX idx_sessions_expires ON sessions(expiresAt);
```

### Themes

Tenant-specific themes.

```sql
CREATE TABLE themes (
  id UUID PRIMARY KEY,
  tenantId UUID NOT NULL REFERENCES tenants(id),

  name VARCHAR NOT NULL,
  displayName VARCHAR,
  description TEXT,

  -- CSS variables as JSON
  cssVariables JSONB NOT NULL,

  isDefault BOOLEAN DEFAULT false,
  createdAt TIMESTAMP NOT NULL,

  UNIQUE(tenantId, name),
  FOREIGN KEY (tenantId) REFERENCES tenants(id)
);

CREATE INDEX idx_themes_tenant ON themes(tenantId);
```

### User Preferences

```sql
CREATE TABLE user_preferences (
  id UUID PRIMARY KEY,
  principalId UUID NOT NULL UNIQUE REFERENCES principals(id),
  tenantId UUID NOT NULL REFERENCES tenants(id),

  theme VARCHAR DEFAULT 'light',
  language VARCHAR DEFAULT 'en',
  timezone VARCHAR DEFAULT 'UTC',
  itemsPerPage INT DEFAULT 25,
  emailNotifications BOOLEAN DEFAULT true,

  preferencesJson JSONB,  -- Extensible

  updatedAt TIMESTAMP NOT NULL,

  FOREIGN KEY (principalId) REFERENCES principals(id),
  FOREIGN KEY (tenantId) REFERENCES tenants(id)
);
```

## Materialized Views

Materialized views provide pre-computed aggregates for dashboard and reporting queries.

### mv_cases_summary

Summary of cases by week, type, and status.

```sql
CREATE MATERIALIZED VIEW mv_cases_summary AS
SELECT
  tenantId,
  DATE_TRUNC('week', createdAt) as week,
  caseTypeId,
  status,
  COUNT(*) as count,
  AVG(EXTRACT(EPOCH FROM (COALESCE(closedAt, NOW()) - createdAt))) as avg_duration_seconds
FROM cases
GROUP BY tenantId, DATE_TRUNC('week', createdAt), caseTypeId, status;

CREATE INDEX idx_mv_cases_summary_tenant ON mv_cases_summary(tenantId);
```

### mv_sla_compliance

SLA metrics by week.

```sql
CREATE MATERIALIZED VIEW mv_sla_compliance AS
SELECT
  tenantId,
  DATE_TRUNC('week', createdAt) as week,
  COUNT(*) as total_cases,
  COUNT(*) FILTER (WHERE slaBreach = false) as on_time,
  COUNT(*) FILTER (WHERE slaBreach = true) as breached,
  ROUND(
    100.0 * COUNT(*) FILTER (WHERE slaBreach = false) / COUNT(*),
    2
  ) as compliance_rate
FROM cases
WHERE status IN ('closed', 'cancelled')
GROUP BY tenantId, DATE_TRUNC('week', createdAt);

CREATE INDEX idx_mv_sla_compliance_tenant ON mv_sla_compliance(tenantId);
```

## Refresh Schedule

Materialized views are refreshed:
- On-demand via API (`POST /reports/refresh`)
- Nightly via background job (configurable)
- After significant case operations (close, cancel)

## Key Indexes

Summary of important indexes (not exhaustive):

| Table | Index | Purpose |
|-------|-------|---------|
| cases | (tenantId, status) | Filter cases by status |
| cases | (tenantId, createdAt DESC) | Recent cases |
| cases | (tenantId, slaDeadline) | SLA monitoring |
| cases | GIN tsvector | Full-text search |
| case_steps | (assignedTo, status) | Inbox queries |
| case_events | (caseId) | Event log retrieval |
| case_events | (tenantId, timestamp DESC) | Activity feed |
| principals | (tenantId, email) | User lookup |
| workflows | (tenantId) | Workflow list |
| role_permissions | (roleId, permission) | Permission checks |

## Data Validation

### Schema Validation

Case data is validated against the case type's JSON schema **before** insert/update:

```go
// Pseudocode
func ValidateCaseData(data interface{}, schema *jsonschema.Schema) error {
  return schema.Validate(data)
}
```

Validation fails with 400 Bad Request if data doesn't match.

### Referential Integrity

All foreign key constraints are enforced by the database (FOREIGN KEY clauses with appropriate ON DELETE behavior).

### Audit Trail Integrity

The hash-chain in `case_events` is verified on read:

```go
// Pseudocode
func VerifyEventChain(caseId UUID) error {
  events := db.Query("SELECT hash, previousHash FROM case_events WHERE caseId = ? ORDER BY timestamp")
  for i, event := range events {
    if i == 0 && event.previousHash != nil {
      return errors.New("First event should not have previousHash")
    }
    if i > 0 && event.previousHash != events[i-1].hash {
      return errors.New("Hash chain broken at event " + i)
    }
    // Verify hash is correct for this event's data
    if ComputeHash(event) != event.hash {
      return errors.New("Event hash tampered")
    }
  }
  return nil
}
```

## Backup & Restore

### Backup

Backup includes:
- PostgreSQL database dump (via `pg_dump`)
- Vault documents archive (tarball of all stored files)

```bash
aceryx backup --output backup.tar.gz
```

### Restore

Restore from backup:

```bash
aceryx restore --input backup.tar.gz
```

This is destructive—it wipes the current database and vault.

## Performance Considerations

1. **Case data JSONB**: Ensure JSON queries use appropriate indexes (`USING GIN`)
2. **Materialized views**: Refresh periodically; queries don't block on refresh
3. **Audit log**: The hash-chain check is O(n); consider pagination for large cases
4. **Full-text search**: tsvector index is built on case data; keep search queries efficient
5. **Tenant isolation**: Every query filters by `tenant_id` to prevent accidental cross-tenant leaks

## PostgreSQL Extensions

Required:
- `uuid-ossp` — for UUID generation (if not using application layer)
- `pgvector` — for vector embeddings (for semantic search, if enabled)

Installation:

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "vector";
```
