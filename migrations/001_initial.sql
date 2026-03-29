-- 001_initial.sql
-- Aceryx v1 foundational schema (spec 001)

CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "vector";

CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tenants (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    branding        JSONB NOT NULL DEFAULT '{}',
    terminology     JSONB NOT NULL DEFAULT '{}',
    settings        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN tenants.branding IS
'{"company_name":"string","logo_url":"string","favicon_url":"string","colors":{"primary":"#hex","secondary":"#hex","accent":"#hex"},"powered_by":true}';
COMMENT ON COLUMN tenants.terminology IS
'{"case":"string","cases":"string","task":"string","tasks":"string","inbox":"string"}';
COMMENT ON COLUMN tenants.settings IS
'{"default_theme":"light","sla_warning_pct":0.75}';

CREATE TABLE principals (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    type            TEXT NOT NULL CHECK (type IN ('human', 'agent')),
    name            TEXT NOT NULL,
    email           TEXT,
    password_hash   TEXT,
    api_key_hash    TEXT,
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, email)
);

COMMENT ON COLUMN principals.metadata IS
'{"avatar_url":"string","department":"string"}';

CREATE INDEX idx_principals_api_key ON principals(api_key_hash)
    WHERE api_key_hash IS NOT NULL;

CREATE TABLE themes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    name        TEXT NOT NULL,
    key         TEXT NOT NULL,
    preset      TEXT NOT NULL DEFAULT 'aura',
    mode        TEXT NOT NULL DEFAULT 'light' CHECK (mode IN ('light', 'dark')),
    overrides   JSONB NOT NULL DEFAULT '{}',
    is_default  BOOLEAN NOT NULL DEFAULT false,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    UNIQUE(tenant_id, key)
);

COMMENT ON COLUMN themes.overrides IS
'{"--primary-color":"#hex","--surface-ground":"#hex"}';

CREATE TABLE user_preferences (
    principal_id    UUID PRIMARY KEY REFERENCES principals(id),
    theme_id        UUID REFERENCES themes(id),
    locale          TEXT NOT NULL DEFAULT 'en',
    notifications   JSONB NOT NULL DEFAULT '{}',
    preferences     JSONB NOT NULL DEFAULT '{}',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN user_preferences.notifications IS
'{"email_enabled":true,"digest_mode":false}';
COMMENT ON COLUMN user_preferences.preferences IS
'{"inbox_page_size":25,"case_list_columns":["case_number","status"]}';

CREATE TABLE sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    principal_id    UUID NOT NULL REFERENCES principals(id),
    token_hash      TEXT NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    ip_address      TEXT,
    user_agent      TEXT
);

CREATE INDEX idx_sessions_principal ON sessions(principal_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

CREATE TABLE case_types (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    name        TEXT NOT NULL,
    version     INTEGER NOT NULL DEFAULT 1,
    schema      JSONB NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  UUID NOT NULL REFERENCES principals(id),
    UNIQUE(tenant_id, name, version)
);

COMMENT ON COLUMN case_types.schema IS
'{"fields":[{"id":"loan_amount","type":"number","required":true}],"validation":{}}';

CREATE TABLE workflows (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    name        TEXT NOT NULL,
    case_type   TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  UUID NOT NULL REFERENCES principals(id),
    UNIQUE(tenant_id, name)
);

CREATE TABLE workflow_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id     UUID NOT NULL REFERENCES workflows(id),
    version         INTEGER NOT NULL,
    status          TEXT NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft', 'published', 'withdrawn')),
    ast             JSONB NOT NULL,
    yaml_source     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID NOT NULL REFERENCES principals(id),
    published_at    TIMESTAMPTZ,
    UNIQUE(workflow_id, version)
);

COMMENT ON COLUMN workflow_versions.ast IS
'{"nodes":[],"edges":[],"metadata":{}}';

CREATE INDEX idx_wfv_published ON workflow_versions(workflow_id, status)
    WHERE status = 'published';

CREATE TABLE cases (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL REFERENCES tenants(id),
    case_type_id     UUID NOT NULL REFERENCES case_types(id),
    case_number      TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'open'
                     CHECK (status IN ('open', 'in_progress', 'completed', 'cancelled')),
    data             JSONB NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by       UUID NOT NULL REFERENCES principals(id),
    assigned_to      UUID REFERENCES principals(id),
    due_at           TIMESTAMPTZ,
    priority         INTEGER NOT NULL DEFAULT 0,
    version          INTEGER NOT NULL DEFAULT 1,
    workflow_id      UUID NOT NULL REFERENCES workflows(id),
    workflow_version INTEGER NOT NULL,
    UNIQUE(tenant_id, case_number)
);

COMMENT ON COLUMN cases.data IS
'{"domain_fields":{},"attachments":[]}';

CREATE INDEX idx_cases_tenant_status ON cases(tenant_id, status);
CREATE INDEX idx_cases_assigned ON cases(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX idx_cases_due ON cases(due_at) WHERE due_at IS NOT NULL AND status NOT IN ('completed', 'cancelled');
CREATE INDEX idx_cases_fts ON cases USING GIN (to_tsvector('english', data::text));

CREATE TABLE case_steps (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id         UUID NOT NULL REFERENCES cases(id),
    step_id         TEXT NOT NULL,
    state           TEXT NOT NULL DEFAULT 'pending'
                    CHECK (state IN ('pending', 'ready', 'active', 'completed', 'failed', 'skipped')),
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    result          JSONB,
    events          JSONB NOT NULL DEFAULT '[]',
    error           JSONB,
    assigned_to     UUID REFERENCES principals(id),
    sla_deadline    TIMESTAMPTZ,
    retry_count     INTEGER NOT NULL DEFAULT 0,
    draft_data      JSONB,
    metadata        JSONB,
    UNIQUE(case_id, step_id)
);

COMMENT ON COLUMN case_steps.result IS
'{"output":{},"summary":"string"}';
COMMENT ON COLUMN case_steps.events IS
'[{"type":"started|progress|completed|failed","at":"timestamp","data":{}}]';
COMMENT ON COLUMN case_steps.error IS
'{"code":"string","message":"string","details":{}}';
COMMENT ON COLUMN case_steps.draft_data IS
'{"partial_form":{},"saved_at":"timestamp"}';
COMMENT ON COLUMN case_steps.metadata IS
'{"executor":"human|agent|integration","attempt":1}';

CREATE INDEX idx_cs_state ON case_steps(state) WHERE state IN ('ready', 'active');
CREATE INDEX idx_cs_sla ON case_steps(sla_deadline)
    WHERE state = 'active' AND sla_deadline IS NOT NULL;
CREATE INDEX idx_cs_assigned ON case_steps(assigned_to)
    WHERE assigned_to IS NOT NULL AND state = 'active';
CREATE INDEX idx_cs_case ON case_steps(case_id);

CREATE TABLE case_events (
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

COMMENT ON COLUMN case_events.data IS
'{"from_state":"pending","to_state":"active","context":{}}';

CREATE INDEX idx_ce_case ON case_events(case_id, created_at);
CREATE INDEX idx_ce_step ON case_events(case_id, step_id) WHERE step_id IS NOT NULL;

CREATE TABLE vault_documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    case_id         UUID REFERENCES cases(id),
    step_id         TEXT,
    filename        TEXT NOT NULL,
    mime_type       TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL,
    content_hash    TEXT NOT NULL,
    storage_uri     TEXT NOT NULL,
    uploaded_by     UUID NOT NULL REFERENCES principals(id),
    uploaded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    extracted_text  TEXT,
    extracted_data  JSONB,
    embedding       vector(1536),
    metadata        JSONB
);

COMMENT ON COLUMN vault_documents.extracted_data IS
'{"entities":[],"summary":"string"}';
COMMENT ON COLUMN vault_documents.metadata IS
'{"source":"upload","tags":["id-doc"]}';

CREATE INDEX idx_vd_case ON vault_documents(case_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_vd_hash ON vault_documents(content_hash);
CREATE INDEX idx_vd_tenant ON vault_documents(tenant_id) WHERE deleted_at IS NULL;

CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    name        TEXT NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

CREATE TABLE principal_roles (
    principal_id UUID NOT NULL REFERENCES principals(id),
    role_id      UUID NOT NULL REFERENCES roles(id),
    PRIMARY KEY (principal_id, role_id)
);

CREATE TABLE role_permissions (
    role_id     UUID NOT NULL REFERENCES roles(id),
    permission  TEXT NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE prompt_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    name            TEXT NOT NULL,
    version         INTEGER NOT NULL DEFAULT 1,
    template        TEXT NOT NULL,
    output_schema   JSONB,
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID NOT NULL REFERENCES principals(id),
    UNIQUE(tenant_id, name, version)
);

COMMENT ON COLUMN prompt_templates.output_schema IS
'{"type":"object","properties":{}}';
COMMENT ON COLUMN prompt_templates.metadata IS
'{"model":"gpt-5.4","temperature":0.1}';

CREATE TABLE webhook_deliveries (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key TEXT NOT NULL UNIQUE,
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    payload         JSONB,
    status          TEXT NOT NULL DEFAULT 'processed',
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN webhook_deliveries.payload IS
'{"headers":{},"body":{},"source":"connector_name"}';

CREATE INDEX idx_wd_processed ON webhook_deliveries(processed_at);

CREATE TABLE case_number_sequences (
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    case_type   TEXT NOT NULL,
    last_number BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, case_type)
);

CREATE OR REPLACE FUNCTION seed_default_themes_for_tenant(p_tenant_id UUID)
RETURNS VOID
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO themes (tenant_id, name, key, preset, mode, overrides, is_default, sort_order)
    VALUES
        (p_tenant_id, 'Light', 'light', 'aura', 'light', '{}', true, 10),
        (p_tenant_id, 'Dark', 'dark', 'aura', 'dark', '{}', false, 20),
        (p_tenant_id, 'High Contrast Light', 'high-contrast-light', 'aura', 'light', '{}', false, 30),
        (p_tenant_id, 'High Contrast Dark', 'high-contrast-dark', 'aura', 'dark', '{}', false, 40)
    ON CONFLICT (tenant_id, key) DO NOTHING;
END;
$$;

CREATE OR REPLACE FUNCTION tenants_after_insert_seed_themes()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM seed_default_themes_for_tenant(NEW.id);
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_tenants_seed_themes
AFTER INSERT ON tenants
FOR EACH ROW
EXECUTE FUNCTION tenants_after_insert_seed_themes();

DO $$
BEGIN
    INSERT INTO themes (tenant_id, name, key, preset, mode, overrides, is_default, sort_order)
    SELECT
        t.id,
        v.name,
        v.key,
        'aura',
        v.mode,
        '{}'::jsonb,
        v.is_default,
        v.sort_order
    FROM tenants t
    CROSS JOIN (
        VALUES
            ('Light', 'light', 'light', true, 10),
            ('Dark', 'dark', 'dark', false, 20),
            ('High Contrast Light', 'high-contrast-light', 'light', false, 30),
            ('High Contrast Dark', 'high-contrast-dark', 'dark', false, 40)
    ) AS v(name, key, mode, is_default, sort_order)
    ON CONFLICT (tenant_id, key) DO NOTHING;
END $$;
