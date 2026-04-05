package triggers

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) Create(ctx context.Context, instance *TriggerInstanceRecord) error {
	if s == nil || s.db == nil || instance == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO trigger_instances (
    id, tenant_id, channel_id, plugin_id, plugin_version, status,
    started_at, stopped_at, events_received, events_processed, events_failed,
    last_event_at, error_message, restart_count, config
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, NULLIF($13, ''), $14, $15::jsonb
)
`, instance.ID, instance.TenantID, instance.ChannelID, instance.PluginID, instance.PluginVersion, instance.Status,
		instance.StartedAt, instance.StoppedAt, instance.EventsReceived, instance.EventsProcessed, instance.EventsFailed,
		instance.LastEventAt, instance.ErrorMessage, instance.RestartCount, string(instance.Config))
	if err != nil {
		return fmt.Errorf("create trigger instance: %w", err)
	}
	return nil
}

func (s *PostgresStore) Update(ctx context.Context, instance *TriggerInstanceRecord) error {
	if s == nil || s.db == nil || instance == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE trigger_instances
SET status = $2,
    started_at = $3,
    stopped_at = $4,
    events_received = $5,
    events_processed = $6,
    events_failed = $7,
    last_event_at = $8,
    error_message = NULLIF($9, ''),
    restart_count = $10,
    config = $11::jsonb
WHERE id = $1
`, instance.ID, instance.Status, instance.StartedAt, instance.StoppedAt,
		instance.EventsReceived, instance.EventsProcessed, instance.EventsFailed,
		instance.LastEventAt, instance.ErrorMessage, instance.RestartCount, string(instance.Config))
	if err != nil {
		return fmt.Errorf("update trigger instance: %w", err)
	}
	return nil
}

func (s *PostgresStore) Get(ctx context.Context, id uuid.UUID) (*TriggerInstanceRecord, error) {
	if s == nil || s.db == nil {
		return nil, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, channel_id, plugin_id, plugin_version, status,
       started_at, stopped_at, events_received, events_processed, events_failed,
       last_event_at, COALESCE(error_message, ''), restart_count, config
FROM trigger_instances
WHERE id = $1
`, id)
	return scanTriggerRecord(row)
}

func (s *PostgresStore) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*TriggerInstanceRecord, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, channel_id, plugin_id, plugin_version, status,
       started_at, stopped_at, events_received, events_processed, events_failed,
       last_event_at, COALESCE(error_message, ''), restart_count, config
FROM trigger_instances
WHERE tenant_id = $1
ORDER BY started_at DESC NULLS LAST, id
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list trigger instances by tenant: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanTriggerRows(rows)
}

func (s *PostgresStore) ListByChannel(ctx context.Context, channelID uuid.UUID) ([]*TriggerInstanceRecord, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, channel_id, plugin_id, plugin_version, status,
       started_at, stopped_at, events_received, events_processed, events_failed,
       last_event_at, COALESCE(error_message, ''), restart_count, config
FROM trigger_instances
WHERE channel_id = $1
ORDER BY started_at DESC NULLS LAST, id
`, channelID)
	if err != nil {
		return nil, fmt.Errorf("list trigger instances by channel: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanTriggerRows(rows)
}

func scanTriggerRecord(row *sql.Row) (*TriggerInstanceRecord, error) {
	out := &TriggerInstanceRecord{}
	if err := row.Scan(&out.ID, &out.TenantID, &out.ChannelID, &out.PluginID, &out.PluginVersion, &out.Status,
		&out.StartedAt, &out.StoppedAt, &out.EventsReceived, &out.EventsProcessed, &out.EventsFailed,
		&out.LastEventAt, &out.ErrorMessage, &out.RestartCount, &out.Config); err != nil {
		return nil, err
	}
	return out, nil
}

func scanTriggerRows(rows *sql.Rows) ([]*TriggerInstanceRecord, error) {
	out := make([]*TriggerInstanceRecord, 0)
	for rows.Next() {
		item := &TriggerInstanceRecord{}
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ChannelID, &item.PluginID, &item.PluginVersion, &item.Status,
			&item.StartedAt, &item.StoppedAt, &item.EventsReceived, &item.EventsProcessed, &item.EventsFailed,
			&item.LastEventAt, &item.ErrorMessage, &item.RestartCount, &item.Config); err != nil {
			return nil, fmt.Errorf("scan trigger instance row: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trigger instance rows: %w", err)
	}
	return out, nil
}
