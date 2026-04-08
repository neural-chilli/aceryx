CREATE TABLE ai_assistant_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    user_id         UUID NOT NULL REFERENCES principals(id),
    page_context    TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ai_assistant_messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID NOT NULL REFERENCES ai_assistant_sessions(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
    content         TEXT NOT NULL,
    mode            TEXT CHECK (mode IN ('describe', 'refactor', 'explain', 'test_generate')),
    yaml_before     TEXT,
    yaml_after      TEXT,
    diff            TEXT,
    applied         BOOLEAN NOT NULL DEFAULT FALSE,
    model_used      TEXT,
    tokens_used     INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ai_assistant_diffs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    workflow_id     UUID NOT NULL REFERENCES workflows(id),
    message_id      UUID NOT NULL REFERENCES ai_assistant_messages(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES principals(id),
    prompt          TEXT NOT NULL,
    diff            TEXT NOT NULL,
    applied         BOOLEAN NOT NULL DEFAULT FALSE,
    applied_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_sessions_tenant_updated
    ON ai_assistant_sessions (tenant_id, updated_at DESC);

CREATE INDEX idx_ai_messages_session_created
    ON ai_assistant_messages (session_id, created_at);

CREATE INDEX idx_ai_diffs_workflow_created
    ON ai_assistant_diffs (workflow_id, created_at DESC);
