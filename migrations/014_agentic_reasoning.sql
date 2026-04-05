CREATE TABLE IF NOT EXISTS agentic_reasoning_traces (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id),
    case_id           UUID NOT NULL REFERENCES cases(id),
    step_id           TEXT NOT NULL,
    instance_id       UUID NOT NULL,
    model_used        TEXT NOT NULL,
    goal              TEXT NOT NULL,
    status            TEXT NOT NULL CHECK (status IN ('running', 'concluded', 'escalated', 'timeout', 'error')),
    conclusion        JSONB,
    total_iterations  INTEGER NOT NULL DEFAULT 0,
    total_tool_calls  INTEGER NOT NULL DEFAULT 0,
    total_tokens      INTEGER NOT NULL DEFAULT 0,
    total_duration_ms INTEGER,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_agentic_traces_case ON agentic_reasoning_traces (case_id);
CREATE INDEX IF NOT EXISTS idx_agentic_traces_tenant ON agentic_reasoning_traces (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS agentic_reasoning_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id    UUID NOT NULL REFERENCES agentic_reasoning_traces(id) ON DELETE CASCADE,
    iteration   INTEGER NOT NULL,
    sequence    INTEGER NOT NULL,
    event_type  TEXT NOT NULL CHECK (event_type IN ('goal_set', 'plan', 'tool_call', 'tool_result', 'observation', 'reasoning', 'conclusion', 'escalation', 'error')),
    content     JSONB NOT NULL,
    tool_id     TEXT,
    tool_source TEXT,
    tool_safety TEXT,
    side_effect BOOLEAN NOT NULL DEFAULT false,
    token_count INTEGER,
    duration_ms INTEGER,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_agentic_events_trace ON agentic_reasoning_events (trace_id, iteration, sequence);
