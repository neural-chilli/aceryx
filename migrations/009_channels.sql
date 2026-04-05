CREATE TABLE channels (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL REFERENCES tenants(id),
    name           TEXT NOT NULL,
    type           TEXT NOT NULL CHECK (type IN ('email', 'webhook', 'form', 'file_drop', 'plugin')),
    plugin_ref     TEXT,
    config         JSONB NOT NULL,
    case_type_id   UUID NOT NULL REFERENCES case_types(id),
    workflow_id    UUID REFERENCES workflows(id),
    adapter_config JSONB NOT NULL DEFAULT '{}',
    dedup_config   JSONB NOT NULL DEFAULT '{}',
    enabled        BOOLEAN NOT NULL DEFAULT TRUE,
    deleted_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN channels.config IS '{"email":{},"webhook":{},"form":{},"file_drop":{},"plugin":{}}';
COMMENT ON COLUMN channels.adapter_config IS '{"mappings":[{"source":"payload.foo","target":"case.data.foo","type":"direct"}]}';
COMMENT ON COLUMN channels.dedup_config IS '{"strategy":"field_hash","fields":["message_id"],"window_mins":1440}';

CREATE TABLE channel_events (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL REFERENCES tenants(id),
    channel_id     UUID NOT NULL REFERENCES channels(id),
    raw_payload    JSONB,
    attachments    JSONB NOT NULL DEFAULT '[]',
    case_id        UUID REFERENCES cases(id),
    status         TEXT NOT NULL CHECK (status IN ('processed', 'deduped', 'failed')),
    error_message  TEXT,
    processing_ms  INTEGER,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN channel_events.raw_payload IS '{"payload":{},"_truncated":true}';
COMMENT ON COLUMN channel_events.attachments IS '[{"vault_id":"uuid","filename":"file.pdf","content_type":"application/pdf","size":1234,"checksum":"sha256"}]';

CREATE INDEX idx_channels_tenant ON channels (tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_channel_events_channel ON channel_events (channel_id, created_at DESC);
CREATE INDEX idx_channel_events_tenant ON channel_events (tenant_id, created_at DESC);
CREATE INDEX idx_channel_events_case ON channel_events (case_id);
