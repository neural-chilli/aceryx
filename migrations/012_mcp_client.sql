CREATE TABLE IF NOT EXISTS mcp_server_cache (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    server_url      TEXT NOT NULL,
    tools           JSONB NOT NULL,
    last_discovered TIMESTAMPTZ NOT NULL DEFAULT now(),
    status          TEXT NOT NULL CHECK (status IN ('active', 'error', 'stale')),
    error_message   TEXT,
    UNIQUE (tenant_id, server_url)
);

CREATE INDEX IF NOT EXISTS idx_mcp_server_cache_tenant
    ON mcp_server_cache (tenant_id, last_discovered DESC);
