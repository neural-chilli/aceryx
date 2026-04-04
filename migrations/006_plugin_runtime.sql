CREATE TABLE IF NOT EXISTS plugins (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_id       TEXT NOT NULL,
    name            TEXT NOT NULL,
    version         TEXT NOT NULL,
    type            TEXT NOT NULL CHECK (type IN ('step', 'trigger')),
    category        TEXT NOT NULL,
    licence_tier    TEXT NOT NULL CHECK (licence_tier IN ('open_source', 'commercial')),
    maturity_tier   TEXT NOT NULL CHECK (maturity_tier IN ('core', 'certified', 'community', 'generated')),
    manifest_hash   TEXT NOT NULL,
    wasm_hash       TEXT NOT NULL,
    is_latest       BOOLEAN NOT NULL DEFAULT TRUE,
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'error')),
    error_message   TEXT,
    loaded_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata        JSONB NOT NULL DEFAULT '{}'
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_plugins_id_version ON plugins (plugin_id, version);
CREATE UNIQUE INDEX IF NOT EXISTS idx_plugins_latest ON plugins (plugin_id) WHERE is_latest = TRUE;

CREATE TABLE IF NOT EXISTS plugin_invocations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    plugin_id       TEXT NOT NULL,
    plugin_version  TEXT NOT NULL,
    wasm_hash       TEXT NOT NULL,
    case_id         UUID,
    step_id         TEXT,
    invocation_type TEXT NOT NULL CHECK (invocation_type IN ('step_execute', 'trigger_event')),
    input_hash      TEXT NOT NULL,
    output_hash     TEXT,
    duration_ms     INTEGER NOT NULL,
    host_calls      JSONB NOT NULL DEFAULT '[]',
    status          TEXT NOT NULL CHECK (status IN ('success', 'error', 'timeout')),
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_plugin_invocations_tenant ON plugin_invocations (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_plugin_invocations_case ON plugin_invocations (case_id);
CREATE INDEX IF NOT EXISTS idx_plugin_invocations_plugin ON plugin_invocations (plugin_id, plugin_version, created_at DESC);
