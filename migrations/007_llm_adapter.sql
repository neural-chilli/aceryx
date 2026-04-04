CREATE TABLE IF NOT EXISTS llm_provider_configs (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             UUID NOT NULL REFERENCES tenants(id),
    provider              TEXT NOT NULL CHECK (provider IN (
        'openai', 'anthropic', 'google', 'cohere', 'mistral', 'ollama', 'custom'
    )),
    name                  TEXT NOT NULL,
    endpoint_url          TEXT,
    api_key_secret        TEXT NOT NULL,
    default_model         TEXT NOT NULL,
    max_tokens            INTEGER NOT NULL DEFAULT 4096,
    temperature           NUMERIC(3,2) NOT NULL DEFAULT 0.7,
    is_default            BOOLEAN NOT NULL DEFAULT FALSE,
    is_fallback           BOOLEAN NOT NULL DEFAULT FALSE,
    enabled               BOOLEAN NOT NULL DEFAULT TRUE,
    model_map             JSONB NOT NULL DEFAULT '{}'::jsonb,
    model_pricing         JSONB NOT NULL DEFAULT '{}'::jsonb,
    requests_per_min      INTEGER NOT NULL DEFAULT 60,
    tenant_requests_per_min INTEGER NOT NULL DEFAULT 60,
    monthly_token_budget  BIGINT NOT NULL DEFAULT 0,
    monthly_cost_budget   NUMERIC(12,6) NOT NULL DEFAULT 0,
    azure_api_version     TEXT,
    azure_deployment      TEXT,
    azure                 BOOLEAN NOT NULL DEFAULT FALSE,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN llm_provider_configs.model_map IS
    'Maps model hints to provider models, e.g. {"small":"gpt-4o-mini","large":"gpt-4o"}';
COMMENT ON COLUMN llm_provider_configs.model_pricing IS
    'Per-model pricing in USD token units, e.g. {"gpt-4o":{"cost_per_input_token":0.000005,"cost_per_output_token":0.000015}}';

CREATE INDEX IF NOT EXISTS idx_llm_configs_tenant ON llm_provider_configs (tenant_id);

CREATE TABLE IF NOT EXISTS llm_invocations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    provider_id     UUID NOT NULL REFERENCES llm_provider_configs(id),
    provider        TEXT NOT NULL,
    model           TEXT NOT NULL,
    purpose         TEXT NOT NULL,
    input_tokens    INTEGER,
    output_tokens   INTEGER,
    total_tokens    INTEGER,
    duration_ms     INTEGER NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('success', 'error', 'rate_limited')),
    error_message   TEXT,
    cost_usd        NUMERIC(10,6),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_llm_invocations_tenant ON llm_invocations (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_invocations_purpose ON llm_invocations (tenant_id, purpose, created_at DESC);

CREATE TABLE IF NOT EXISTS llm_usage_monthly (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id),
    year_month        TEXT NOT NULL,
    total_tokens      BIGINT NOT NULL DEFAULT 0,
    total_cost_usd    NUMERIC(12,6) NOT NULL DEFAULT 0,
    invocation_count  INTEGER NOT NULL DEFAULT 0,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, year_month)
);
