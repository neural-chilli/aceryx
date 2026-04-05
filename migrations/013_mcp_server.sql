CREATE TABLE IF NOT EXISTS mcp_api_keys (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id),
    user_id      UUID NOT NULL REFERENCES principals(id),
    name         TEXT NOT NULL,
    key_hash     TEXT NOT NULL,
    roles        JSONB NOT NULL DEFAULT '[]'::jsonb,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_mcp_keys_tenant
    ON mcp_api_keys (tenant_id);

CREATE TABLE IF NOT EXISTS mcp_tool_invocations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL REFERENCES tenants(id),
    user_id        UUID NOT NULL REFERENCES principals(id),
    api_key_id     UUID NOT NULL REFERENCES mcp_api_keys(id) ON DELETE CASCADE,
    tool_name      TEXT NOT NULL,
    arguments      JSONB NOT NULL DEFAULT '{}'::jsonb,
    result_status  TEXT NOT NULL,
    duration_ms    INTEGER NOT NULL DEFAULT 0,
    source         TEXT NOT NULL DEFAULT 'mcp',
    correlation_id TEXT NOT NULL,
    depth          INTEGER NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_mcp_tool_invocations_tenant_created
    ON mcp_tool_invocations (tenant_id, created_at DESC);
