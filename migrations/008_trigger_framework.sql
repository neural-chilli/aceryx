CREATE TABLE trigger_instances (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL REFERENCES tenants(id),
    channel_id       UUID NOT NULL,
    plugin_id        TEXT NOT NULL,
    plugin_version   TEXT NOT NULL,
    status           TEXT NOT NULL CHECK (status IN ('starting', 'running', 'stopping', 'stopped', 'error')),
    started_at       TIMESTAMPTZ,
    stopped_at       TIMESTAMPTZ,
    events_received  BIGINT NOT NULL DEFAULT 0,
    events_processed BIGINT NOT NULL DEFAULT 0,
    events_failed    BIGINT NOT NULL DEFAULT 0,
    last_event_at    TIMESTAMPTZ,
    error_message    TEXT,
    restart_count    INTEGER NOT NULL DEFAULT 0,
    config           JSONB NOT NULL
);

COMMENT ON COLUMN trigger_instances.config IS 'Trigger runtime config JSON payload';

CREATE INDEX idx_trigger_instances_channel ON trigger_instances (channel_id);
CREATE INDEX idx_trigger_instances_tenant ON trigger_instances (tenant_id, status);

CREATE TABLE trigger_checkpoints (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trigger_id       UUID NOT NULL REFERENCES trigger_instances(id) ON DELETE CASCADE,
    checkpoint_key   TEXT NOT NULL,
    checkpoint_value TEXT NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(trigger_id, checkpoint_key)
);
