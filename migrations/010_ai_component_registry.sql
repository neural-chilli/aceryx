CREATE TABLE IF NOT EXISTS tenant_ai_components (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    definition  JSONB NOT NULL,
    created_by  UUID NOT NULL REFERENCES principals(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_ai_components
    ON tenant_ai_components (tenant_id, (definition->>'id'));

CREATE INDEX IF NOT EXISTS idx_tenant_ai_components_tenant
    ON tenant_ai_components (tenant_id);

COMMENT ON COLUMN tenant_ai_components.definition IS
    'Full AI component definition JSON (AIComponentDef).';
