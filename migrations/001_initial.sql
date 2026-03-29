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
