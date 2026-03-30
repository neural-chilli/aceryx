CREATE TABLE IF NOT EXISTS secrets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    key             TEXT NOT NULL,
    value_encrypted TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, key)
);

COMMENT ON COLUMN secrets.value_encrypted IS
'Encrypted secret value. v1 may store plaintext for local development.';

CREATE TABLE IF NOT EXISTS document_templates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    name        TEXT NOT NULL,
    version     INTEGER NOT NULL DEFAULT 1,
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    template    JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  UUID NOT NULL REFERENCES principals(id),
    UNIQUE(tenant_id, name, version)
);

COMMENT ON COLUMN document_templates.template IS
'{"filename":"Offer-{{case.case_number}}.pdf","layout":[{"type":"paragraph","text":"..."}]}';

CREATE TABLE IF NOT EXISTS webhook_routes (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               UUID NOT NULL REFERENCES tenants(id),
    path                    TEXT NOT NULL UNIQUE,
    case_type               TEXT NOT NULL,
    mode                    TEXT NOT NULL DEFAULT 'create' CHECK (mode IN ('create', 'update')),
    signature_header        TEXT,
    signature_secret_key    TEXT,
    idempotency_key_path    TEXT,
    case_number_field_path  TEXT,
    created_by              UUID NOT NULL REFERENCES principals(id),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS connector_dead_letters (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    connector_key   TEXT NOT NULL,
    action_key      TEXT NOT NULL,
    case_id         UUID REFERENCES cases(id),
    step_id         TEXT,
    payload         JSONB,
    error_message   TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
